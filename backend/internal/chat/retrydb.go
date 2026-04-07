package chat

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

const persistMaxAttempts = 5

func isRetryableDB(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "40001", "40P01", "08000", "08003", "08006", "08001":
			return true
		}
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "connection refused") ||
		strings.Contains(s, "connection reset") ||
		strings.Contains(s, "broken pipe") ||
		strings.Contains(s, "i/o timeout") ||
		strings.Contains(s, "no connection") ||
		strings.Contains(s, "server closed") ||
		strings.Contains(s, "connection pool")
}

// withPersistRetry runs op up to persistMaxAttempts times with exponential backoff on retryable errors.
func withPersistRetry(ctx context.Context, op func() error) error {
	var last error
	for attempt := 0; attempt < persistMaxAttempts; attempt++ {
		if attempt > 0 {
			wait := time.Duration(100*(1<<uint(attempt-1))) * time.Millisecond
			if wait > 2*time.Second {
				wait = 2 * time.Second
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
		}
		last = op()
		if last == nil {
			return nil
		}
		if !isRetryableDB(last) {
			return last
		}
	}
	return last
}

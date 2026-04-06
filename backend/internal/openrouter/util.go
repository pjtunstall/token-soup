package openrouter

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"strings"
)

func isOpenRouterTimeout(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// DefaultModelID returns the configured OpenRouter model id (public.messages.sender_model for bot rows).
func DefaultModelID() string {
	return openRouterModel
}

// formatModelDisplayName turns an OpenRouter model id (e.g. "meta-llama/llama-3.1-8b-instruct:free")
// into a short display label (last path segment after '/').
func formatModelDisplayName(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return openRouterName
	}
	if i := strings.LastIndex(model, "/"); i >= 0 && i+1 < len(model) {
		return model[i+1:]
	}
	return model
}

// closeBodyOnCancel starts a goroutine that closes body when ctx is canceled.
// The returned function must be called when the body is no longer needed (e.g. defer) to stop the goroutine.
func closeBodyOnCancel(ctx context.Context, body io.ReadCloser) (stop func()) {
	ch := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = body.Close()
		case <-ch:
		}
	}()
	return func() { close(ch) }
}

func openRouterLogDeltaStreamEnabled() bool {
	v := strings.TrimSpace(os.Getenv("OPENROUTER_LOG_DELTA_STREAM"))
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}

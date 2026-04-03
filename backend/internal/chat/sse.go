package chat

import (
	"fmt"
	"net/http"
)

// WriteSSE writes one SSE event with a JSON payload and flushes if possible.
func WriteSSE(w http.ResponseWriter, flusher http.Flusher, payload []byte) error {
	if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
		return err
	}
	if flusher != nil {
		flusher.Flush()
	}
	return nil
}

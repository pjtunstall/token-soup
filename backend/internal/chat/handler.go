package chat

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/lestrrat-go/jwx/v2/jwk"

	"token-soup/internal/openrouter"
)

var chatJobCounter uint64

type chatPostBody struct {
	Message string `json:"message"`
}

// ChatHandler handles POST /api/chat with SSE response and Supabase JWT auth.
func ChatHandler(store *HistoryStore, cache *jwk.Cache, jwksURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := AuthUserID(r.Context(), cache, jwksURL, r.Header.Get("Authorization"))
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var body chatPostBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		prompt := store.Append(userID, body.Message, true)
		if prompt == "" {
			http.Error(w, "empty message", http.StatusBadRequest)
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)

		jobID := atomic.AddUint64(&chatJobCounter, 1)
		ctx := r.Context()

		assistantText, canceled := openrouter.StreamOpenRouter(ctx, jobID, prompt, func(ev openrouter.AIEvent) {
			payload, err := json.Marshal(ev)
			if err != nil {
				log.Printf("marshal AI event: %v", err)
				return
			}
			if err := WriteSSE(w, flusher, payload); err != nil {
				log.Printf("sse write: %v", err)
			}
		})

		if !canceled && strings.TrimSpace(assistantText) != "" {
			store.Append(userID, assistantText, false)
		}
	}
}

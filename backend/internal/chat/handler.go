package chat

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/lestrrat-go/jwx/v2/jwk"

	"token-soup/internal/openrouter"
)

var chatJobCounter uint64

type chatPostBody struct {
	Message    string `json:"message"`
	SenderName string `json:"senderName,omitempty"`
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

		senderName := strings.TrimSpace(body.SenderName)
		if senderName == "" {
			senderName = "You"
		}
		history := store.AppendUser(userID, senderName, body.Message)
		if len(history) == 0 {
			http.Error(w, "empty message", http.StatusBadRequest)
			return
		}
		turns := make([]openrouter.HistoryTurn, len(history))
		for i := range history {
			turns[i] = openrouter.HistoryTurn{
				User:    history[i].SenderType == SenderUser,
				Content: history[i].Content,
			}
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

		var sseMu sync.Mutex
		emit := func(ev openrouter.AIEvent) {
			sseMu.Lock()
			defer sseMu.Unlock()
			payload, err := json.Marshal(ev)
			if err != nil {
				log.Printf("marshal AI event: %v", err)
				return
			}
			if err := WriteSSE(w, flusher, payload); err != nil {
				log.Printf("sse write: %v", err)
			}
		}

		firstUserText := strings.TrimSpace(body.Message)
		var wg sync.WaitGroup
		if len(history) == 1 && firstUserText != "" {
			wg.Add(1)
			go func() {
				defer wg.Done()
				raw, err := openrouter.GenerateChatTitle(ctx, firstUserText)
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					log.Printf("chat title: %v", err)
					raw = ""
				}
				title := NormalizeChatTitle(raw, firstUserText)
				sseMu.Lock()
				defer sseMu.Unlock()
				if ctx.Err() != nil {
					return
				}
				payload, err := json.Marshal(openrouter.AIEvent{
					Type:  "chat_title",
					JobID: jobID,
					Text:  title,
				})
				if err != nil {
					log.Printf("marshal chat_title: %v", err)
					return
				}
				if err := WriteSSE(w, flusher, payload); err != nil {
					log.Printf("sse write chat_title: %v", err)
				}
			}()
		}

		assistantText, canceled := openrouter.StreamOpenRouter(ctx, jobID, openrouter.GuruSystemPrompt(), turns, emit)

		wg.Wait()

		if !canceled && strings.TrimSpace(assistantText) != "" {
			store.AppendBot(userID, openrouter.RoleGuruName, openrouter.DefaultModelID(), assistantText, nil)
		}
	}
}

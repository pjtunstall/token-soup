package chat

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lestrrat-go/jwx/v2/jwk"
)

type chatMessageJSON struct {
	SenderType  string  `json:"sender_type"`
	SenderName  string  `json:"sender_name"`
	Content     string  `json:"content"`
	SenderModel *string `json:"sender_model,omitempty"`
	Reasoning   *string `json:"reasoning,omitempty"`
	RoleID      *int64  `json:"role_id,omitempty"`
}

type chatDetailJSON struct {
	ID       int64             `json:"id"`
	Title    string            `json:"title"`
	Messages []chatMessageJSON `json:"messages"`
}

func messageToJSON(m Message) chatMessageJSON {
	m = clampMessageForDisplay(m)
	out := chatMessageJSON{
		SenderType: string(m.SenderType),
		SenderName: m.SenderName,
		Content:    m.Content,
		RoleID:     m.RoleID,
	}
	if m.SenderType == SenderBot {
		out.SenderModel = m.SenderModel
		out.Reasoning = m.Reasoning
	}
	return out
}

func chatsDBError(w http.ResponseWriter, err error, operation, fallback string) {
	log.Printf("chats %s: %v", operation, err)
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		switch pe.Code {
		case "42P01":
			http.Error(
				w,
				`database has no public.chats table; apply schema (see supabase/schema.sql)`,
				http.StatusInternalServerError,
			)
			return
		}
	}
	http.Error(w, fallback, http.StatusInternalServerError)
}

func chatsRequire(persist *Persist, cache *jwk.Cache, jwksURL string, w http.ResponseWriter, r *http.Request) (userID string, ok bool) {
	uid, err := AuthUserID(r.Context(), cache, jwksURL, r.Header.Get("Authorization"))
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", false
	}
	if persist == nil || persist.Pool == nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return "", false
	}
	return uid, true
}

func parseListPageParams(r *http.Request) (limit int, offset int, ok bool) {
	limit = 10
	offset = 0
	limitRaw := r.URL.Query().Get("limit")
	if limitRaw != "" {
		v, err := strconv.Atoi(limitRaw)
		if err != nil || v < 1 || v > 100 {
			return 0, 0, false
		}
		limit = v
	}
	offsetRaw := r.URL.Query().Get("offset")
	if offsetRaw != "" {
		v, err := strconv.Atoi(offsetRaw)
		if err != nil || v < 0 {
			return 0, 0, false
		}
		offset = v
	}
	return limit, offset, true
}

// ChatsCollectionHandler: GET /api/chats (list)
func ChatsCollectionHandler(persist *Persist, cache *jwk.Cache, jwksURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := chatsRequire(persist, cache, jwksURL, w, r)
		if !ok {
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		limit, offset, ok := parseListPageParams(r)
		if !ok {
			http.Error(w, "invalid pagination params", http.StatusBadRequest)
			return
		}
		chats, err := persist.ListChats(r.Context(), userID, limit, offset)
		if err != nil {
			chatsDBError(w, err, "list", "could not list chats")
			return
		}
		if chats == nil {
			chats = []ChatSummary{}
		}
		for i := range chats {
			chats[i] = clampChatSummaryForDisplay(chats[i])
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(chats)
	}
}

// ChatsItemHandler: GET /api/chats/{id} (messages)
func ChatsItemHandler(persist *Persist, cache *jwk.Cache, jwksURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := chatsRequire(persist, cache, jwksURL, w, r)
		if !ok {
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		idStr := r.PathValue("id")
		chatID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || chatID < 1 {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		title, err := persist.GetChatTitle(r.Context(), userID, chatID)
		if err != nil {
			if errors.Is(err, errChatNotFound) {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			chatsDBError(w, err, "get title", "could not load chat")
			return
		}
		msgs, err := persist.LoadMessages(r.Context(), userID, chatID)
		if err != nil {
			chatsDBError(w, err, "load messages", "could not load messages")
			return
		}
		out := chatDetailJSON{
			ID:       chatID,
			Title:    truncateWithEllipsis(title, maxTitleRunes),
			Messages: make([]chatMessageJSON, 0, len(msgs)),
		}
		for i := range msgs {
			out.Messages = append(out.Messages, messageToJSON(msgs[i]))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}
}

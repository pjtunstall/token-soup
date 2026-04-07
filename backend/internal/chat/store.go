package chat

import (
	"strconv"
	"strings"
	"sync"
)

// HistoryStore keeps chat messages in memory for building OpenRouter turns (same shape as public.messages).
// Keys are either per-user (legacy / ephemeral) or per-user-and-chat when chatID != 0.
type HistoryStore struct {
	mu   sync.Mutex
	byID map[string][]Message
}

func chatSessionKey(userID string, chatID int64) string {
	if chatID == 0 {
		return userID
	}
	return userID + "\x1e" + strconv.FormatInt(chatID, 10)
}

// AppendUser appends a user message for the legacy single-thread-per-user session (no persisted chat id).
func (s *HistoryStore) AppendUser(userID, senderName, content string) []Message {
	return s.appendUserKey(chatSessionKey(userID, 0), senderName, content)
}

// AppendUserChat appends a user message for a persisted chat thread (chatID from Postgres).
func (s *HistoryStore) AppendUserChat(userID string, chatID int64, senderName, content string) []Message {
	return s.appendUserKey(chatSessionKey(userID, chatID), senderName, content)
}

func (s *HistoryStore) appendUserKey(key, senderName, content string) []Message {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.byID == nil {
		s.byID = make(map[string][]Message)
	}
	m := Message{
		SenderName: strings.TrimSpace(senderName),
		SenderType: SenderUser,
		Content:    trimmed,
	}
	if m.SenderName == "" {
		m.SenderName = "You"
	}
	h := append(s.byID[key], m)
	s.byID[key] = h
	return h
}

// AppendBot appends a bot message for the legacy single-thread-per-user session.
func (s *HistoryStore) AppendBot(userID, senderName, senderModel, content string, reasoning *string) {
	s.appendBotKey(chatSessionKey(userID, 0), senderName, senderModel, content, reasoning)
}

// AppendBotChat appends a bot message for a persisted chat thread.
func (s *HistoryStore) AppendBotChat(userID string, chatID int64, senderName, senderModel, content string, reasoning *string) {
	s.appendBotKey(chatSessionKey(userID, chatID), senderName, senderModel, content, reasoning)
}

func (s *HistoryStore) appendBotKey(key, senderName, senderModel, content string, reasoning *string) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return
	}
	sm := strings.TrimSpace(senderModel)
	if sm == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.byID == nil {
		s.byID = make(map[string][]Message)
	}
	name := strings.TrimSpace(senderName)
	if name == "" {
		name = "assistant"
	}
	m := Message{
		SenderName:  name,
		SenderType:  SenderBot,
		SenderModel: &sm,
		Reasoning:   reasoning,
		Content:     trimmed,
	}
	s.byID[key] = append(s.byID[key], m)
}

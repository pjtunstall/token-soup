package chat

import (
	"strings"
	"sync"
)

// HistoryStore keeps per-user chat messages in memory (same shape as public.messages).
type HistoryStore struct {
	mu     sync.Mutex
	byUser map[string][]Message
}

// AppendUser appends a user message and returns the full history including it.
func (s *HistoryStore) AppendUser(userID, senderName, content string) []Message {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.byUser == nil {
		s.byUser = make(map[string][]Message)
	}
	m := Message{
		SenderName: strings.TrimSpace(senderName),
		SenderType: SenderUser,
		Content:    trimmed,
	}
	if m.SenderName == "" {
		m.SenderName = "You"
	}
	h := append(s.byUser[userID], m)
	s.byUser[userID] = h
	return h
}

// AppendBot appends a bot message after a completed assistant turn.
func (s *HistoryStore) AppendBot(userID, senderName, senderModel, content string, reasoning *string) {
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
	if s.byUser == nil {
		s.byUser = make(map[string][]Message)
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
	s.byUser[userID] = append(s.byUser[userID], m)
}

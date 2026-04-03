package chat

import (
	"strings"
	"sync"
)

// HistoryStore keeps per-user transcript lines in memory ("User: ..." / "Assistant: ...").
type HistoryStore struct {
	mu     sync.Mutex
	byUser map[string][]string
}

func (s *HistoryStore) AppendUserMessage(userID, text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.byUser == nil {
		s.byUser = make(map[string][]string)
	}
	h := s.byUser[userID]
	h = append(h, "User: "+trimmed)
	s.byUser[userID] = h
	return strings.Join(h, "\n\n")
}

// AppendAssistant records a completed assistant reply for the transcript.
func (s *HistoryStore) AppendAssistant(userID, text string) {
	t := strings.TrimSpace(text)
	if t == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.byUser == nil {
		s.byUser = make(map[string][]string)
	}
	s.byUser[userID] = append(s.byUser[userID], "Assistant: "+t)
}

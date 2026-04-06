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

// Append adds a transcript line for userID. fromUser selects "User: " vs "Assistant: ".
// For user messages, returns the full transcript; assistant lines return "".
func (s *HistoryStore) Append(userID, text string, fromUser bool) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.byUser == nil {
		s.byUser = make(map[string][]string)
	}
	prefix := "Assistant: "
	if fromUser {
		prefix = "User: "
	}
	h := append(s.byUser[userID], prefix+trimmed)
	s.byUser[userID] = h
	if fromUser {
		return strings.Join(h, "\n\n")
	}
	return ""
}

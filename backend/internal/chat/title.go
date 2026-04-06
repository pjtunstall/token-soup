package chat

import (
	"strings"
	"unicode"
)

const maxTitleRunes = 72

// NormalizeChatTitle applies strict display rules: single line, trim, strip quotes,
// collapse space, then truncate with "..." if still too long. Empty or unusable
// input falls back to FallbackTitleFromUserMessage.
func NormalizeChatTitle(raw string, firstUserMessage string) string {
	s := strings.TrimSpace(raw)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !prevSpace && b.Len() > 0 {
				b.WriteRune(' ')
				prevSpace = true
			}
			continue
		}
		prevSpace = false
		b.WriteRune(r)
	}
	s = strings.TrimSpace(b.String())
	for len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	s = strings.TrimRight(s, ".,;:!?")
	s = strings.TrimSpace(s)
	if s == "" {
		return FallbackTitleFromUserMessage(firstUserMessage)
	}
	return truncateWithEllipsis(s, maxTitleRunes)
}

// FallbackTitleFromUserMessage derives a title when the LLM is unavailable or canceled:
// prefer the first short sentence; otherwise truncate with "...".
func FallbackTitleFromUserMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return "…"
	}
	if end := firstSentenceEnd(msg); end > 0 {
		sent := strings.TrimSpace(msg[:end])
		sent = strings.TrimRight(sent, ".!?")
		sent = strings.TrimSpace(sent)
		r := []rune(sent)
		if len(r) <= maxTitleRunes {
			return sent
		}
		return truncateWithEllipsis(sent, maxTitleRunes)
	}
	return truncateWithEllipsis(msg, maxTitleRunes)
}

func firstSentenceEnd(s string) int {
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		switch rs[i] {
		case '!', '?':
			return i + 1
		case '.':
			if i+1 < len(rs) && rs[i+1] == '.' {
				continue
			}
			return i + 1
		}
	}
	return -1
}

func truncateWithEllipsis(s string, maxRunes int) string {
	if maxRunes < 4 {
		return "..."
	}
	runes := []rune(strings.TrimSpace(s))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes-3]) + "..."
}

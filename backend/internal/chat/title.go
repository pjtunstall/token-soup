package chat

import (
	"strings"
	"unicode"
)

const maxTitleRunes = 72

// NormalizeChatTitle applies strict display rules: single line, trim, strip quotes,
// collapse space, drop a trailing comma only, then truncate with "..." if still too long.
// Sentence-ending . ? ! are kept when the result fits within maxTitleRunes.
// Empty or unusable input falls back to FallbackTitleFromUserMessage.
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
	// Drop a stray trailing comma only; keep . ? ! when the title fits (short questions, etc.).
	s = strings.TrimRight(s, ",")
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
		sent = strings.TrimRight(sent, ",")
		sent = strings.TrimSpace(sent)
		r := []rune(sent)
		if len(r) <= maxTitleRunes {
			return sent
		}
		return truncateWithEllipsis(sent, maxTitleRunes)
	}
	return truncateWithEllipsis(msg, maxTitleRunes)
}

// isApostrophe reports ASCII ' between two letters (don't, it's), not quote delimiters.
func isApostrophe(rs []rune, i int) bool {
	if i <= 0 || i >= len(rs)-1 || rs[i] != '\'' {
		return false
	}
	return unicode.IsLetter(rs[i-1]) && unicode.IsLetter(rs[i+1])
}

// firstSentenceEnd returns the index after the first sentence-ending .?! outside of quoted spans.
// Double "..." and single '...' pairs are tracked so a ? inside dialogue does not end the outer sentence.
func firstSentenceEnd(s string) int {
	rs := []rune(s)
	inDouble := false
	inSingle := false
	for i := 0; i < len(rs); i++ {
		switch rs[i] {
		case '"':
			inDouble = !inDouble
			continue
		case '\'':
			if isApostrophe(rs, i) {
				continue
			}
			inSingle = !inSingle
			continue
		}
		if inDouble || inSingle {
			continue
		}
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

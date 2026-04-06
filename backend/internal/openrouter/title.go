package openrouter

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"strings"
)

const titleSystemPrompt = `You write short chat session titles. Output rules (must follow exactly):
- Output a single line only: plain UTF-8 text, no markdown, no quotes, no numbering.
- 3 to 8 words, at most 60 characters total.
- No trailing punctuation (no period at the end).
- Summarize the user's first message in a neutral, concise phrase.
If you cannot comply, output the single word Untitled`

// GenerateChatTitle calls OpenRouter once (non-streaming) to produce a title for the first user message.
func GenerateChatTitle(ctx context.Context, userMessage string) (string, error) {
	apiKey := strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
	if apiKey == "" {
		return "", errors.New("OPENROUTER_API_KEY not set")
	}
	trimmed := strings.TrimSpace(userMessage)
	if trimmed == "" {
		return "", errors.New("empty message")
	}
	const maxIn = 4000
	if len(trimmed) > maxIn {
		trimmed = trimmed[:maxIn]
	}

	body := openRouterTitleRequest{
		Model:       openRouterModel,
		Stream:      false,
		MaxTokens:   48,
		Temperature: 0.2,
		Messages: []openRouterMessage{
			{Role: "system", Content: titleSystemPrompt},
			{Role: "user", Content: trimmed},
		},
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := newOpenRouterHTTPRequest(ctx, jsonBody, apiKey)
	if err != nil {
		return "", err
	}
	resp, err := openRouterHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		log.Printf("openrouter title: status=%d body=%s", resp.StatusCode, truncateForLog(bodyBytes))
		return "", errors.New("openrouter title http error")
	}

	var payload openRouterResponse
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		return "", err
	}
	if payload.Error.Message != "" {
		return "", errors.New(payload.Error.Message)
	}
	if len(payload.Choices) == 0 {
		return "", errors.New("no choices")
	}
	return strings.TrimSpace(payload.Choices[0].Message.Content), nil
}

type openRouterTitleRequest struct {
	Model       string              `json:"model"`
	Messages    []openRouterMessage `json:"messages"`
	Stream      bool                `json:"stream"`
	MaxTokens   int                 `json:"max_tokens"`
	Temperature float32             `json:"temperature,omitempty"`
}

func truncateForLog(b []byte) string {
	const max = 200
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "…"
}

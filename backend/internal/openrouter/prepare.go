package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
)

// GuruSystemPrompt returns the default "guru" persona system prompt (matches future public.roles.description).
func GuruSystemPrompt() string {
	return openRouterSystemPrompt
}

func newOpenRouterRequest(systemPrompt string, history []HistoryTurn) openRouterRequest {
	msgs := make([]openRouterMessage, 0, 1+len(history))
	msgs = append(msgs, openRouterMessage{Role: "system", Content: systemPrompt})
	for _, t := range history {
		role := "assistant"
		if t.User {
			role = "user"
		}
		msgs = append(msgs, openRouterMessage{Role: role, Content: t.Content})
	}
	return openRouterRequest{
		Model:       openRouterModel,
		Messages:    msgs,
		Stream:      true,
		Temperature: 1.2,
	}
}

func newOpenRouterHTTPRequest(ctx context.Context, jsonBody []byte, apiKey string) (*http.Request, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openRouterURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

// openRouterStreamPrepare validates env and history, builds the JSON body. ok is false when the
// caller should return ("", false) immediately (emit may have been called for missing API key).
func openRouterStreamPrepare(jobID uint64, systemPrompt string, history []HistoryTurn, emit func(AIEvent)) (assistantName, apiKey string, jsonBody []byte, ok bool) {
	assistantName = formatModelDisplayName(openRouterModel)
	apiKey = os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		log.Println("OPENROUTER_API_KEY is not set; skipping OpenRouter call")
		emitOpenRouterNotConfigured(emit, jobID, assistantName)
		return "", "", nil, false
	}
	if len(history) == 0 {
		return "", "", nil, false
	}
	for _, t := range history {
		if strings.TrimSpace(t.Content) == "" {
			return "", "", nil, false
		}
	}
	requestBody := newOpenRouterRequest(systemPrompt, history)
	var err error
	jsonBody, err = json.Marshal(requestBody)
	if err != nil {
		log.Printf("failed to marshal OpenRouter request: %v", err)
		return "", "", nil, false
	}
	return assistantName, apiKey, jsonBody, true
}

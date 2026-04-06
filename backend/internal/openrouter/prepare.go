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

func newOpenRouterRequest(userPrompt string) openRouterRequest {
	return openRouterRequest{
		Model: openRouterModel,
		Messages: []openRouterMessage{
			{Role: "system", Content: openRouterSystemPrompt},
			{Role: "user", Content: userPrompt},
		},
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

// openRouterStreamPrepare validates env and prompt, builds the JSON body. ok is false when the
// caller should return ("", false) immediately (emit may have been called for missing API key).
func openRouterStreamPrepare(jobID uint64, userPrompt string, emit func(AIEvent)) (assistantName, apiKey string, jsonBody []byte, ok bool) {
	assistantName = formatModelDisplayName(openRouterModel)
	apiKey = os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		log.Println("OPENROUTER_API_KEY is not set; skipping OpenRouter call")
		emitOpenRouterNotConfigured(emit, jobID, assistantName)
		return "", "", nil, false
	}
	if strings.TrimSpace(userPrompt) == "" {
		return "", "", nil, false
	}
	requestBody := newOpenRouterRequest(userPrompt)
	var err error
	jsonBody, err = json.Marshal(requestBody)
	if err != nil {
		log.Printf("failed to marshal OpenRouter request: %v", err)
		return "", "", nil, false
	}
	return assistantName, apiKey, jsonBody, true
}

package openrouter

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
)

func emitAI(emit func(AIEvent), jobID uint64, assistantName string, event AIEvent) {
	if emit == nil {
		return
	}
	event.JobID = jobID
	event.Name = assistantName
	emit(event)
}

func emitOpenRouterNotConfigured(emit func(AIEvent), jobID uint64, assistantName string) {
	emitAI(emit, jobID, assistantName, AIEvent{
		Type: "ai_replace",
		Text: assistantName + ": [OpenRouter not configured]",
	})
	emitAI(emit, jobID, assistantName, AIEvent{Type: "ai_end"})
}

func handleOpenRouterRequestError(ctx context.Context, jobID uint64, err error, emit func(AIEvent), assistantName string) (canceled bool) {
	if errors.Is(ctx.Err(), context.Canceled) {
		log.Printf("openrouter: job=%d canceled before response", jobID)
		return true
	}
	log.Printf("OpenRouter request failed: %v", err)
	if isOpenRouterTimeout(err) {
		emitAI(emit, jobID, assistantName, AIEvent{
			Type: "ai_replace",
			Text: assistantName + ": " + openRouterBusyUserMsg,
		})
	} else {
		emitAI(emit, jobID, assistantName, AIEvent{
			Type: "ai_replace",
			Text: assistantName + ": [request failed]",
		})
	}
	return false
}

// handleOpenRouterHTTPError reads and emits for resp with StatusCode >= 400. resp.Body must still be open.
func handleOpenRouterHTTPError(ctx context.Context, jobID uint64, resp *http.Response, emit func(AIEvent), assistantName string) (canceled bool) {
	if errors.Is(ctx.Err(), context.Canceled) {
		log.Printf("openrouter: job=%d canceled with error status", jobID)
		return true
	}
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		emitAI(emit, jobID, assistantName, AIEvent{
			Type: "ai_replace",
			Text: assistantName + ": [invalid response]",
		})
		return false
	}

	var payload openRouterResponse
	if err := json.Unmarshal(bodyBytes, &payload); err == nil && payload.Error.Message != "" {
		emitAI(emit, jobID, assistantName, AIEvent{
			Type: "ai_replace",
			Text: assistantName + ": [api error: " + payload.Error.Message + "]",
		})
		return false
	}

	emitAI(emit, jobID, assistantName, AIEvent{
		Type: "ai_replace",
		Text: assistantName + ": [api error]",
	})
	return false
}

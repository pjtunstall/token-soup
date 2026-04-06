package openrouter

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"
)

func openRouterHTTPPost(ctx context.Context, jsonBody []byte, apiKey string) (*http.Response, error) {
	req, err := newOpenRouterHTTPRequest(ctx, jsonBody, apiKey)
	if err != nil {
		log.Printf("failed to create OpenRouter request: %v", err)
		return nil, err
	}
	return openRouterHTTPClient.Do(req)
}

// finishOpenRouterStream runs after SSE parsing succeeds: empty-response handling, usage log, ai_end.
func finishOpenRouterStream(
	stream *openRouterStreamState,
	emit func(AIEvent),
	jobID uint64,
	started time.Time,
) (assistantText string, canceled bool) {
	if strings.TrimSpace(stream.buf.String()) == "" {
		stream.endDeltaStreamLogLine()
		emitAI(emit, jobID, stream.assistantName, AIEvent{
			Type: "ai_replace",
			Text: stream.assistantName + ": [empty response]",
		})
		emitAI(emit, jobID, stream.assistantName, AIEvent{
			Type: "ai_end",
		})
		return "", false
	}

	assistantText = stream.buf.String()
	stream.endDeltaStreamLogLine()
	if stream.latestUsage.hasAny() {
		log.Printf(
			"openrouter: job=%d usage prompt_tokens=%d completion_tokens=%d total_tokens=%d",
			jobID,
			stream.latestUsage.PromptTokens,
			stream.latestUsage.CompletionTokens,
			stream.latestUsage.TotalTokens,
		)
	}
	log.Printf("openrouter: job=%d done after=%s response=%q", jobID, time.Since(started).Truncate(time.Millisecond), assistantText)
	emitAI(emit, jobID, stream.assistantName, AIEvent{
		Type: "ai_end",
	})
	return assistantText, false
}

// StreamOpenRouter streams one completion to emit and returns the full assistant text (if any).
// When ctx is canceled (client abort/stop), upstream OpenRouter consumption stops and canceled is true.
func StreamOpenRouter(ctx context.Context, jobID uint64, userPrompt string, emit func(AIEvent)) (assistantText string, canceled bool) {
	assistantName, apiKey, jsonBody, ok := openRouterStreamPrepare(jobID, userPrompt, emit)
	if !ok {
		return "", false
	}

	started := time.Now()
	log.Printf("openrouter: job=%d prompt_bytes=%d", jobID, len(userPrompt))

	resp, err := openRouterHTTPPost(ctx, jsonBody, apiKey)
	if err != nil {
		return "", handleOpenRouterRequestError(ctx, jobID, err, emit, assistantName)
	}
	defer resp.Body.Close()

	stopClose := closeBodyOnCancel(ctx, resp.Body)
	defer stopClose()

	if resp.StatusCode >= 400 {
		return "", handleOpenRouterHTTPError(ctx, jobID, resp, emit, assistantName)
	}

	stream := &openRouterStreamState{
		ctx:            ctx,
		jobID:          jobID,
		emit:           emit,
		started:        started,
		assistantName:  assistantName,
		logDeltaStream: openRouterLogDeltaStreamEnabled(),
	}

	streamErr := parseSSEStream(resp, stream.onSSEData)
	if streamErr != nil {
		return "", handleOpenRouterStreamError(stream, streamErr)
	}

	return finishOpenRouterStream(stream, emit, jobID, started)
}

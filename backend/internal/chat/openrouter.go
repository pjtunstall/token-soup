package chat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	openRouterURL   = "https://openrouter.ai/api/v1/chat/completions"
	openRouterModel = "openrouter/free"
	openRouterName  = "openrouter"

	openRouterBusyUserMsg = "OpenRouter seems to be busy right now; please try again later."

	openRouterSystemPrompt = "In this game, your challenge is to appear helpful, but answer only with more questions. Don't give any information away. Instead probe the reasons for the user's request. Challenge them like a therapist or spiritual teacher, gently persistent at first, but drilling more as the response goes on. You can use wordplay and distant associations to reframe the discussion. Try to conclude with one particularly pointed and personally unsettling question."
)

func isOpenRouterTimeout(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

type openRouterMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openRouterRequest struct {
	Model       string              `json:"model"`
	Messages    []openRouterMessage `json:"messages"`
	Stream      bool                `json:"stream,omitempty"`
	Temperature float32             `json:"temperature,omitempty"`
}

type openRouterChoice struct {
	Message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
}

type openRouterError struct {
	Message string `json:"message"`
}

type openRouterResponse struct {
	Choices []openRouterChoice `json:"choices"`
	Error   openRouterError    `json:"error"`
}

type openRouterStreamDelta struct {
	Role             string `json:"role,omitempty"`
	Content          string `json:"content,omitempty"`
	Reasoning        string `json:"reasoning,omitempty"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
}

type openRouterStreamChoice struct {
	Delta openRouterStreamDelta `json:"delta"`
}

type openRouterStreamResponse struct {
	Model   string                   `json:"model,omitempty"`
	Choices []openRouterStreamChoice `json:"choices"`
	Error   openRouterError          `json:"error"`
	Usage   openRouterUsage          `json:"usage"`
}

type openRouterUsage struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

func (u openRouterUsage) hasAny() bool {
	return u.PromptTokens > 0 || u.CompletionTokens > 0 || u.TotalTokens > 0
}

// AIEvent is one JSON object sent on the chat SSE stream (same shape as the old WebSocket payloads).
type AIEvent struct {
	Type  string `json:"type"`
	JobID uint64 `json:"jobId"`
	Text  string `json:"text,omitempty"`
	// Name is the assistant label (from OpenRouter’s resolved model id), without trailing colon.
	Name string `json:"name,omitempty"`
}

var openRouterHTTPClient = &http.Client{
	// Give streaming enough time to complete.
	Timeout: 90 * time.Second,
}

func openRouterLogDeltaStreamEnabled() bool {
	v := strings.TrimSpace(os.Getenv("OPENROUTER_LOG_DELTA_STREAM"))
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}

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

func parseSSEStream(resp *http.Response, onData func(data []byte) error) error {
	// OpenRouter streams SSE. We parse `data:` lines and treat a blank line as
	// the end of one SSE event.
	reader := bufio.NewReader(resp.Body)
	var dataLines []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return err
		}
		line = strings.TrimRight(line, "\r\n")

		if line == "" {
			if len(dataLines) > 0 {
				joined := strings.Join(dataLines, "\n")
				if err := onData([]byte(joined)); err != nil {
					return err
				}
				dataLines = dataLines[:0]
			}
			if err == io.EOF {
				return nil
			}
			continue
		}

		// Ignore SSE comments (lines starting with ':').
		if strings.HasPrefix(line, ":") {
			if err == io.EOF {
				return nil
			}
			continue
		}

		if rest, ok := strings.CutPrefix(line, "data:"); ok {
			val := strings.TrimSpace(rest)
			dataLines = append(dataLines, val)
		}

		if err == io.EOF {
			// If the stream ended without a blank line, flush what we have.
			if len(dataLines) > 0 {
				joined := strings.Join(dataLines, "\n")
				return onData([]byte(joined))
			}
			return nil
		}
	}
}

func emitAI(emit func(AIEvent), jobID uint64, assistantName string, event AIEvent) {
	if emit == nil {
		return
	}
	event.JobID = jobID
	event.Name = assistantName
	emit(event)
}

// formatModelDisplayName turns an OpenRouter model id (e.g. "meta-llama/llama-3.1-8b-instruct:free")
// into a short display label (last path segment after '/').
func formatModelDisplayName(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return openRouterName
	}
	if i := strings.LastIndex(model, "/"); i >= 0 && i+1 < len(model) {
		return model[i+1:]
	}
	return model
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

// closeBodyOnCancel starts a goroutine that closes body when ctx is cancelled.
// The returned function must be called when the body is no longer needed (e.g. defer) to stop the goroutine.
func closeBodyOnCancel(ctx context.Context, body io.ReadCloser) (stop func()) {
	ch := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = body.Close()
		case <-ch:
		}
	}()
	return func() { close(ch) }
}

func emitOpenRouterNotConfigured(emit func(AIEvent), jobID uint64, assistantName string) {
	emitAI(emit, jobID, assistantName, AIEvent{
		Type: "ai_replace",
		Text: assistantName + ": [OpenRouter not configured]",
	})
	emitAI(emit, jobID, assistantName, AIEvent{Type: "ai_end"})
}

func handleOpenRouterRequestError(ctx context.Context, jobID uint64, err error, emit func(AIEvent), assistantName string) (cancelled bool) {
	if errors.Is(ctx.Err(), context.Canceled) {
		log.Printf("openrouter: job=%d cancelled before response", jobID)
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
func handleOpenRouterHTTPError(ctx context.Context, jobID uint64, resp *http.Response, emit func(AIEvent), assistantName string) (cancelled bool) {
	if errors.Is(ctx.Err(), context.Canceled) {
		log.Printf("openrouter: job=%d cancelled with error status", jobID)
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

// openRouterStreamState holds mutable state while parsing the SSE stream.
type openRouterStreamState struct {
	ctx     context.Context
	jobID   uint64
	emit    func(AIEvent)
	started time.Time

	assistantName string

	buf                    strings.Builder
	aiBubbleStarted        bool
	sentFirstNonWhitespace bool
	firstDeltaLogged       bool
	latestUsage            openRouterUsage
	logDeltaStream         bool
	logDeltaStreamStarted  bool
}

func (s *openRouterStreamState) endDeltaStreamLogLine() {
	if s.logDeltaStreamStarted {
		_, _ = fmt.Fprint(os.Stdout, "\n")
	}
}

func (s *openRouterStreamState) ensureDeltaStreamPrefix() {
	if !s.logDeltaStreamStarted {
		s.logDeltaStreamStarted = true
		_, _ = fmt.Fprintf(os.Stdout, "openrouter: job=%d delta_stream=", s.jobID)
	}
}

// startAIBubble emits ai_start once when the assistant message bubble begins.
func (s *openRouterStreamState) startAIBubble() {
	if s.aiBubbleStarted {
		return
	}
	emitAI(s.emit, s.jobID, s.assistantName, AIEvent{
		Type: "ai_start",
		Text: s.assistantName + ": ",
	})
	s.aiBubbleStarted = true
}

// maybeStartBubbleForChunk starts the bubble when this SSE chunk carries model info or choices,
// so the UI can show the header before the first token.
func (s *openRouterStreamState) maybeStartBubbleForChunk(chunk *openRouterStreamResponse) {
	if len(chunk.Choices) == 0 && strings.TrimSpace(chunk.Model) == "" {
		return
	}
	s.startAIBubble()
}

func (s *openRouterStreamState) applyChunkMetadata(chunk *openRouterStreamResponse) {
	if chunk.Model != "" {
		s.assistantName = formatModelDisplayName(chunk.Model)
	}
	if chunk.Usage.hasAny() {
		s.latestUsage = chunk.Usage
	}
}

func (s *openRouterStreamState) unmarshalSSEData(data []byte) (openRouterStreamResponse, error) {
	var chunk openRouterStreamResponse
	if err := json.Unmarshal(data, &chunk); err != nil {
		emitAI(s.emit, s.jobID, s.assistantName, AIEvent{
			Type: "ai_replace",
			Text: s.assistantName + ": [invalid response]",
		})
		return chunk, err
	}
	return chunk, nil
}

func (s *openRouterStreamState) emitReasoningDelta(reasoningPart string) {
	if reasoningPart == "" {
		return
	}
	if s.logDeltaStream {
		s.ensureDeltaStreamPrefix()
		_, _ = fmt.Fprint(os.Stdout, reasoningPart)
	} else {
		log.Printf("openrouter: job=%d reasoning_delta=%q", s.jobID, strings.TrimSpace(reasoningPart))
	}
	emitAI(s.emit, s.jobID, s.assistantName, AIEvent{
		Type: "ai_reasoning_delta",
		Text: reasoningPart,
	})
}

// processContentDelta handles one content delta: leading-whitespace trim for the first visible
// token, optional delta logging, buffer append, and ai_delta emission.
func (s *openRouterStreamState) processContentDelta(deltaText string) {
	if deltaText == "" {
		return
	}
	s.startAIBubble()
	if !s.sentFirstNonWhitespace {
		deltaText = strings.TrimLeft(deltaText, " \n\t\r")
		if deltaText == "" {
			return
		}
		s.sentFirstNonWhitespace = true
	}
	if !s.firstDeltaLogged {
		s.firstDeltaLogged = true
		log.Printf("openrouter: job=%d first_delta_after=%s", s.jobID, time.Since(s.started).Truncate(time.Millisecond))
	}
	if s.logDeltaStream {
		s.ensureDeltaStreamPrefix()
		_, _ = fmt.Fprint(os.Stdout, deltaText)
	}
	s.buf.WriteString(deltaText)
	emitAI(s.emit, s.jobID, s.assistantName, AIEvent{
		Type: "ai_delta",
		Text: deltaText,
	})
}

func (s *openRouterStreamState) onSSEData(data []byte) error {
	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	default:
	}

	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "[DONE]" {
		return nil
	}

	chunk, err := s.unmarshalSSEData(data)
	if err != nil {
		return err
	}
	s.applyChunkMetadata(&chunk)
	s.maybeStartBubbleForChunk(&chunk)

	for _, choice := range chunk.Choices {
		reasoningPart := choice.Delta.Reasoning
		if reasoningPart == "" {
			reasoningPart = choice.Delta.ReasoningContent
		}
		s.emitReasoningDelta(reasoningPart)
		s.processContentDelta(choice.Delta.Content)
	}
	return nil
}

func handleOpenRouterStreamError(
	s *openRouterStreamState,
	streamErr error,
) (cancelled bool) {
	if errors.Is(streamErr, context.Canceled) || errors.Is(s.ctx.Err(), context.Canceled) {
		log.Printf("openrouter: job=%d cancelled during stream after=%s", s.jobID, time.Since(s.started).Truncate(time.Millisecond))
		s.endDeltaStreamLogLine()
		return true
	}
	log.Printf("openrouter: job=%d stream error after=%s: %v", s.jobID, time.Since(s.started).Truncate(time.Millisecond), streamErr)
	s.endDeltaStreamLogLine()
	emitAI(s.emit, s.jobID, s.assistantName, AIEvent{
		Type: "ai_replace",
		Text: s.assistantName + ": [stream error]",
	})
	emitAI(s.emit, s.jobID, s.assistantName, AIEvent{
		Type: "ai_end",
	})
	return false
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
) (assistantText string, cancelled bool) {
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
// When ctx is cancelled (client abort/stop), upstream OpenRouter consumption stops and cancelled is true.
func StreamOpenRouter(ctx context.Context, jobID uint64, userPrompt string, emit func(AIEvent)) (assistantText string, cancelled bool) {
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

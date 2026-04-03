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

// StreamOpenRouter streams one completion to emit and returns the full assistant text (if any).
// When ctx is cancelled (client abort/stop), upstream OpenRouter consumption stops and cancelled is true.
func StreamOpenRouter(ctx context.Context, jobID uint64, userPrompt string, emit func(AIEvent)) (assistantText string, cancelled bool) {
	assistantName := formatModelDisplayName(openRouterModel)

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		log.Println("OPENROUTER_API_KEY is not set; skipping OpenRouter call")
		emitAI(emit, jobID, assistantName, AIEvent{
			Type: "ai_replace",
			Text: assistantName + ": [OpenRouter not configured]",
		})
		emitAI(emit, jobID, assistantName, AIEvent{Type: "ai_end"})
		return "", false
	}

	if strings.TrimSpace(userPrompt) == "" {
		return "", false
	}

	started := time.Now()
	log.Printf("openrouter: job=%d prompt_bytes=%d", jobID, len(userPrompt))

	requestBody := newOpenRouterRequest(userPrompt)

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		log.Printf("failed to marshal OpenRouter request: %v", err)
		return "", false
	}

	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openRouterURL, bytes.NewReader(jsonBody))
	if err != nil {
		log.Printf("failed to create OpenRouter request: %v", err)
		return "", false
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := openRouterHTTPClient.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			log.Printf("openrouter: job=%d cancelled before response", jobID)
			return "", true
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
		return "", false
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		if errors.Is(ctx.Err(), context.Canceled) {
			log.Printf("openrouter: job=%d cancelled with error status", jobID)
			return "", true
		}
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			emitAI(emit, jobID, assistantName, AIEvent{
				Type: "ai_replace",
				Text: assistantName + ": [invalid response]",
			})
			return "", false
		}

		var payload openRouterResponse
		if err := json.Unmarshal(bodyBytes, &payload); err == nil && payload.Error.Message != "" {
			emitAI(emit, jobID, assistantName, AIEvent{
				Type: "ai_replace",
				Text: assistantName + ": [api error: " + payload.Error.Message + "]",
			})
			return "", false
		}

		emitAI(emit, jobID, assistantName, AIEvent{
			Type: "ai_replace",
			Text: assistantName + ": [api error]",
		})
		return "", false
	}

	var buf strings.Builder
	var aiBubbleStarted bool
	sentFirstNonWhitespace := false
	firstDeltaLogged := false
	var latestUsage openRouterUsage
	logDeltaStream := openRouterLogDeltaStreamEnabled()
	logDeltaStreamStarted := false

	stopBody := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = resp.Body.Close()
		case <-stopBody:
		}
	}()
	defer close(stopBody)

	streamErr := parseSSEStream(resp, func(data []byte) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		trimmed := strings.TrimSpace(string(data))
		if trimmed == "" {
			return nil
		}
		if trimmed == "[DONE]" {
			return nil
		}

		var chunk openRouterStreamResponse
		if err := json.Unmarshal(data, &chunk); err != nil {
			emitAI(emit, jobID, assistantName, AIEvent{
				Type: "ai_replace",
				Text: assistantName + ": [invalid response]",
			})
			return err
		}
		if chunk.Model != "" {
			assistantName = formatModelDisplayName(chunk.Model)
		}
		if chunk.Usage.hasAny() {
			latestUsage = chunk.Usage
		}

		if !aiBubbleStarted && (len(chunk.Choices) > 0 || strings.TrimSpace(chunk.Model) != "") {
			emitAI(emit, jobID, assistantName, AIEvent{
				Type: "ai_start",
				Text: assistantName + ": ",
			})
			aiBubbleStarted = true
		}

		for _, choice := range chunk.Choices {
			if logDeltaStream {
				if !logDeltaStreamStarted {
					logDeltaStreamStarted = true
					_, _ = fmt.Fprintf(os.Stdout, "openrouter: job=%d delta_stream=", jobID)
				}
				if choice.Delta.Reasoning != "" {
					_, _ = fmt.Fprint(os.Stdout, choice.Delta.Reasoning)
				} else if choice.Delta.ReasoningContent != "" {
					_, _ = fmt.Fprint(os.Stdout, choice.Delta.ReasoningContent)
				}
			} else {
				reasoningText := strings.TrimSpace(choice.Delta.Reasoning)
				if reasoningText == "" {
					reasoningText = strings.TrimSpace(choice.Delta.ReasoningContent)
				}
				if reasoningText != "" {
					log.Printf("openrouter: job=%d reasoning_delta=%q", jobID, reasoningText)
				}
			}

			deltaText := choice.Delta.Content
			if deltaText == "" {
				continue
			}

			if !aiBubbleStarted {
				emitAI(emit, jobID, assistantName, AIEvent{
					Type: "ai_start",
					Text: assistantName + ": ",
				})
				aiBubbleStarted = true
			}

			if !sentFirstNonWhitespace {
				deltaText = strings.TrimLeft(deltaText, " \n\t\r")
				if deltaText == "" {
					continue
				}
				sentFirstNonWhitespace = true
			}

			if !firstDeltaLogged {
				firstDeltaLogged = true
				log.Printf("openrouter: job=%d first_delta_after=%s", jobID, time.Since(started).Truncate(time.Millisecond))
			}

			if logDeltaStream {
				if !logDeltaStreamStarted {
					logDeltaStreamStarted = true
					_, _ = fmt.Fprintf(os.Stdout, "openrouter: job=%d delta_stream=", jobID)
				}
				_, _ = fmt.Fprint(os.Stdout, deltaText)
			}

			buf.WriteString(deltaText)
			emitAI(emit, jobID, assistantName, AIEvent{
				Type: "ai_delta",
				Text: deltaText,
			})
		}
		return nil
	})
	if streamErr != nil {
		if errors.Is(streamErr, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			log.Printf("openrouter: job=%d cancelled during stream after=%s", jobID, time.Since(started).Truncate(time.Millisecond))
			if logDeltaStreamStarted {
				_, _ = fmt.Fprint(os.Stdout, "\n")
			}
			return "", true
		}
		log.Printf("openrouter: job=%d stream error after=%s: %v", jobID, time.Since(started).Truncate(time.Millisecond), streamErr)
		if logDeltaStreamStarted {
			_, _ = fmt.Fprint(os.Stdout, "\n")
		}
		emitAI(emit, jobID, assistantName, AIEvent{
			Type: "ai_replace",
			Text: assistantName + ": [stream error]",
		})
		emitAI(emit, jobID, assistantName, AIEvent{
			Type: "ai_end",
		})
		return "", false
	}

	if strings.TrimSpace(buf.String()) == "" {
		if logDeltaStreamStarted {
			_, _ = fmt.Fprint(os.Stdout, "\n")
		}
		emitAI(emit, jobID, assistantName, AIEvent{
			Type: "ai_replace",
			Text: assistantName + ": [empty response]",
		})
		emitAI(emit, jobID, assistantName, AIEvent{
			Type: "ai_end",
		})
		return "", false
	}

	assistantText = buf.String()
	if logDeltaStreamStarted {
		_, _ = fmt.Fprint(os.Stdout, "\n")
	}
	if latestUsage.hasAny() {
		log.Printf(
			"openrouter: job=%d usage prompt_tokens=%d completion_tokens=%d total_tokens=%d",
			jobID,
			latestUsage.PromptTokens,
			latestUsage.CompletionTokens,
			latestUsage.TotalTokens,
		)

	}
	log.Printf("openrouter: job=%d done after=%s response=%q", jobID, time.Since(started).Truncate(time.Millisecond), assistantText)
	emitAI(emit, jobID, assistantName, AIEvent{
		Type: "ai_end",
	})
	return assistantText, false
}

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

type wsAIEvent struct {
	Type  string `json:"type"`
	JobID uint64 `json:"jobId"`
	Text  string `json:"text,omitempty"`
}

var openRouterHTTPClient = &http.Client{
	// Give streaming enough time to complete.
	Timeout: 90 * time.Second,
}

func openRouterLogDeltaStreamEnabled() bool {
	v := strings.TrimSpace(os.Getenv("OPENROUTER_LOG_DELTA_STREAM"))
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}

// AiClient receives user messages from the hub and runs each OpenRouter
// request in its own goroutine so multiple clients can be in flight at once.
// Assistant replies are sent on reply for the hub to deliver to the right
// connection.
type AiClient struct {
	jobs  <-chan aiJob
	reply chan<- aiReply
	done  chan<- aiResult
}

// NewAiClient wires an AiClient to the hub's AI job queue and reply channel.
func NewAiClient(h *Hub) *AiClient {
	return &AiClient{
		jobs:  h.aiJobs,
		reply: h.replies,
		done:  h.results,
	}
}

// Run blocks until jobs is closed. Each job is processed concurrently.
func (a *AiClient) Run() {
	for job := range a.jobs {
		go a.forward(job)
	}
}

func shouldForwardToAI(message []byte) bool {
	prefix := []byte(openRouterName + ": ")
	return !bytes.HasPrefix(message, prefix)
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

func extractPrompt(message []byte) string {
	// Incoming messages are in the form "ip:port: text".
	parts := strings.SplitN(string(message), ": ", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return string(message)
}

func extractAssistantContent(payload openRouterResponse) string {
	for _, choice := range payload.Choices {
		if choice.Message.Role != "assistant" {
			continue
		}
		if strings.TrimSpace(choice.Message.Content) != "" {
			return choice.Message.Content
		}
	}
	return ""
}

func sendAIEvent(a *AiClient, job aiJob, event wsAIEvent) {
	evtBytes, err := json.Marshal(event)
	if err != nil {
		log.Printf("failed to marshal ws AI event: %v", err)
		return
	}
	a.reply <- aiReply{client: job.client, jobID: job.id, body: evtBytes}
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

func (a *AiClient) forward(job aiJob) {
	// Always report completion so the hub can unblock input.
	var assistantText string
	cancelled := false
	defer func() {
		a.done <- aiResult{
			client:        job.client,
			jobID:         job.id,
			assistantText: assistantText,
			cancelled:     cancelled,
		}
	}()

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		log.Println("OPENROUTER_API_KEY is not set; skipping OpenRouter call")
		return
	}

	// job.body is already the full prompt text we want to send.
	// (We only use extractPrompt for raw inbound websocket messages that include
	// an "ip:port: " prefix.)
	userPrompt := string(job.body)
	if strings.TrimSpace(userPrompt) == "" {
		return
	}

	started := time.Now()
	log.Printf("openrouter: job=%d prompt_bytes=%d", job.id, len(userPrompt))

	requestBody := newOpenRouterRequest(userPrompt)

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		log.Printf("failed to marshal OpenRouter request: %v", err)
		return
	}

	ctx := job.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openRouterURL, bytes.NewReader(jsonBody))
	if err != nil {
		log.Printf("failed to create OpenRouter request: %v", err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := openRouterHTTPClient.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			cancelled = true
			log.Printf("openrouter: job=%d cancelled before response", job.id)
			return
		}
		log.Printf("OpenRouter request failed: %v", err)
		if isOpenRouterTimeout(err) {
			sendAIEvent(a, job, wsAIEvent{
				Type:  "ai_replace",
				JobID: job.id,
				Text:  openRouterName + ": " + openRouterBusyUserMsg,
			})
		} else {
			sendAIEvent(a, job, wsAIEvent{
				Type:  "ai_replace",
				JobID: job.id,
				Text:  openRouterName + ": [request failed]",
			})
		}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		if errors.Is(ctx.Err(), context.Canceled) {
			cancelled = true
			log.Printf("openrouter: job=%d cancelled with error status", job.id)
			return
		}
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			sendAIEvent(a, job, wsAIEvent{
				Type:  "ai_replace",
				JobID: job.id,
				Text:  openRouterName + ": [invalid response]",
			})
			return
		}

		var payload openRouterResponse
		if err := json.Unmarshal(bodyBytes, &payload); err == nil && payload.Error.Message != "" {
			sendAIEvent(a, job, wsAIEvent{
				Type:  "ai_replace",
				JobID: job.id,
				Text:  openRouterName + ": [api error: " + payload.Error.Message + "]",
			})
			return
		}

		sendAIEvent(a, job, wsAIEvent{
			Type:  "ai_replace",
			JobID: job.id,
			Text:  openRouterName + ": [api error]",
		})
		return
	}

	// Start streaming: prefix a bubble, append deltas, then end.
	sendAIEvent(a, job, wsAIEvent{
		Type:  "ai_start",
		JobID: job.id,
		Text:  openRouterName + ": ",
	})

	var buf strings.Builder
	sentFirstNonWhitespace := false
	firstDeltaLogged := false
	var latestUsage openRouterUsage
	logDeltaStream := openRouterLogDeltaStreamEnabled()
	logDeltaStreamStarted := false

	// Ensure a user "stop" cancels any blocked stream reads immediately.
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
			sendAIEvent(a, job, wsAIEvent{
				Type:  "ai_replace",
				JobID: job.id,
				Text:  openRouterName + ": [invalid response]",
			})
			return err
		}
		if chunk.Usage.hasAny() {
			latestUsage = chunk.Usage
		}

		for _, choice := range chunk.Choices {
			if logDeltaStream {
				// For debugging: print a contiguous stream of deltas with no newline
				// between chunks. This avoids "token soup" formatting from line-based logs.
				// We still keep the structured timing/usage logs via log.Printf.
				if !logDeltaStreamStarted {
					logDeltaStreamStarted = true
					_, _ = fmt.Fprintf(os.Stdout, "openrouter: job=%d delta_stream=", job.id)
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
					log.Printf("openrouter: job=%d reasoning_delta=%q", job.id, reasoningText)
				}
			}

			deltaText := choice.Delta.Content
			if deltaText == "" {
				continue
			}

			// Don't send leading whitespace until the first non-whitespace character.
			if !sentFirstNonWhitespace {
				deltaText = strings.TrimLeft(deltaText, " \n\t\r")
				if deltaText == "" {
					continue
				}
				sentFirstNonWhitespace = true
			}

			if !firstDeltaLogged {
				firstDeltaLogged = true
				log.Printf("openrouter: job=%d first_delta_after=%s", job.id, time.Since(started).Truncate(time.Millisecond))
			}

			if logDeltaStream {
				if !logDeltaStreamStarted {
					logDeltaStreamStarted = true
					_, _ = fmt.Fprintf(os.Stdout, "openrouter: job=%d delta_stream=", job.id)
				}
				_, _ = fmt.Fprint(os.Stdout, deltaText)
			}

			buf.WriteString(deltaText)
			sendAIEvent(a, job, wsAIEvent{
				Type:  "ai_delta",
				JobID: job.id,
				Text:  deltaText,
			})
		}
		return nil
	})
	if streamErr != nil {
		if errors.Is(streamErr, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			cancelled = true
			log.Printf("openrouter: job=%d cancelled during stream after=%s", job.id, time.Since(started).Truncate(time.Millisecond))
			if logDeltaStreamStarted {
				_, _ = fmt.Fprint(os.Stdout, "\n")
			}
			return
		}
		// `ai_replace` was already emitted for decode failures.
		log.Printf("openrouter: job=%d stream error after=%s: %v", job.id, time.Since(started).Truncate(time.Millisecond), streamErr)
		if logDeltaStreamStarted {
			_, _ = fmt.Fprint(os.Stdout, "\n")
		}
		// Ensure the UI isn't left with a hanging "in progress" bubble.
		sendAIEvent(a, job, wsAIEvent{
			Type:  "ai_replace",
			JobID: job.id,
			Text:  openRouterName + ": [stream error]",
		})
		sendAIEvent(a, job, wsAIEvent{
			Type:  "ai_end",
			JobID: job.id,
		})
		return
	}

	// If we never streamed any non-whitespace content, replace the prefix bubble.
	if strings.TrimSpace(buf.String()) == "" {
		if logDeltaStreamStarted {
			_, _ = fmt.Fprint(os.Stdout, "\n")
		}
		sendAIEvent(a, job, wsAIEvent{
			Type:  "ai_replace",
			JobID: job.id,
			Text:  openRouterName + ": [empty response]",
		})
		// Ensure the frontend restores its send button state.
		sendAIEvent(a, job, wsAIEvent{
			Type:  "ai_end",
			JobID: job.id,
		})
		return
	}

	assistantText = buf.String()
	if logDeltaStreamStarted {
		_, _ = fmt.Fprint(os.Stdout, "\n")
	}
	if latestUsage.hasAny() {
		log.Printf(
			"openrouter: job=%d usage prompt_tokens=%d completion_tokens=%d total_tokens=%d",
			job.id,
			latestUsage.PromptTokens,
			latestUsage.CompletionTokens,
			latestUsage.TotalTokens,
		)

	}
	log.Printf("openrouter: job=%d done after=%s response=%q", job.id, time.Since(started).Truncate(time.Millisecond), assistantText)
	sendAIEvent(a, job, wsAIEvent{
		Type:  "ai_end",
		JobID: job.id,
	})
}

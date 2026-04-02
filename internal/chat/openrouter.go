package chat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	Model    string              `json:"model"`
	Messages []openRouterMessage `json:"messages"`
	Stream   bool                `json:"stream,omitempty"`
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
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

type openRouterStreamChoice struct {
	Delta openRouterStreamDelta `json:"delta"`
}

type openRouterStreamResponse struct {
	Choices []openRouterStreamChoice `json:"choices"`
	Error   openRouterError          `json:"error"`
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

// AiClient receives user messages from the hub and runs each OpenRouter
// request in its own goroutine so multiple clients can be in flight at once.
// Assistant replies are sent on reply for the hub to deliver to the right
// connection.
type AiClient struct {
	jobs  <-chan aiJob
	reply chan<- aiReply
}

// NewAiClient wires an AiClient to the hub's AI job queue and reply channel.
func NewAiClient(h *Hub) *AiClient {
	return &AiClient{
		jobs:  h.aiJobs,
		reply: h.replies,
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
		Stream: true,
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
	a.reply <- aiReply{client: job.client, body: evtBytes}
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

		if strings.HasPrefix(line, "data:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
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
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		log.Println("OPENROUTER_API_KEY is not set; skipping OpenRouter call")
		return
	}

	userPrompt := extractPrompt(job.body)
	if strings.TrimSpace(userPrompt) == "" {
		return
	}

	requestBody := newOpenRouterRequest(userPrompt)

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		log.Printf("failed to marshal OpenRouter request: %v", err)
		return
	}

	req, err := http.NewRequest(http.MethodPost, openRouterURL, bytes.NewReader(jsonBody))
	if err != nil {
		log.Printf("failed to create OpenRouter request: %v", err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := openRouterHTTPClient.Do(req)
	if err != nil {
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

	streamErr := parseSSEStream(resp, func(data []byte) error {
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

		for _, choice := range chunk.Choices {
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
		// `ai_replace` was already emitted for decode failures.
		return
	}

	// If we never streamed any non-whitespace content, replace the prefix bubble.
	if strings.TrimSpace(buf.String()) == "" {
		sendAIEvent(a, job, wsAIEvent{
			Type:  "ai_replace",
			JobID: job.id,
			Text:  openRouterName + ": [empty response]",
		})
		return
	}

	sendAIEvent(a, job, wsAIEvent{
		Type:  "ai_end",
		JobID: job.id,
	})
}

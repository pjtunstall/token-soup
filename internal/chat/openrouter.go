package chat

import (
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

var openRouterHTTPClient = &http.Client{
	Timeout: 20 * time.Second,
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
			{
				Role:    "user",
				Content: userPrompt,
			},
		},
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
			a.reply <- aiReply{client: job.client, body: []byte(openRouterName + ": " + openRouterBusyUserMsg)}
		} else {
			a.reply <- aiReply{client: job.client, body: []byte(openRouterName + ": [request failed]")}
		}
		return
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("failed to read OpenRouter response body: %v", err)
		if isOpenRouterTimeout(err) {
			a.reply <- aiReply{client: job.client, body: []byte(openRouterName + ": " + openRouterBusyUserMsg)}
		} else {
			a.reply <- aiReply{client: job.client, body: []byte(openRouterName + ": [invalid response]")}
		}
		return
	}

	log.Printf("OpenRouter response JSON: %s", bodyBytes)

	var payload openRouterResponse
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		log.Printf("failed to decode OpenRouter response: %v", err)
		a.reply <- aiReply{client: job.client, body: []byte(openRouterName + ": [invalid response]")}
		return
	}

	if resp.StatusCode >= 400 {
		if payload.Error.Message != "" {
			log.Printf("OpenRouter API error (%d): %s", resp.StatusCode, payload.Error.Message)
			a.reply <- aiReply{client: job.client, body: []byte(openRouterName + ": [api error: " + payload.Error.Message + "]")}
			return
		}
		log.Printf("OpenRouter API error (%d)", resp.StatusCode)
		a.reply <- aiReply{client: job.client, body: []byte(openRouterName + ": [api error]")}
		return
	}

	content := extractAssistantContent(payload)
	if strings.TrimSpace(content) == "" {
		log.Println("OpenRouter returned no assistant text")
		a.reply <- aiReply{client: job.client, body: []byte(openRouterName + ": [empty response]")}
		return
	}

	reply := []byte(openRouterName + ": " + strings.TrimSpace(content))
	a.reply <- aiReply{client: job.client, body: reply}
}

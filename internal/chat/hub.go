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

type Hub struct {
	clients map[*Client]struct{}

	// "Inbound messages from the clients." Inbound? Well, this is the channel
	// on which the hub receives messages from each instance of the Client
	// struct, which receives the message from the corresponding user. The hub
	// then broadcasts the message by sending it on each client's send channel.
	// So it's broadcast in the sense that when the message is on the broadcast
	// channel it's still destined for all users; when it's on a client.send
	// channel, it's then being sent to an individual rather than being
	// broadcast, although it is being broadcast, when you consider that it's
	// also being sent on the other client.send channels.
	broadcast chan []byte

	register chan *Client
	unregister chan *Client
}

func NewHub() *Hub {
	return &Hub{
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]struct{}),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = struct{}{}
		case client := <-h.unregister:
			// Compare Rust `if let Some(_) = h.clients.get(client) {`.
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
		case message := <-h.broadcast:
			if shouldForwardToOpenRouter(message) {
				// Keep request/response latency off the hub loop.
				go h.forwardToOpenRouter(message)
			}

			for client := range h.clients {
				select {
				case client.send <- message:
					// Default runs when there exists a client that we can't
					// send to without blocking. This happens when their buffer
					// is full: 256 messages are queued and writePump is not
					// draining it fast enough. So deafult means; "This client
					// is too slow; don't stall the while hub; drop them."
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
		}
	}
}

func shouldForwardToOpenRouter(message []byte) bool {
	prefix := []byte(openRouterName + ": ")
	return !bytes.HasPrefix(message, prefix)
}

func (h *Hub) forwardToOpenRouter(message []byte) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		log.Println("OPENROUTER_API_KEY is not set; skipping OpenRouter call")
		return
	}

	userPrompt := extractPrompt(message)
	if strings.TrimSpace(userPrompt) == "" {
		return
	}

	requestBody := openRouterRequest{
		Model: openRouterModel,
		Messages: []openRouterMessage{
			{
				Role:    "user",
				Content: userPrompt,
			},
		},
	}

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
			h.broadcast <- []byte(openRouterName + ": " + openRouterBusyUserMsg)
		} else {
			h.broadcast <- []byte(openRouterName + ": [request failed]")
		}
		return
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("failed to read OpenRouter response body: %v", err)
		if isOpenRouterTimeout(err) {
			h.broadcast <- []byte(openRouterName + ": " + openRouterBusyUserMsg)
		} else {
			h.broadcast <- []byte(openRouterName + ": [invalid response]")
		}
		return
	}

	log.Printf("OpenRouter response JSON: %s", bodyBytes)

	var payload openRouterResponse
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		log.Printf("failed to decode OpenRouter response: %v", err)
		h.broadcast <- []byte(openRouterName + ": [invalid response]")
		return
	}

	if resp.StatusCode >= 400 {
		if payload.Error.Message != "" {
			log.Printf("OpenRouter API error (%d): %s", resp.StatusCode, payload.Error.Message)
			h.broadcast <- []byte(openRouterName + ": [api error: " + payload.Error.Message + "]")
			return
		}
		log.Printf("OpenRouter API error (%d)", resp.StatusCode)
		h.broadcast <- []byte(openRouterName + ": [api error]")
		return
	}

	content := extractAssistantContent(payload)
	if strings.TrimSpace(content) == "" {
		log.Println("OpenRouter returned no assistant text")
		h.broadcast <- []byte(openRouterName + ": [empty response]")
		return
	}

	reply := []byte(openRouterName + ": " + strings.TrimSpace(content))
	h.broadcast <- reply
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
	// Response shape: choices[] -> message.
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
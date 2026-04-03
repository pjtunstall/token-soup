package chat

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	"github.com/lestrrat-go/jwx/v2/jwk"
)

// userInbound is a message from one websocket client.
type userInbound struct {
	client *Client
	body   []byte
}

// aiReply is an assistant message for a single connection.
type aiReply struct {
	client *Client
	jobID  uint64
	body   []byte
}

// aiJob is one user message queued for OpenRouter (handled concurrently).
type aiJob struct {
	id     uint64
	client *Client
	body   []byte

	ctx context.Context
}

// aiResult reports job completion so the hub can update state/history.
type aiResult struct {
	client        *Client
	jobID         uint64
	assistantText string
	cancelled     bool
}

type Hub struct {
	clients map[*Client]struct{}

	inbound chan userInbound

	// Jobs for AiClient (OpenRouter). Buffered so the hub loop does not block
	// on AI enqueue when the network is slow.
	aiJobs chan aiJob

	// Assistant text from AiClient back to one client.
	replies chan aiReply

	// Job completion from AiClient back to hub.
	results chan aiResult

	register   chan *Client
	unregister chan *Client

	nextJobID uint64

	cache *jwk.Cache
	jwksURL string
}

func NewHub(cache *jwk.Cache, jwksURL string) *Hub {
	return &Hub{
		inbound:    make(chan userInbound),
		aiJobs:     make(chan aiJob, 64),
		replies:    make(chan aiReply, 64),
		results:    make(chan aiResult, 64),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]struct{}),
		nextJobID:  0,
		cache: cache,
		jwksURL: jwksURL,
	}
}

type clientInboundControl struct {
	Type string `json:"type"`
}

func isStopMessage(prompt string) bool {
	var ctl clientInboundControl
	if err := json.Unmarshal([]byte(prompt), &ctl); err != nil {
		return false
	}
	return strings.EqualFold(ctl.Type, "stop")
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = struct{}{}
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				if client.aiCancel != nil {
					client.aiCancel()
					client.aiCancel = nil
				}
				close(client.send)
			}
		case u := <-h.inbound:
			prompt := extractPrompt(u.body)

			if isStopMessage(prompt) {
				// Best-effort cancel. Unblock input immediately and ignore any late AI
				// events for the old job.
				if u.client.aiCancel != nil {
					// Replace any in-flight bubble with a cancellation message.
					sendHubAIEvent(u.client, wsAIEvent{
						Type:  "ai_replace",
						JobID: u.client.aiJobID,
						Text:  openRouterName + ": [cancelled]",
					})
					sendHubAIEvent(u.client, wsAIEvent{
						Type:  "ai_end",
						JobID: u.client.aiJobID,
					})

					u.client.aiCancel()
					u.client.aiCancel = nil
				}
				u.client.aiBusy = false
				u.client.aiJobID = 0
				continue
			}

			// Block user messages while an assistant response is in flight.
			if u.client.aiBusy {
				continue
			}

			select {
			case u.client.send <- u.body:
			default:
				close(u.client.send)
				delete(h.clients, u.client)
			}
			if shouldForwardToAI(u.body) {
				id := h.nextJobID
				h.nextJobID++
				ctx, cancel := context.WithCancel(context.Background())
				u.client.aiBusy = true
				u.client.aiJobID = id
				u.client.aiCancel = cancel

				trimmed := strings.TrimSpace(prompt)
				if trimmed != "" {
					// Store as plain text transcript.
					u.client.history = append(u.client.history, "User: "+trimmed)
				}
				// Concatenate history (including latest user message) into one prompt.
				joinedPrompt := strings.Join(u.client.history, "\n\n")

				select {
				case h.aiJobs <- aiJob{id: id, client: u.client, body: []byte(joinedPrompt), ctx: ctx}:
				default:
					log.Println("OpenRouter job queue full; dropping forward")
					u.client.aiBusy = false
					u.client.aiJobID = 0
					if u.client.aiCancel != nil {
						u.client.aiCancel()
						u.client.aiCancel = nil
					}
				}
			}
		case r := <-h.replies:
			if _, ok := h.clients[r.client]; !ok {
				continue
			}
			// Drop any late events after a cancellation or superseding job.
			if r.client.aiJobID != r.jobID {
				continue
			}
			select {
			case r.client.send <- r.body:
			default:
				close(r.client.send)
				delete(h.clients, r.client)
			}
		case res := <-h.results:
			if _, ok := h.clients[res.client]; !ok {
				continue
			}
			// Only apply results for the current in-flight job.
			if res.client.aiJobID != res.jobID {
				continue
			}

			res.client.aiBusy = false
			res.client.aiJobID = 0
			if res.client.aiCancel != nil {
				// Cancel context to ensure any lingering reads stop.
				res.client.aiCancel()
				res.client.aiCancel = nil
			}

			if !res.cancelled {
				if strings.TrimSpace(res.assistantText) != "" {
					res.client.history = append(res.client.history, "Assistant: "+strings.TrimSpace(res.assistantText))
				}
			}
		}
	}
}

func sendHubAIEvent(client *Client, event wsAIEvent) {
	evtBytes, err := json.Marshal(event)
	if err != nil {
		log.Printf("failed to marshal ws AI event: %v", err)
		return
	}
	select {
	case client.send <- evtBytes:
	default:
		close(client.send)
	}
}

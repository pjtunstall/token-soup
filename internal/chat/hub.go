package chat

import "log"

// userInbound is a message from one websocket client.
type userInbound struct {
	client *Client
	body   []byte
}

// aiReply is an assistant message for a single connection.
type aiReply struct {
	client *Client
	body   []byte
}

// aiJob is one user message queued for OpenRouter (handled concurrently).
type aiJob struct {
	client *Client
	body   []byte
}

type Hub struct {
	clients map[*Client]struct{}

	inbound chan userInbound

	// Jobs for AiClient (OpenRouter). Buffered so the hub loop does not block
	// on AI enqueue when the network is slow.
	aiJobs chan aiJob

	// Assistant text from AiClient back to one client.
	replies chan aiReply

	register   chan *Client
	unregister chan *Client
}

func NewHub() *Hub {
	return &Hub{
		inbound:    make(chan userInbound),
		aiJobs:     make(chan aiJob, 64),
		replies:    make(chan aiReply, 64),
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
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
		case u := <-h.inbound:
			select {
			case u.client.send <- u.body:
			default:
				close(u.client.send)
				delete(h.clients, u.client)
			}
			if shouldForwardToAI(u.body) {
				select {
				case h.aiJobs <- aiJob{client: u.client, body: u.body}:
				default:
					log.Println("OpenRouter job queue full; dropping forward")
				}
			}
		case r := <-h.replies:
			if _, ok := h.clients[r.client]; !ok {
				continue
			}
			select {
			case r.client.send <- r.body:
			default:
				close(r.client.send)
				delete(h.clients, r.client)
			}
		}
	}
}

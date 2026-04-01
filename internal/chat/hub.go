package chat

import "log"

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

	// User messages to forward to AiClient (OpenRouter). Buffered so the hub
	// loop does not block on AI enqueue when the network is slow.
	aiForward chan []byte

	register   chan *Client
	unregister chan *Client
}

func NewHub() *Hub {
	return &Hub{
		broadcast:  make(chan []byte),
		aiForward:  make(chan []byte, 64),
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
			if shouldForwardMessage(message) {
				select {
				case h.aiForward <- message:
				default:
					log.Println("OpenRouter job queue full; dropping forward")
				}
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

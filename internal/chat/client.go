package chat

import (
	"bytes"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 8192
)

var (
	newline = []byte{'\n'}
	space   = []byte{' '}
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	hub *Hub
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	send chan []byte // Buffer: 256.
}

// readPump pumps messages from the websocket connection to the hub.
//
// The application runs readPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)

	// We should have a pong to read before the next pongwait has expired, given
	// that we want pings at regular intervals in writePump: `c.conn.WriteMessage(websocket.PingMessage, nil)`.
	c.conn.SetReadDeadline(time.Now().Add(pongWait))

	pongHandler := func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil }
	c.conn.SetPongHandler(pongHandler)
	
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			// This function is variadic. We pass the errors we don't want to log. (CloseAbnormalClosure names a WebSocket close code, not
			// “unexpected” in this API’s sense. Putting it in the list means
			// "don’t log when the peer disappears uncleanly"--common for tabs
			// closed, network drops, etc.)
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
		message = bytes.TrimSpace(bytes.ReplaceAll(message, newline, space))
		prefix := []byte(c.conn.RemoteAddr().String() + ": ")
		message = append(prefix, message...)
		c.hub.broadcast <- message
	}
}

// writePump pumps messages from the hub to the websocket connection.
//
// A goroutine running writePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (c *Client) writePump() {
	// ticker has a channel, ticker.C, with a 1-element time buffer. (According
	// to the docs, if the client falls behind while reading, the ticker drops
	// ticks on the floor until the client catches up.)
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		// As of Go 1.23, the ticker doesn't need to be explicitly stopped for
		// it to be garbage collected, but still good to clean up here so it's not needlessly firing.
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Add queued chat messages to the current WebSocket message.
			n := len(c.send)
			for range n {
				w.Write(newline)
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}
			// 
		case <-ticker.C:
			// Every tick, try to send a ping on the socket. If it doesn't
			// succeed within the writeWait period, return from this function,
			// triggering defer to close the connection.
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// serveWs handles websocket requests from the peer.
func ServeWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	client := &Client{hub: hub, conn: conn, send: make(chan []byte, 256)}
	client.hub.register <- client

	// "Allow collection of memory referenced by the caller by doing all work in
	// new goroutines." I.e. this lets the handler return and saves us from keeping the request/handler frame and related temporaries alive for the whole WebSocket lifetime.
	go client.writePump()
	go client.readPump()
}
package ezsock

import (
	"encoding/json"
	"time"

	"github.com/gorilla/websocket"
)

type Client struct {
	id          uint64
	conn        *websocket.Conn
	server      *Server
	sender      chan []byte
	rooms       map[string]*room
	msgHandlers map[string]messageHandler
}

func (client *Client) readLoop() {
	client.conn.SetReadDeadline(time.Now().Add(time.Duration(client.server.config.PongTimeout) * time.Millisecond))
	client.conn.SetPongHandler(func(string) error {
		client.conn.SetReadDeadline(time.Now().Add(time.Duration(client.server.config.PongTimeout) * time.Millisecond))
		return nil
	})

	defer client.disconnected()

	for {
		t, data, err := client.conn.ReadMessage()
		if err != nil {
			return
		}

		if t == websocket.BinaryMessage && data[0] == 1 {
			var event string
			var sep int
			for sep = 1; sep < len(data); sep++ {
				if data[sep] == 0 {
					event = string(data[1:sep])
					break
				}
			}

			if handler, ok := client.msgHandlers[event]; ok {
				json.Unmarshal(data[sep+1:], handler.form)
				handler.callback(handler.form)
			}
		}
	}
}

func (client *Client) writeLoop() {
	pingTicker := time.NewTicker(time.Duration(client.server.config.PingInterval) * time.Millisecond)
	for {
		select {
		case data, ok := <-client.sender:
			if !ok {
				return
			}

			if err := client.conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
				return
			}

		case <-pingTicker.C:
			if err := client.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (client *Client) disconnected() {
	client.server.lock.Lock()
	defer client.server.lock.Unlock()

	close(client.sender)

	for _, r := range client.rooms {
		client.unsubscribe(r)
	}

	delete(client.server.clients, client.id)
}

func (client *Client) send(data []byte) {
	client.sender <- data
}

func (client *Client) Emit(event string, data interface{}) error {
	blob, err := NewEmitMessage(event, data)
	if err != nil {
		return err
	}

	client.sender <- blob
	return nil
}

func (client *Client) On(event string, form interface{}, callback func(interface{})) {
	if _, ok := client.msgHandlers[event]; !ok {
		client.msgHandlers[event] = messageHandler{
			form:     form,
			callback: callback,
		}
	}
}

func (client *Client) Subscribe(roomName string) {
	client.server.lock.Lock()
	defer client.server.lock.Unlock()

	r, ok := client.server.rooms[roomName]
	if !ok {
		r = client.server.createRoom(roomName)
	}

	r.clients[client.id] = client
	client.rooms[roomName] = r
}

func (client *Client) Unsubscribe(roomName string) {
	client.server.lock.Lock()
	defer client.server.lock.Unlock()

	if r, ok := client.rooms[roomName]; ok {
		client.unsubscribe(r)
	}
}

func (client *Client) unsubscribe(r *room) {
	delete(r.clients, client.id)
	delete(client.rooms, r.name)

	if len(r.clients) == 0 {
		r.close()
	}
}

type messageHandler struct {
	form     interface{}
	callback func(interface{})
}

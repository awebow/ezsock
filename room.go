package ezsock

import (
	"context"
	"encoding/base64"

	"github.com/go-redis/redis/v8"
)

type room struct {
	server  *Server
	name    string
	clients map[uint64]*Client
	pubsub  *redis.PubSub
}

func (server *Server) createRoom(name string) (r *room) {
	r = &room{
		server:  server,
		name:    name,
		clients: make(map[uint64]*Client),
	}

	server.rooms[name] = r
	if server.clustered() {
		r.pubsub = server.redis.Subscribe(context.Background(), r.name)
		go func() {
			for m := range r.pubsub.Channel() {
				if m.Payload == "close" {
					server.lock.Lock()
					r.close()
					server.lock.Unlock()
				} else if len(m.Payload) >= 10 && m.Payload[:10] == "broadcast " {
					data, _ := base64.StdEncoding.DecodeString(m.Payload[10:])

					server.lock.RLock()
					r.broadcast(data)
					server.lock.RUnlock()
				}
			}
		}()
	}

	return
}

func (r *room) close() {
	for _, c := range r.clients {
		delete(c.rooms, r.name)
	}

	delete(r.server.rooms, r.name)

	if r.server.clustered() {
		r.pubsub.Unsubscribe(context.Background(), r.name)
		r.pubsub.Close()
	}
}

func (r *room) broadcast(data []byte) {
	for _, c := range r.clients {
		go c.send(data)
	}
}

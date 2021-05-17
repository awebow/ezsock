package ezsock

import (
	"context"
	"encoding/base64"
	"net/http"
	"sync"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
)

type Server struct {
	cumulative uint64
	clients    map[uint64]*Client
	rooms      map[string]*room
	lock       *sync.RWMutex
	upgrader   websocket.Upgrader
	config     Config
	redis      *redis.Client
}

func NewServer(config Config) (server *Server) {
	server = &Server{
		clients:  make(map[uint64]*Client),
		rooms:    make(map[string]*room),
		lock:     new(sync.RWMutex),
		config:   config,
		upgrader: websocket.Upgrader{CheckOrigin: config.CheckOrigin},
	}

	if server.clustered() {
		server.redis = redis.NewClient(&redis.Options{
			Addr:     config.Redis.Addr,
			Password: config.Redis.Password,
			DB:       config.Redis.DB,
		})
	}

	return
}

func (server *Server) newClient(conn *websocket.Conn) (client *Client) {
	server.lock.Lock()
	defer server.lock.Unlock()

	client = &Client{
		id:          server.cumulative,
		sender:      make(chan []byte),
		rooms:       make(map[string]*room),
		conn:        conn,
		server:      server,
		msgHandlers: make(map[string]messageHandler),
	}

	server.clients[client.id] = client

	server.cumulative++

	return
}

func (server *Server) clustered() bool {
	return len(server.config.Redis.Addr) > 0
}

type Config struct {
	PingInterval int
	PongTimeout  int
	Redis        RedisConfig
	CheckOrigin  func(r *http.Request) bool
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

func (server *Server) Handle(w http.ResponseWriter, r *http.Request) (*Client, error) {
	conn, err := server.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, err
	}

	client := server.newClient(conn)
	go client.writeLoop()
	go client.readLoop()
	return client, nil
}

func (server *Server) Publish(roomName string, event string, data interface{}) error {
	blob, err := NewEmitMessage(event, data)
	if err != nil {
		return err
	}

	if server.clustered() {
		server.redis.Publish(context.Background(), roomName, "broadcast "+base64.StdEncoding.EncodeToString(blob))
	} else {
		server.lock.RLock()
		if r, ok := server.rooms[roomName]; ok {
			r.broadcast(blob)
		}
		server.lock.RUnlock()
	}
	return nil
}

func (server *Server) UnsubscribeAll(roomName string) {
	if server.clustered() {
		server.redis.Publish(context.Background(), roomName, "close")
	} else {
		server.lock.RLock()
		defer server.lock.RUnlock()

		if r, ok := server.rooms[roomName]; ok {
			r.close()
		}
	}
}

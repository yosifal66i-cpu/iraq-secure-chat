package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/iraq-secure-chat/common/config"
	"github.com/iraq-secure-chat/common/logging"
	"github.com/iraq-secure-chat/common/types"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

type WSGateway struct {
	cfg    *config.Config
	logger *zap.Logger
	redis  *redis.Client
	kafka  *kafka.Writer

	mu      sync.RWMutex
	clients map[string]map[string]*ClientConn // userID -> [connID -> conn]

	upgrader websocket.Upgrader
}

type ClientConn struct {
	UserID    string
	ConnID    string
	Conn      *websocket.Conn
	ChatSubs  map[string]bool
	mu        sync.Mutex
	done      chan struct{}
}

type callState struct {
	CallID    string
	FromID    string
	ToID      string
	CallType  string
	Status    string // initiated, accepted, rejected, ended
	SDPMap    map[string]string
	mu        sync.Mutex
}

var activeCalls = make(map[string]*callState)
var callMu sync.RWMutex

func main() {
	cfg := config.Load()
	logger := logging.Init(cfg.ServiceName, cfg.Environment, cfg.LogLevel)

	redisClient := redis.NewClient(&redis.Options{
		Addr:         cfg.Cache.RedisURL,
		Password:     cfg.Cache.RedisPassword,
		DB:           cfg.Cache.RedisDB,
		PoolSize:     cfg.Cache.PoolSize,
		MinIdleConns: cfg.Cache.MinIdleConns,
	})

	ctx := context.Background()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		logger.Fatal("redis connection failed", zap.Error(err))
	}

	kafkaWriter := &kafka.Writer{
		Addr:     kafka.TCP(cfg.Kafka.Brokers...),
		Topic:    cfg.Kafka.TopicPrefix + ".messages.inbound",
		Balancer: &kafka.Hash{},
		Async:    true,
	}

	gateway := &WSGateway{
		cfg:      cfg,
		logger:   logger,
		redis:    redisClient,
		kafka:    kafkaWriter,
		clients:  make(map[string]map[string]*ClientConn),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			CheckOrigin:     func(r *http.Request) bool { return true },
			EnableCompression: true,
		},
	}

	// Start Redis pub/sub listener for cross-node delivery
	go gateway.listenRedisPubSub(ctx)

	// Start heartbeat checker
	go gateway.heartbeatChecker(ctx)

	// HTTP routes for calls
	app := fiber.New()
	app.Post("/v1/calls/initiate", gateway.HandleInitiateCall)
	app.Post("/v1/calls/:callId/accept", gateway.HandleAcceptCall)
	app.Post("/v1/calls/:callId/reject", gateway.HandleRejectCall)
	app.Post("/v1/calls/:callId/end", gateway.HandleEndCall)
	app.Get("/v1/calls/:callId/ice-servers", gateway.HandleICEServers)

	// WebSocket endpoint
	app.Get("/v1/ws", gateway.HandleWebSocket)

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		logger.Info("shutting down websocket gateway")
		gateway.disconnectAll()
		app.ShutdownWithTimeout(cfg.Server.GracefulTimeout)
	}()

	logger.Info("websocket gateway starting", zap.String("addr", cfg.Addr()))
	if err := app.Listen(cfg.Addr()); err != nil {
		logger.Fatal("server error", zap.Error(err))
	}
}

func (gw *WSGateway) HandleWebSocket(c *fiber.Ctx) error {
	tokenStr := c.Query("token")
	if tokenStr == "" {
		// Try Authorization header
		authHeader := c.Get("Authorization")
		tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
	}
	if tokenStr == "" {
		return c.Status(401).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 401, "message": "missing token"},
		})
	}

	claims := &middleware.Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(gw.cfg.JWT.AccessSecret), nil
	})
	if err != nil || !token.Valid {
		return c.Status(401).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 401, "message": "invalid token"},
		})
	}

	// Upgrade to WebSocket
	conn, err := gw.upgrader.Upgrade(c.Context(), c.Request(), c.Response())
	if err != nil {
		gw.logger.Error("websocket upgrade failed", zap.Error(err))
		return err
	}

	connID := uuid.New().String()
	client := &ClientConn{
		UserID:   claims.UserID,
		ConnID:   connID,
		Conn:     conn,
		ChatSubs: make(map[string]bool),
		done:     make(chan struct{}),
	}

	gw.mu.Lock()
	if gw.clients[claims.UserID] == nil {
		gw.clients[claims.UserID] = make(map[string]*ClientConn)
	}
	gw.clients[claims.UserID][connID] = client
	gw.mu.Unlock()

	// Set presence
	gw.redis.Set(c.Context(), fmt.Sprintf("user:%s:online", claims.UserID), connID, 30*time.Second)

	// Publish presence event
	gw.publishPresence(claims.UserID, "online")

	gw.logger.Info("client connected",
		zap.String("user_id", claims.UserID),
		zap.String("conn_id", connID),
		zap.Int("total_clients", gw.totalClients()),
	)

	// Read loop
	go gw.readLoop(client)
	// Write loop (ping/pong)
	go gw.writeLoop(client)

	return nil
}

func (gw *WSGateway) readLoop(client *ClientConn) {
	defer func() {
		gw.removeClient(client)
	}()

	maxMessageSize := int64(65536) // 64KB max frame

	for {
		_, message, err := client.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				gw.logger.Warn("websocket read error",
					zap.String("user_id", client.UserID),
					zap.Error(err),
				)
			}
			break
		}

		if int64(len(message)) > maxMessageSize {
			gw.sendToClient(client, types.WSServerFrame{
				Type: "error",
				Data: fiber.Map{"message": "message too large"},
			})
			continue
		}

		var frame types.WSClientFrame
		if err := json.Unmarshal(message, &frame); err != nil {
			gw.sendToClient(client, types.WSServerFrame{
				Type: "error",
				Data: fiber.Map{"message": "invalid frame format"},
			})
			continue
		}

		switch frame.Type {
		case "ping":
			gw.sendToClient(client, types.WSServerFrame{Type: "pong"})

		case "subscribe":
			for _, chatID := range frame.ChatIDs {
				client.ChatSubs[chatID.String()] = true
			}

		case "typing":
			if frame.ChatID.String() != "" && frame.Action != "" {
				gw.broadcastToChat(frame.ChatID.String(), types.WSServerFrame{
					Type: "typing",
					Data: fiber.Map{
						"chat_id": frame.ChatID,
						"user_id": client.UserID,
						"action":  frame.Action,
					},
				})
			}

		case "read":
			if frame.ChatID.String() != "" && frame.MsgID.String() != "" {
				gw.broadcastToChat(frame.ChatID.String(), types.WSServerFrame{
					Type: "read_receipt",
					Data: fiber.Map{
						"chat_id":    frame.ChatID,
						"user_id":    client.UserID,
						"message_id": frame.MsgID,
					},
				})
			}

		case "call.offer":
			gw.handleCallSignaling(client, frame)

		case "call.answer":
			gw.handleCallSignaling(client, frame)

		case "call.ice":
			gw.handleCallSignaling(client, frame)

		default:
			gw.logger.Warn("unknown frame type",
				zap.String("type", frame.Type),
				zap.String("user_id", client.UserID),
			)
		}
	}
}

func (gw *WSGateway) writeLoop(client *ClientConn) {
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			gw.mu.RLock()
			_, exists := gw.clients[client.UserID][client.ConnID]
			gw.mu.RUnlock()
			if !exists {
				return
			}

			// Update presence heartbeat
			gw.redis.Expire(context.Background(), fmt.Sprintf("user:%s:online", client.UserID), 30*time.Second)

			if err := client.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}

		case <-client.done:
			return
		}
	}
}

func (gw *WSGateway) listenRedisPubSub(ctx context.Context) {
	pubsub := gw.redis.Subscribe(ctx, "presence", "calls")
	defer pubsub.Close()

	// Subscribe to all chat channels dynamically
	ch := pubsub.Channel()

	for msg := range ch {
		var event map[string]interface{}
		if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
			continue
		}

		chatID, _ := event["chat_id"].(string)
		if chatID != "" {
			gw.broadcastToChat(chatID, types.WSServerFrame{
				Type: msg.Channel,
				Data: event,
			})
		}
	}
}

func (gw *WSGateway) heartbeatChecker(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			gw.mu.RLock()
			userCount := len(gw.clients)
			connCount := 0
			for _, conns := range gw.clients {
				connCount += len(conns)
			}
			gw.mu.RUnlock()
			gw.logger.Debug("heartbeat",
				zap.Int("users", userCount),
				zap.Int("connections", connCount),
			)
		case <-ctx.Done():
			return
		}
	}
}

func (gw *WSGateway) sendToClient(client *ClientConn, frame types.WSServerFrame) {
	client.mu.Lock()
	defer client.mu.Unlock()

	data, err := json.Marshal(frame)
	if err != nil {
		gw.logger.Error("marshal frame", zap.Error(err))
		return
	}

	client.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := client.Conn.WriteMessage(websocket.TextMessage, data); err != nil {
		gw.logger.Warn("write to client failed",
			zap.String("user_id", client.UserID),
			zap.Error(err),
		)
	}
}

func (gw *WSGateway) sendToUser(userID string, frame types.WSServerFrame) {
	gw.mu.RLock()
	conns, exists := gw.clients[userID]
	gw.mu.RUnlock()

	if !exists {
		return
	}

	for _, client := range conns {
		gw.sendToClient(client, frame)
	}
}

func (gw *WSGateway) broadcastToChat(chatID string, frame types.WSServerFrame) {
	// First, try local clients
	gw.mu.RLock()
	for _, conns := range gw.clients {
		for _, client := range conns {
			if client.ChatSubs[chatID] {
				gw.sendToClient(client, frame)
			}
		}
	}
	gw.mu.RUnlock()

	// Publish to Redis for cross-node delivery
	data, _ := json.Marshal(frame)
	gw.redis.Publish(context.Background(), fmt.Sprintf("chat:%s", chatID), data)
}

func (gw *WSGateway) publishPresence(userID, status string) {
	event := map[string]interface{}{
		"type":    "presence",
		"user_id": userID,
		"status":  status,
	}
	data, _ := json.Marshal(event)
	gw.redis.Publish(context.Background(), "presence", data)
}

func (gw *WSGateway) removeClient(client *ClientConn) {
	gw.mu.Lock()
	if conns, ok := gw.clients[client.UserID]; ok {
		delete(conns, client.ConnID)
		if len(conns) == 0 {
			delete(gw.clients, client.UserID)
		}
	}
	totalUsers := len(gw.clients)
	gw.mu.Unlock()

	close(client.done)
	client.Conn.Close()

	// Check if user still has any connections
	gw.mu.RLock()
	_, stillConnected := gw.clients[client.UserID]
	gw.mu.RUnlock()

	if !stillConnected {
		gw.redis.Del(context.Background(), fmt.Sprintf("user:%s:online", client.UserID))
		gw.publishPresence(client.UserID, "offline")
	}

	gw.logger.Info("client disconnected",
		zap.String("user_id", client.UserID),
		zap.String("conn_id", client.ConnID),
		zap.Int("total_users", totalUsers),
	)
}

func (gw *WSGateway) disconnectAll() {
	gw.mu.Lock()
	defer gw.mu.Unlock()

	for _, conns := range gw.clients {
		for _, client := range conns {
			close(client.done)
			client.Conn.Close()
		}
	}
	gw.clients = make(map[string]map[string]*ClientConn)
}

func (gw *WSGateway) totalClients() int {
	gw.mu.RLock()
	defer gw.mu.RUnlock()
	count := 0
	for _, conns := range gw.clients {
		count += len(conns)
	}
	return count
}

// Call signaling
func (gw *WSGateway) handleCallSignaling(client *ClientConn, frame types.WSClientFrame) {
	data, _ := json.Marshal(frame.Data)
	var callEvent map[string]interface{}
	json.Unmarshal(data, &callEvent)

	targetID, _ := callEvent["target_id"].(string)
	if targetID != "" {
		gw.sendToUser(targetID, types.WSServerFrame{
			Type: fmt.Sprintf("call.%s", strings.SplitN(frame.Type, ".", 2)[1]),
			Data: callEvent,
		})
	}
}

type middleware struct{}

var _ = middleware.Claims

func (gw *WSGateway) HandleInitiateCall(c *fiber.Ctx) error {
	var req struct {
		PeerID   string `json:"peer_id"`
		CallType string `json:"type"` // audio, video
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request"}})
	}

	userID := c.Query("user_id")
	if userID == "" {
		userID = c.Get("X-User-ID")
	}

	callID := uuid.New().String()
	call := &callState{
		CallID:   callID,
		FromID:   userID,
		ToID:     req.PeerID,
		CallType: req.CallType,
		Status:   "initiated",
		SDPMap:   make(map[string]string),
	}

	callMu.Lock()
	activeCalls[callID] = call
	callMu.Unlock()

	// Notify peer via WS
	gw.sendToUser(req.PeerID, types.WSServerFrame{
		Type: "call.incoming",
		Data: fiber.Map{
			"call_id":   callID,
			"caller_id": userID,
			"type":      req.CallType,
		},
	})

	return c.JSON(types.NewAPIResponse(fiber.Map{"call_id": callID}))
}

func (gw *WSGateway) HandleAcceptCall(c *fiber.Ctx) error {
	callID := c.Params("callId")
	callMu.RLock()
	call, exists := activeCalls[callID]
	callMu.RUnlock()

	if !exists {
		return c.Status(404).JSON(fiber.Map{"ok": false, "error": fiber.Map{"code": 404, "message": "call not found"}})
	}

	call.mu.Lock()
	call.Status = "accepted"
	call.mu.Unlock()

	gw.sendToUser(call.FromID, types.WSServerFrame{
		Type: "call.accepted",
		Data: fiber.Map{"call_id": callID},
	})

	return c.JSON(types.NewAPIResponse(fiber.Map{"message": "call accepted"}))
}

func (gw *WSGateway) HandleRejectCall(c *fiber.Ctx) error {
	callID := c.Params("callId")
	callMu.RLock()
	call, exists := activeCalls[callID]
	callMu.RUnlock()

	if !exists {
		return c.Status(404).JSON(fiber.Map{"ok": false, "error": fiber.Map{"code": 404, "message": "call not found"}})
	}

	call.mu.Lock()
	call.Status = "rejected"
	call.mu.Unlock()

	gw.sendToUser(call.FromID, types.WSServerFrame{
		Type: "call.rejected",
		Data: fiber.Map{"call_id": callID},
	})

	callMu.Lock()
	delete(activeCalls, callID)
	callMu.Unlock()

	return c.JSON(types.NewAPIResponse(fiber.Map{"message": "call rejected"}))
}

func (gw *WSGateway) HandleEndCall(c *fiber.Ctx) error {
	callID := c.Params("callId")
	callMu.Lock()
	call, exists := activeCalls[callID]
	if exists {
		delete(activeCalls, callID)
	}
	callMu.Unlock()

	if !exists {
		return c.Status(404).JSON(fiber.Map{"ok": false, "error": fiber.Map{"code": 404, "message": "call not found"}})
	}

	gw.sendToUser(call.FromID, types.WSServerFrame{
		Type: "call.ended",
		Data: fiber.Map{"call_id": callID},
	})
	gw.sendToUser(call.ToID, types.WSServerFrame{
		Type: "call.ended",
		Data: fiber.Map{"call_id": callID},
	})

	return c.JSON(types.NewAPIResponse(fiber.Map{"message": "call ended"}))
}

func (gw *WSGateway) HandleICEServers(c *fiber.Ctx) error {
	return c.JSON(types.NewAPIResponse(fiber.Map{
		"ice_servers": []fiber.Map{
			{"urls": gw.cfg.WebRTC.STUNServers},
			{"urls": gw.cfg.WebRTC.TURNURL,
				"username":   gw.cfg.WebRTC.TURNUsername,
				"credential": gw.cfg.WebRTC.TURNCredential,
			},
		},
	}))
}

// To make middleware.Claims available
type Claims struct {
	UserID    string   `json:"user_id"`
	SessionID string   `json:"session_id"`
	Roles     []string `json:"roles,omitempty"`
	jwt.RegisteredClaims
}

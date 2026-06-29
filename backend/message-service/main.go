package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/iraq-secure-chat/common/config"
	"github.com/iraq-secure-chat/common/logging"
	"github.com/iraq-secure-chat/common/middleware"
	"github.com/iraq-secure-chat/common/types"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

type MessageService struct {
	cfg          *config.Config
	logger       *zap.Logger
	db           *pgxpool.Pool
	redis        *redis.Client
	kafkaWriter  *kafka.Writer
	kafkaReaders []*kafka.Reader
	mu           sync.RWMutex
	chatCache    map[string]*types.Chat
}

type SendMessageRequest struct {
	Type           string    `json:"type"`
	Text           string    `json:"text,omitempty"`
	MediaID        string    `json:"media_id,omitempty"`
	ReplyTo        string    `json:"reply_to,omitempty"`
	ForwardFrom    string    `json:"forward_from,omitempty"`
	ScheduleAt     *int64    `json:"schedule_at,omitempty"`
	AutoDeleteTTL  *int      `json:"auto_delete_ttl,omitempty"`
	IDempotencyKey string    `json:"idempotency_key,omitempty"`
}

type EditMessageRequest struct {
	Text string `json:"text"`
}

type ReactRequest struct {
	Emoji string `json:"emoji"`
}

type ReadRequest struct {
	MaxMessageID string `json:"max_message_id"`
}

type CreateGroupRequest struct {
	Title       string   `json:"title" validate:"required,min=1,max=128"`
	UserIDs     []string `json:"user_ids" validate:"required,min=1"`
	Description string   `json:"description,omitempty"`
}

type CreateChannelRequest struct {
	Title       string `json:"title" validate:"required,min=1,max=128"`
	Description string `json:"description,omitempty"`
	Username    string `json:"username,omitempty"`
}

type AddMemberRequest struct {
	UserID string `json:"user_id"`
	Role   string `json:"role,omitempty"`
}

func main() {
	cfg := config.Load()
	logger := logging.Init(cfg.ServiceName, cfg.Environment, cfg.LogLevel)

	ctx := context.Background()

	db, err := pgxpool.New(ctx, cfg.Database.PostgresDSN)
	if err != nil {
		logger.Fatal("database connection failed", zap.Error(err))
	}
	defer db.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr:         cfg.Cache.RedisURL,
		Password:     cfg.Cache.RedisPassword,
		DB:           cfg.Cache.RedisDB,
		PoolSize:     cfg.Cache.PoolSize,
		MinIdleConns: cfg.Cache.MinIdleConns,
	})

	if err := redisClient.Ping(ctx).Err(); err != nil {
		logger.Fatal("redis connection failed", zap.Error(err))
	}

	kafkaWriter := &kafka.Writer{
		Addr:     kafka.TCP(cfg.Kafka.Brokers...),
		Topic:    cfg.Kafka.TopicPrefix + ".messages.inbound",
		Balancer: &kafka.Hash{},
		Async:    true,
		BatchSize: cfg.Kafka.BatchSize,
		BatchTimeout: cfg.Kafka.BatchTimeout,
		RequiredAcks: kafka.RequireOne,
	}
	defer kafkaWriter.Close()

	svc := &MessageService{
		cfg:         cfg,
		logger:      logger,
		db:          db,
		redis:       redisClient,
		kafkaWriter: kafkaWriter,
		chatCache:   make(map[string]*types.Chat),
	}

	app := fiber.New(fiber.Config{
		AppName:           "IraqSecureChat Message Service",
		ReadTimeout:       cfg.Server.ReadTimeout,
		WriteTimeout:      cfg.Server.WriteTimeout,
		ReduceMemoryUsage: true,
	})

	// Chat routes
	app.Get("/v1/chats", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleListChats)
	app.Post("/v1/chats/create-group", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleCreateGroup)
	app.Post("/v1/chats/create-channel", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleCreateChannel)
	app.Get("/v1/chats/:chatId", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleGetChat)
	app.Put("/v1/chats/:chatId", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleUpdateChat)
	app.Get("/v1/chats/:chatId/members", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleListMembers)
	app.Post("/v1/chats/:chatId/members", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleAddMember)
	app.Delete("/v1/chats/:chatId/members/:userId", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleRemoveMember)
	app.Post("/v1/chats/:chatId/invite-link", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleCreateInviteLink)
	app.Post("/v1/chats/:chatId/join", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleJoinChat)
	app.Post("/v1/chats/:chatId/leave", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleLeaveChat)

	// Message routes
	app.Get("/v1/chats/:chatId/messages", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleGetMessages)
	app.Post("/v1/chats/:chatId/messages", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleSendMessage)
	app.Put("/v1/chats/:chatId/messages/:msgId", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleEditMessage)
	app.Delete("/v1/chats/:chatId/messages/:msgId", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleDeleteMessage)
	app.Post("/v1/chats/:chatId/messages/:msgId/react", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleReactToMessage)
	app.Post("/v1/chats/:chatId/messages/:msgId/pin", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandlePinMessage)
	app.Post("/v1/chats/:chatId/messages/read", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleMarkRead)
	app.Get("/v1/chats/:chatId/messages/search", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleSearchMessages)
	app.Post("/v1/chats/:chatId/messages/:msgId/forward", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleForwardMessage)
	app.Post("/v1/chats/:chatId/messages/:msgId/schedule", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleScheduleMessage)
	app.Post("/v1/chats/:chatId/messages/:msgId/auto-delete", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleSetAutoDelete)

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		logger.Info("shutting down message service")
		app.ShutdownWithTimeout(cfg.Server.GracefulTimeout)
	}()

	logger.Info("message service starting", zap.String("addr", cfg.Addr()))
	if err := app.Listen(cfg.Addr()); err != nil {
		logger.Fatal("server error", zap.Error(err))
	}
}

func (s *MessageService) HandleListChats(c *fiber.Ctx) error {
	userID := middleware.ExtractUserID(c)
	if userID == "" {
		return c.Status(401).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 401, "message": "unauthorized"},
		})
	}

	cursor := c.Query("cursor")
	limit := c.QueryInt("limit", 50)
	if limit > 100 {
		limit = 100
	}

	query := `SELECT c.id, c.type, c.title, COALESCE(c.username,''), COALESCE(c.description,''),
	                 COALESCE(c.avatar_url,''), c.created_by, c.settings, c.created_at
	          FROM chats c
	          INNER JOIN chat_members cm ON cm.chat_id = c.id
	          WHERE cm.user_id = $1`
	args := []interface{}{userID}
	argIdx := 2

	if cursor != "" {
		query += fmt.Sprintf(` AND c.created_at < $%d`, argIdx)
		args = append(args, cursor)
		argIdx++
	}

	query += fmt.Sprintf(` ORDER BY c.created_at DESC LIMIT $%d`, argIdx)
	args = append(args, limit+1)

	rows, err := s.db.Query(c.Context(), query, args...)
	if err != nil {
		s.logger.Error("list chats", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "failed to list chats"},
		})
	}
	defer rows.Close()

	var chats []types.Chat
	for rows.Next() {
		var chat types.Chat
		if err := rows.Scan(&chat.ID, &chat.Type, &chat.Title, &chat.Username,
			&chat.Description, &chat.AvatarURL, &chat.CreatedBy, &chat.Settings, &chat.CreatedAt); err != nil {
			continue
		}
		chats = append(chats, chat)
	}

	hasMore := len(chats) > limit
	if hasMore {
		chats = chats[:limit]
	}

	var nextCursor string
	if len(chats) > 0 {
		nextCursor = chats[len(chats)-1].CreatedAt.Format(time.RFC3339Nano)
	}

	return c.JSON(types.NewAPIResponse(types.PaginatedResponse{
		Items:      chats,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}))
}

func (s *MessageService) HandleCreateGroup(c *fiber.Ctx) error {
	userID := middleware.ExtractUserID(c)
	if userID == "" {
		return c.Status(401).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 401, "message": "unauthorized"},
		})
	}

	var req CreateGroupRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request body"},
		})
	}

	if req.Title == "" {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "title is required"},
		})
	}

	chatID := uuid.New().String()
	now := time.Now()

	tx, err := s.db.Begin(c.Context())
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "internal server error"},
		})
	}
	defer tx.Rollback(c.Context())

	chatType := "group"
	if len(req.UserIDs) > 50 {
		chatType = "supergroup"
	}

	_, err = tx.Exec(c.Context(),
		`INSERT INTO chats (id, type, title, description, created_by, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		chatID, chatType, req.Title, req.Description, userID, now)
	if err != nil {
		s.logger.Error("create chat", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "failed to create group"},
		})
	}

	// Add creator as owner
	_, err = tx.Exec(c.Context(),
		`INSERT INTO chat_members (chat_id, user_id, role, joined_at)
		 VALUES ($1, $2, 'owner', $3)`,
		chatID, userID, now)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "failed to add owner"},
		})
	}

	// Add members
	for _, memberID := range req.UserIDs {
		if memberID == userID {
			continue
		}
		_, err = tx.Exec(c.Context(),
			`INSERT INTO chat_members (chat_id, user_id, role, joined_at)
			 VALUES ($1, $2, 'member', $3) ON CONFLICT DO NOTHING`,
			chatID, memberID, now)
		if err != nil {
			s.logger.Warn("failed to add member", zap.String("user_id", memberID), zap.Error(err))
		}
	}

	if err := tx.Commit(c.Context()); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "failed to create group"},
		})
	}

	s.logger.Info("group created",
		zap.String("chat_id", chatID),
		zap.String("title", req.Title),
		zap.Int("members", len(req.UserIDs)+1),
	)

	return c.JSON(types.NewAPIResponse(fiber.Map{
		"chat_id": chatID,
		"type":    chatType,
		"title":   req.Title,
	}))
}

func (s *MessageService) HandleCreateChannel(c *fiber.Ctx) error {
	userID := middleware.ExtractUserID(c)
	if userID == "" {
		return c.Status(401).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 401, "message": "unauthorized"},
		})
	}

	var req CreateChannelRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request body"},
		})
	}

	if req.Title == "" {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "title is required"},
		})
	}

	chatID := uuid.New().String()
	now := time.Now()

	_, err := s.db.Exec(c.Context(),
		`INSERT INTO chats (id, type, title, description, username, created_by, settings, created_at)
		 VALUES ($1, 'channel', $2, $3, $4, $5, '{"sign_messages":true}', $6)`,
		chatID, req.Title, req.Description, req.Username, userID, now)
	if err != nil {
		s.logger.Error("create channel", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "failed to create channel"},
		})
	}

	_, err = s.db.Exec(c.Context(),
		`INSERT INTO chat_members (chat_id, user_id, role, joined_at) VALUES ($1, $2, 'owner', $3)`,
		chatID, userID, now)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "internal server error"},
		})
	}

	return c.JSON(types.NewAPIResponse(fiber.Map{
		"chat_id": chatID,
		"type":    "channel",
		"title":   req.Title,
	}))
}

func (s *MessageService) HandleGetChat(c *fiber.Ctx) error {
	userID := middleware.ExtractUserID(c)
	chatID := c.Params("chatId")
	if userID == "" || chatID == "" {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request"},
		})
	}

	// Check membership
	isMember, err := s.isChatMember(c.Context(), chatID, userID)
	if err != nil || !isMember {
		return c.Status(403).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 403, "message": "not a member of this chat"},
		})
	}

	var chat types.Chat
	err = s.db.QueryRow(c.Context(),
		`SELECT id, type, title, COALESCE(username,''), COALESCE(description,''),
		        COALESCE(avatar_url,''), created_by, settings, created_at
		 FROM chats WHERE id = $1`, chatID,
	).Scan(&chat.ID, &chat.Type, &chat.Title, &chat.Username, &chat.Description,
		&chat.AvatarURL, &chat.CreatedBy, &chat.Settings, &chat.CreatedAt)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 404, "message": "chat not found"},
		})
	}

	return c.JSON(types.NewAPIResponse(chat))
}

func (s *MessageService) HandleUpdateChat(c *fiber.Ctx) error {
	userID := middleware.ExtractUserID(c)
	chatID := c.Params("chatId")
	if userID == "" || chatID == "" {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request"},
		})
	}

	role, err := s.getMemberRole(c.Context(), chatID, userID)
	if err != nil || (role != "owner" && role != "admin") {
		return c.Status(403).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 403, "message": "only admins can update chat"},
		})
	}

	var updates map[string]interface{}
	if err := c.BodyParser(&updates); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request body"},
		})
	}

	setClauses := []string{}
	args := []interface{}{}
	argIdx := 1

	if title, ok := updates["title"]; ok {
		setClauses = append(setClauses, fmt.Sprintf("title = $%d", argIdx))
		args = append(args, title)
		argIdx++
	}
	if desc, ok := updates["description"]; ok {
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, desc)
		argIdx++
	}
	if username, ok := updates["username"]; ok {
		setClauses = append(setClauses, fmt.Sprintf("username = $%d", argIdx))
		args = append(args, username)
		argIdx++
	}

	if len(setClauses) == 0 {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "no fields to update"},
		})
	}

	query := fmt.Sprintf(`UPDATE chats SET %s WHERE id = $%d`,
		strings.Join(setClauses, ", "), argIdx)
	args = append(args, chatID)

	_, err = s.db.Exec(c.Context(), query, args...)
	if err != nil {
		s.logger.Error("update chat", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "update failed"},
		})
	}

	return c.JSON(types.NewAPIResponse(fiber.Map{"message": "chat updated"}))
}

func (s *MessageService) HandleListMembers(c *fiber.Ctx) error {
	chatID := c.Params("chatId")
	role := c.Query("role")
	cursor := c.Query("cursor")
	limit := c.QueryInt("limit", 50)
	if limit > 200 {
		limit = 200
	}

	query := `SELECT u.id, u.display_name, COALESCE(u.username,''), COALESCE(u.avatar_url,''),
	                 cm.role, cm.joined_at
	          FROM chat_members cm
	          INNER JOIN users u ON u.id = cm.user_id
	          WHERE cm.chat_id = $1`
	args := []interface{}{chatID}
	argIdx := 2

	if role != "" {
		query += fmt.Sprintf(` AND cm.role = $%d`, argIdx)
		args = append(args, role)
		argIdx++
	}
	if cursor != "" {
		query += fmt.Sprintf(` AND cm.joined_at < $%d`, argIdx)
		args = append(args, cursor)
		argIdx++
	}

	query += fmt.Sprintf(` ORDER BY cm.joined_at DESC LIMIT $%d`, argIdx)
	args = append(args, limit+1)

	rows, err := s.db.Query(c.Context(), query, args...)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "failed to list members"},
		})
	}
	defer rows.Close()

	type MemberInfo struct {
		UserID      string    `json:"user_id"`
		DisplayName string    `json:"display_name"`
		Username    string    `json:"username,omitempty"`
		AvatarURL   string    `json:"avatar_url,omitempty"`
		Role        string    `json:"role"`
		JoinedAt    time.Time `json:"joined_at"`
	}

	var members []MemberInfo
	for rows.Next() {
		var m MemberInfo
		if err := rows.Scan(&m.UserID, &m.DisplayName, &m.Username, &m.AvatarURL, &m.Role, &m.JoinedAt); err != nil {
			continue
		}
		members = append(members, m)
	}

	hasMore := len(members) > limit
	if hasMore {
		members = members[:limit]
	}

	var nextCursor string
	if len(members) > 0 {
		nextCursor = members[len(members)-1].JoinedAt.Format(time.RFC3339Nano)
	}

	return c.JSON(types.NewAPIResponse(types.PaginatedResponse{
		Items:      members,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}))
}

func (s *MessageService) HandleAddMember(c *fiber.Ctx) error {
	userID := middleware.ExtractUserID(c)
	chatID := c.Params("chatId")
	if userID == "" || chatID == "" {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request"},
		})
	}

	var req AddMemberRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request body"},
		})
	}

	role, err := s.getMemberRole(c.Context(), chatID, userID)
	if err != nil || (role != "owner" && role != "admin") {
		return c.Status(403).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 403, "message": "only admins can add members"},
		})
	}

	memberRole := req.Role
	if memberRole == "" {
		memberRole = "member"
	}

	_, err = s.db.Exec(c.Context(),
		`INSERT INTO chat_members (chat_id, user_id, role, joined_at)
		 VALUES ($1, $2, $3, NOW()) ON CONFLICT (chat_id, user_id)
		 DO UPDATE SET role = EXCLUDED.role, joined_at = NOW()`,
		chatID, req.UserID, memberRole)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "failed to add member"},
		})
	}

	// Invalidate member list cache
	s.redis.Del(c.Context(), fmt.Sprintf("chat_members:%s", chatID))

	return c.JSON(types.NewAPIResponse(fiber.Map{"message": "member added"}))
}

func (s *MessageService) HandleRemoveMember(c *fiber.Ctx) error {
	userID := middleware.ExtractUserID(c)
	chatID := c.Params("chatId")
	targetUserID := c.Params("userId")
	if userID == "" || chatID == "" || targetUserID == "" {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request"},
		})
	}

	role, err := s.getMemberRole(c.Context(), chatID, userID)
	if err != nil || (role != "owner" && role != "admin") {
		return c.Status(403).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 403, "message": "only admins can remove members"},
		})
	}

	_, err = s.db.Exec(c.Context(),
		`DELETE FROM chat_members WHERE chat_id = $1 AND user_id = $2`,
		chatID, targetUserID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "failed to remove member"},
		})
	}

	s.redis.Del(c.Context(), fmt.Sprintf("chat_members:%s", chatID))

	return c.JSON(types.NewAPIResponse(fiber.Map{"message": "member removed"}))
}

func (s *MessageService) HandleCreateInviteLink(c *fiber.Ctx) error {
	chatID := c.Params("chatId")
	inviteHash := uuid.New().String()[:12]

	_, err := s.db.Exec(c.Context(),
		`UPDATE chats SET settings = jsonb_set(COALESCE(settings,'{}'::jsonb), '{invite_hash}', to_jsonb($1::text))
		 WHERE id = $2`,
		inviteHash, chatID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "failed to create invite link"},
		})
	}

	return c.JSON(types.NewAPIResponse(fiber.Map{
		"invite_link": fmt.Sprintf("https://t.me/+%s", inviteHash),
		"hash":        inviteHash,
	}))
}

func (s *MessageService) HandleJoinChat(c *fiber.Ctx) error {
	userID := middleware.ExtractUserID(c)
	chatID := c.Params("chatId")
	if userID == "" || chatID == "" {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request"},
		})
	}

	var inviteHash string
	var settings types.ChatSettings

	err := s.db.QueryRow(c.Context(),
		`SELECT settings->>'invite_hash', settings FROM chats WHERE id = $1`, chatID,
	).Scan(&inviteHash, &settings)

	if err != nil || inviteHash == "" {
		return c.Status(403).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 403, "message": "no invite link available"},
		})
	}

	_, err = s.db.Exec(c.Context(),
		`INSERT INTO chat_members (chat_id, user_id, role, joined_at)
		 VALUES ($1, $2, 'member', NOW()) ON CONFLICT DO NOTHING`,
		chatID, userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "failed to join chat"},
		})
	}

	return c.JSON(types.NewAPIResponse(fiber.Map{"message": "joined chat"}))
}

func (s *MessageService) HandleLeaveChat(c *fiber.Ctx) error {
	userID := middleware.ExtractUserID(c)
	chatID := c.Params("chatId")
	if userID == "" || chatID == "" {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request"},
		})
	}

	_, err := s.db.Exec(c.Context(),
		`DELETE FROM chat_members WHERE chat_id = $1 AND user_id = $2`,
		chatID, userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "failed to leave chat"},
		})
	}

	return c.JSON(types.NewAPIResponse(fiber.Map{"message": "left chat"}))
}

func (s *MessageService) HandleSendMessage(c *fiber.Ctx) error {
	userID := middleware.ExtractUserID(c)
	chatID := c.Params("chatId")
	if userID == "" || chatID == "" {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request"},
		})
	}

	var req SendMessageRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request body"},
		})
	}

	// Validate input
	if req.Type == "" {
		req.Type = "text"
	}
	if req.Type == "text" && req.Text == "" {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "text message cannot be empty"},
		})
	}
	if len(req.Text) > 4096 {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "message exceeds 4096 character limit"},
		})
	}

	// Check membership
	isMember, err := s.isChatMember(c.Context(), chatID, userID)
	if err != nil || !isMember {
		return c.Status(403).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 403, "message": "not a member of this chat"},
		})
	}

	// Check idempotency
	if req.IDempotencyKey != "" {
		existing, _ := s.redis.Get(c.Context(), fmt.Sprintf("idempotent:%s", req.IDempotencyKey)).Result()
		if existing != "" {
			return c.JSON(types.NewAPIResponse(fiber.Map{
				"message_id": existing,
				"duplicate":  true,
			}))
		}
	}

	messageID := uuid.New().String()
	now := time.Now()

	msg := types.Message{
		ID:         types.MessageID(uuid.MustParse(messageID)),
		ChatID:     types.ChatID(uuid.MustParse(chatID)),
		SenderID:   types.UserID(uuid.MustParse(userID)),
		Type:       types.MessageType(req.Type),
		Content:    req.Text,
		SentAt:     now,
		IDempotencyKey: req.IDempotencyKey,
	}

	if req.ReplyTo != "" {
		replyID := uuid.MustParse(req.ReplyTo)
		msg.ReplyTo = (*types.MessageID)(&replyID)
	}

	// Publish to Kafka
	msgJSON, _ := json.Marshal(msg)
	err = s.kafkaWriter.WriteMessages(c.Context(), kafka.Message{
		Key:   []byte(chatID),
		Value: msgJSON,
		Headers: []kafka.Header{
			{Key: "chat_id", Value: []byte(chatID)},
			{Key: "sender_id", Value: []byte(userID)},
			{Key: "message_type", Value: []byte(req.Type)},
		},
	})
	if err != nil {
		s.logger.Error("kafka write failed", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "failed to send message"},
		})
	}

	// Store idempotency key
	if req.IDempotencyKey != "" {
		s.redis.Set(c.Context(), fmt.Sprintf("idempotent:%s", req.IDempotencyKey), messageID, 24*time.Hour)
	}

	// Update the chat's last message time and content
	s.db.Exec(c.Context(),
		`UPDATE chats SET settings = jsonb_set(COALESCE(settings, '{}'::jsonb), '{last_message_id}', to_jsonb($1::text))
		 WHERE id = $2`,
		messageID, chatID)

	s.logger.Info("message sent",
		zap.String("message_id", messageID),
		zap.String("chat_id", chatID),
		zap.String("sender", userID),
		zap.String("type", req.Type),
	)

	return c.JSON(types.NewAPIResponse(fiber.Map{
		"message_id": messageID,
		"sent_at":    now,
	}))
}

func (s *MessageService) HandleGetMessages(c *fiber.Ctx) error {
	userID := middleware.ExtractUserID(c)
	chatID := c.Params("chatId")
	if userID == "" || chatID == "" {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request"},
		})
	}

	isMember, err := s.isChatMember(c.Context(), chatID, userID)
	if err != nil || !isMember {
		return c.Status(403).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 403, "message": "not a member of this chat"},
		})
	}

	cursor := c.Query("cursor")
	direction := c.Query("direction", "before")
	limit := c.QueryInt("limit", 50)
	if limit > 100 {
		limit = 100
	}

	// For MVP, use PostgreSQL. In production, use Cassandra/ScyllaDB
	query := `SELECT id, chat_id, sender_id, type, content, reply_to, forward_from,
	                 media, poll, edited_at, reactions, entities, sent_at, schedule_at, auto_delete_at
	          FROM messages WHERE chat_id = $1`
	args := []interface{}{chatID}
	argIdx := 2

	if cursor != "" {
		if direction == "before" {
			query += fmt.Sprintf(` AND sent_at < $%d`, argIdx)
		} else {
			query += fmt.Sprintf(` AND sent_at > $%d`, argIdx)
		}
		args = append(args, cursor)
		argIdx++
	}

	order := "DESC"
	if direction == "after" {
		order = "ASC"
	}
	query += fmt.Sprintf(` ORDER BY sent_at %s LIMIT $%d`, order, argIdx)
	args = append(args, limit+1)

	rows, err := s.db.Query(c.Context(), query, args...)
	if err != nil {
		s.logger.Error("get messages", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "failed to fetch messages"},
		})
	}
	defer rows.Close()

	var messages []types.Message
	for rows.Next() {
		var msg types.Message
		if err := rows.Scan(&msg.ID, &msg.ChatID, &msg.SenderID, &msg.Type, &msg.Content,
			&msg.ReplyTo, &msg.ForwardFrom, &msg.Media, &msg.Poll, &msg.EditedAt,
			&msg.Reactions, &msg.Entities, &msg.SentAt, &msg.ScheduleAt, &msg.AutoDeleteAt); err != nil {
			continue
		}
		messages = append(messages, msg)
	}

	hasMore := len(messages) > limit
	if hasMore {
		messages = messages[:limit]
	}

	var nextCursor string
	if len(messages) > 0 {
		nextCursor = messages[len(messages)-1].SentAt.Format(time.RFC3339Nano)
	}

	return c.JSON(types.NewAPIResponse(types.PaginatedResponse{
		Items:      messages,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}))
}

func (s *MessageService) HandleEditMessage(c *fiber.Ctx) error {
	userID := middleware.ExtractUserID(c)
	chatID := c.Params("chatId")
	msgID := c.Params("msgId")
	if userID == "" || chatID == "" || msgID == "" {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request"},
		})
	}

	var req EditMessageRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request body"},
		})
	}

	// Verify ownership
	var senderID string
	err := s.db.QueryRow(c.Context(),
		`SELECT sender_id FROM messages WHERE id = $1 AND chat_id = $2`,
		msgID, chatID).Scan(&senderID)
	if err != nil || senderID != userID {
		return c.Status(403).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 403, "message": "can only edit your own messages"},
		})
	}

	now := time.Now()
	_, err = s.db.Exec(c.Context(),
		`UPDATE messages SET content = $1, edited_at = $2 WHERE id = $3 AND chat_id = $4`,
		req.Text, now, msgID, chatID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "edit failed"},
		})
	}

	// Notify via Redis pub/sub
	s.redis.Publish(c.Context(), fmt.Sprintf("chat:%s", chatID), map[string]interface{}{
		"type":       "message.edited",
		"chat_id":    chatID,
		"message_id": msgID,
		"new_text":   req.Text,
		"edited_at":  now,
	})

	return c.JSON(types.NewAPIResponse(fiber.Map{"message": "message edited"}))
}

func (s *MessageService) HandleDeleteMessage(c *fiber.Ctx) error {
	userID := middleware.ExtractUserID(c)
	chatID := c.Params("chatId")
	msgID := c.Params("msgId")
	if userID == "" || chatID == "" || msgID == "" {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request"},
		})
	}

	forEveryone := c.Query("for_everyone") == "true"

	if forEveryone {
		// Check if user is admin or owner
		role, err := s.getMemberRole(c.Context(), chatID, userID)
		if err != nil || (role != "owner" && role != "admin") {
			// Check if it's the sender's own message
			var senderID string
			s.db.QueryRow(c.Context(),
				`SELECT sender_id FROM messages WHERE id = $1`, msgID).Scan(&senderID)
			if senderID != userID {
				return c.Status(403).JSON(fiber.Map{
					"ok": false, "error": fiber.Map{"code": 403, "message": "not authorized to delete for everyone"},
				})
			}
		}
		_, err = s.db.Exec(c.Context(),
			`UPDATE messages SET deleted_for_all = true WHERE id = $1 AND chat_id = $2`,
			msgID, chatID)
	} else {
		_, err = s.db.Exec(c.Context(),
			`UPDATE messages SET deleted_for = array_append(COALESCE(deleted_for, '{}'), $1::uuid)
			 WHERE id = $2 AND chat_id = $3`,
			userID, msgID, chatID)
	}

	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "delete failed"},
		})
	}

	s.redis.Publish(c.Context(), fmt.Sprintf("chat:%s", chatID), fiber.Map{
		"type":         "message.deleted",
		"chat_id":      chatID,
		"message_id":   msgID,
		"for_everyone": forEveryone,
	})

	return c.JSON(types.NewAPIResponse(fiber.Map{"message": "message deleted"}))
}

func (s *MessageService) HandleReactToMessage(c *fiber.Ctx) error {
	userID := middleware.ExtractUserID(c)
	chatID := c.Params("chatId")
	msgID := c.Params("msgId")
	if userID == "" || chatID == "" || msgID == "" {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request"},
		})
	}

	var req ReactRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request body"},
		})
	}

	if req.Emoji == "" {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "emoji is required"},
		})
	}

	// Toggle reaction: add if not present, remove if present
	var existingReactions map[string][]string
	err := s.db.QueryRow(c.Context(),
		`SELECT reactions FROM messages WHERE id = $1 AND chat_id = $2`,
		msgID, chatID).Scan(&existingReactions)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 404, "message": "message not found"},
		})
	}

	if existingReactions == nil {
		existingReactions = make(map[string][]string)
	}

	users, exists := existingReactions[req.Emoji]
	found := false
	for i, uid := range users {
		if uid == userID {
			users = append(users[:i], users[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		users = append(users, userID)
	}

	existingReactions[req.Emoji] = users
	if len(users) == 0 {
		delete(existingReactions, req.Emoji)
	}

	reactionsJSON, _ := json.Marshal(existingReactions)
	_, err = s.db.Exec(c.Context(),
		`UPDATE messages SET reactions = $1::jsonb WHERE id = $2 AND chat_id = $3`,
		string(reactionsJSON), msgID, chatID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "reaction failed"},
		})
	}

	s.redis.Publish(c.Context(), fmt.Sprintf("chat:%s", chatID), fiber.Map{
		"type":       "reaction.add",
		"chat_id":    chatID,
		"message_id": msgID,
		"user_id":    userID,
		"emoji":      req.Emoji,
	})

	return c.JSON(types.NewAPIResponse(fiber.Map{
		"message":   "reaction updated",
		"emoji":     req.Emoji,
		"count":     len(users),
	}))
}

func (s *MessageService) HandlePinMessage(c *fiber.Ctx) error {
	userID := middleware.ExtractUserID(c)
	chatID := c.Params("chatId")
	msgID := c.Params("msgId")
	if userID == "" || chatID == "" || msgID == "" {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request"},
		})
	}

	role, err := s.getMemberRole(c.Context(), chatID, userID)
	if err != nil || (role != "owner" && role != "admin") {
		return c.Status(403).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 403, "message": "only admins can pin messages"},
		})
	}

	_, err = s.db.Exec(c.Context(),
		`UPDATE chats SET settings = jsonb_set(COALESCE(settings, '{}'::jsonb), '{pinned_message_id}', to_jsonb($1::text))
		 WHERE id = $2`,
		msgID, chatID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "pin failed"},
		})
	}

	return c.JSON(types.NewAPIResponse(fiber.Map{"message": "message pinned"}))
}

func (s *MessageService) HandleMarkRead(c *fiber.Ctx) error {
	userID := middleware.ExtractUserID(c)
	chatID := c.Params("chatId")
	if userID == "" || chatID == "" {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request"},
		})
	}

	var req ReadRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request body"},
		})
	}

	key := fmt.Sprintf("read:%s:%s", chatID, userID)
	s.redis.Set(c.Context(), key, req.MaxMessageID, 24*time.Hour)

	// Notify other members
	s.redis.Publish(c.Context(), fmt.Sprintf("chat:%s", chatID), fiber.Map{
		"type":           "read_receipt",
		"chat_id":        chatID,
		"user_id":        userID,
		"max_message_id": req.MaxMessageID,
	})

	s.logger.Debug("messages marked read",
		zap.String("user_id", userID),
		zap.String("chat_id", chatID),
		zap.String("max_msg_id", req.MaxMessageID),
	)

	return c.JSON(types.NewAPIResponse(fiber.Map{"message": "read receipt sent"}))
}

func (s *MessageService) HandleSearchMessages(c *fiber.Ctx) error {
	userID := middleware.ExtractUserID(c)
	chatID := c.Params("chatId")
	if userID == "" || chatID == "" {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request"},
		})
	}

	query := c.Query("q")
	if query == "" || len(query) < 2 {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "query must be at least 2 characters"},
		})
	}

	cursor := c.Query("cursor")
	limit := c.QueryInt("limit", 50)
	if limit > 100 {
		limit = 100
	}

	sqlQuery := `SELECT id, chat_id, sender_id, type, content, reply_to, sent_at
	             FROM messages
	             WHERE chat_id = $1
	               AND content ILIKE '%' || $2 || '%'
	               AND (deleted_for_all = false OR deleted_for_all IS NULL)`
	args := []interface{}{chatID, query}
	argIdx := 3

	if cursor != "" {
		sqlQuery += fmt.Sprintf(` AND sent_at < $%d`, argIdx)
		args = append(args, cursor)
		argIdx++
	}

	sqlQuery += fmt.Sprintf(` ORDER BY sent_at DESC LIMIT $%d`, argIdx)
	args = append(args, limit+1)

	rows, err := s.db.Query(c.Context(), sqlQuery, args...)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "search failed"},
		})
	}
	defer rows.Close()

	type SearchResult struct {
		MessageID string    `json:"message_id"`
		ChatID    string    `json:"chat_id"`
		SenderID  string    `json:"sender_id"`
		Type      string    `json:"type"`
		Content   string    `json:"content"`
		SentAt    time.Time `json:"sent_at"`
	}

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.MessageID, &r.ChatID, &r.SenderID, &r.Type, &r.Content, &r.SentAt); err != nil {
			continue
		}
		results = append(results, r)
	}

	hasMore := len(results) > limit
	if hasMore {
		results = results[:limit]
	}

	var nextCursor string
	if len(results) > 0 {
		nextCursor = results[len(results)-1].SentAt.Format(time.RFC3339Nano)
	}

	return c.JSON(types.NewAPIResponse(types.PaginatedResponse{
		Items:      results,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}))
}

func (s *MessageService) HandleForwardMessage(c *fiber.Ctx) error {
	userID := middleware.ExtractUserID(c)
	chatID := c.Params("chatId")
	msgID := c.Params("msgId")
	if userID == "" || chatID == "" || msgID == "" {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request"},
		})
	}

	var req struct {
		ToChatID string `json:"to_chat_id"`
	}
	if err := c.BodyParser(&req); err != nil || req.ToChatID == "" {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "to_chat_id is required"},
		})
	}

	// Get original message
	var msg types.Message
	err := s.db.QueryRow(c.Context(),
		`SELECT id, chat_id, sender_id, type, content, media, poll, entities, sent_at
		 FROM messages WHERE id = $1 AND chat_id = $2`,
		msgID, chatID).Scan(&msg.ID, &msg.ChatID, &msg.SenderID, &msg.Type, &msg.Content,
		&msg.Media, &msg.Poll, &msg.Entities, &msg.SentAt)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 404, "message": "message not found"},
		})
	}

	// Create forwarded copy
	newMsgID := uuid.New().String()
	_, err = s.db.Exec(c.Context(),
		`INSERT INTO messages (id, chat_id, sender_id, type, content, forward_from, sent_at)
		 VALUES ($1, $2, $3, $4, $5,
		         jsonb_build_object('from_chat_id', $6::text, 'from_message_id', $7::text, 'sender_id', $8::text),
		         NOW())`,
		newMsgID, req.ToChatID, userID, msg.Type, msg.Content,
		chatID, msgID, msg.SenderID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "forward failed"},
		})
	}

	return c.JSON(types.NewAPIResponse(fiber.Map{
		"message_id": newMsgID,
	}))
}

func (s *MessageService) HandleScheduleMessage(c *fiber.Ctx) error {
	userID := middleware.ExtractUserID(c)
	chatID := c.Params("chatId")
	msgID := c.Params("msgId")
	if userID == "" || chatID == "" || msgID == "" {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request"},
		})
	}

	var req struct {
		ScheduleAt int64 `json:"schedule_at"`
	}
	if err := c.BodyParser(&req); err != nil || req.ScheduleAt == 0 {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "schedule_at timestamp is required"},
		})
	}

	scheduleTime := time.Unix(req.ScheduleAt, 0)
	if scheduleTime.Before(time.Now()) {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "schedule time must be in the future"},
		})
	}

	_, err := s.db.Exec(c.Context(),
		`UPDATE messages SET schedule_at = $1 WHERE id = $2 AND chat_id = $3 AND sender_id = $4`,
		scheduleTime, msgID, chatID, userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "schedule failed"},
		})
	}

	return c.JSON(types.NewAPIResponse(fiber.Map{
		"message":     "message scheduled",
		"schedule_at": scheduleTime.Unix(),
	}))
}

func (s *MessageService) HandleSetAutoDelete(c *fiber.Ctx) error {
	userID := middleware.ExtractUserID(c)
	chatID := c.Params("chatId")
	msgID := c.Params("msgId")
	if userID == "" || chatID == "" || msgID == "" {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request"},
		})
	}

	var req struct {
		TTL int `json:"ttl"` // seconds
	}
	if err := c.BodyParser(&req); err != nil || req.TTL <= 0 {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "valid ttl (seconds) is required"},
		})
	}

	autoDeleteAt := time.Now().Add(time.Duration(req.TTL) * time.Second)
	_, err := s.db.Exec(c.Context(),
		`UPDATE messages SET auto_delete_at = $1 WHERE id = $2 AND chat_id = $3`,
		autoDeleteAt, msgID, chatID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "auto-delete setup failed"},
		})
	}

	return c.JSON(types.NewAPIResponse(fiber.Map{
		"message":        "auto-delete set",
		"auto_delete_at": autoDeleteAt.Unix(),
	}))
}

func (s *MessageService) isChatMember(ctx context.Context, chatID, userID string) (bool, error) {
	cacheKey := fmt.Sprintf("chat_member:%s:%s", chatID, userID)
	cached, err := s.redis.Get(ctx, cacheKey).Bool()
	if err == nil {
		return cached, nil
	}

	var count int
	err = s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM chat_members WHERE chat_id = $1 AND user_id = $2 AND role != 'banned'`,
		chatID, userID).Scan(&count)

	isMember := err == nil && count > 0

	if err == nil {
		s.redis.Set(ctx, cacheKey, isMember, 30*time.Second)
	}

	return isMember, nil
}

func (s *MessageService) getMemberRole(ctx context.Context, chatID, userID string) (string, error) {
	var role string
	err := s.db.QueryRow(ctx,
		`SELECT role FROM chat_members WHERE chat_id = $1 AND user_id = $2`,
		chatID, userID).Scan(&role)
	return role, err
}

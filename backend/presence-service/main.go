package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/iraq-secure-chat/common/config"
	"github.com/iraq-secure-chat/common/logging"
	"github.com/iraq-secure-chat/common/types"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type PresenceService struct {
	cfg    *config.Config
	logger *zap.Logger
	redis  *redis.Client
}

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

	svc := &PresenceService{
		cfg:    cfg,
		logger: logger,
		redis:  redisClient,
	}

	app := fiber.New(fiber.Config{
		AppName: "IraqSecureChat Presence Service",
	})

	// Subscribe to Redis keyspace notifications for expired online keys
	go svc.watchPresence(context.Background())

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		logger.Info("shutting down presence service")
		app.ShutdownWithTimeout(cfg.Server.GracefulTimeout)
	}()

	logger.Info("presence service starting", zap.String("addr", cfg.Addr()))
	if err := app.Listen(cfg.Addr()); err != nil {
		logger.Fatal("server error", zap.Error(err))
	}
}

func (s *PresenceService) watchPresence(ctx context.Context) {
	config := &redis.ResetAll()
	pubsub := s.redis.PSubscribe(ctx, "__keyevent@0__:expired")
	defer pubsub.Close()

	ch := pubsub.Channel()
	for msg := range ch {
		if len(msg.Payload) > 5 && strings.HasPrefix(msg.Payload, "user:") && strings.HasSuffix(msg.Payload, ":online") {
			userID := msg.Payload[5 : len(msg.Payload)-7]

			now := time.Now()
			lastSeenKey := fmt.Sprintf("user:%s:last_seen", userID)
			s.redis.Set(ctx, lastSeenKey, now.Unix(), 0)

			parsedUUID, err := uuid.Parse(userID)
			if err != nil {
				continue
			}

			event := types.PresenceEvent{
				UserID:   types.UserID(parsedUUID),
				Status:   types.PresenceOffline,
				LastSeen: now.Unix(),
			}
			data, _ := json.Marshal(event)
			s.redis.Publish(ctx, "presence", data)

			s.logger.Debug("user went offline",
				zap.String("user_id", userID),
				zap.Time("last_seen", now),
			)
		}
	}
}

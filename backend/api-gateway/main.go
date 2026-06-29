package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/healthcheck"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/gofiber/fiber/v2/middleware/timeout"
	"github.com/iraq-secure-chat/common/config"
	"github.com/iraq-secure-chat/common/logging"
	"github.com/iraq-secure-chat/common/middleware"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

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
	logger.Info("redis connected")

	app := fiber.New(fiber.Config{
		AppName:           "IraqSecureChat API Gateway",
		ReadTimeout:       cfg.Server.ReadTimeout,
		WriteTimeout:      cfg.Server.WriteTimeout,
		BodyLimit:         cfg.Server.BodyLimit * 1024 * 1024,
		ReduceMemoryUsage: true,
		Prefork:           cfg.Environment == "production",
		ErrorHandler:      errorHandler,
	})

	// Global middleware
	app.Use(recover.New())
	app.Use(requestid.New())
	app.Use(compress.New(compress.Config{Level: compress.LevelBestSpeed}))
	app.Use(cors.New(cors.Config{
		AllowOrigins:     "*",
		AllowMethods:     "GET,POST,PUT,DELETE,PATCH,OPTIONS",
		AllowHeaders:     "Authorization,Content-Type,X-Idempotency-Key,X-Device-Info",
		AllowCredentials: true,
		MaxAge:           86400,
	}))
	app.Use(healthcheck.New())

	// Rate limiters
	apiRateLimiter := middleware.NewRateLimiter(
		redisClient, "ratelimit:api", cfg.RateLimit.APICallsPerUser, cfg.RateLimit.APICallsWindow,
	)

	// API routes
	api := app.Group("/v1", apiRateLimiter.Handler)

	// Health check
	api.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"ok": true, "data": fiber.Map{
			"service":     cfg.ServiceName,
			"environment": cfg.Environment,
			"timestamp":   time.Now().Unix(),
		}})
	})

	// Auth routes
	authGroup := api.Group("/auth")
	authGroup.Post("/send-otp", proxyToService(cfg, "auth-service:8091"))
	authGroup.Post("/verify-otp", proxyToService(cfg, "auth-service:8091"))
	authGroup.Post("/refresh", proxyToService(cfg, "auth-service:8091"))
	authGroup.Post("/logout", middleware.JWTAuth(cfg.JWT.AccessSecret), proxyToService(cfg, "auth-service:8091"))
	authGroup.Get("/sessions", middleware.JWTAuth(cfg.JWT.AccessSecret), proxyToService(cfg, "auth-service:8091"))
	authGroup.Post("/enable-2fa", middleware.JWTAuth(cfg.JWT.AccessSecret), proxyToService(cfg, "auth-service:8091"))
	authGroup.Post("/verify-2fa", proxyToService(cfg, "auth-service:8091"))

	// User routes
	userGroup := api.Group("/users", middleware.JWTAuth(cfg.JWT.AccessSecret))
	userGroup.Get("/me", proxyToService(cfg, "auth-service:8091"))
	userGroup.Put("/me", proxyToService(cfg, "auth-service:8091"))
	userGroup.Get("/:id", proxyToService(cfg, "auth-service:8091"))
	userGroup.Get("/search", proxyToService(cfg, "auth-service:8091"))
	userGroup.Post("/:id/contact", proxyToService(cfg, "auth-service:8091"))
	userGroup.Delete("/:id/contact", proxyToService(cfg, "auth-service:8091"))
	userGroup.Post("/:id/block", proxyToService(cfg, "auth-service:8091"))
	userGroup.Get("/:id/common-groups", proxyToService(cfg, "auth-service:8091"))
	userGroup.Post("/:id/report", proxyToService(cfg, "auth-service:8091"))

	// Chat routes
	chatGroup := api.Group("/chats", middleware.JWTAuth(cfg.JWT.AccessSecret))
	chatGroup.Get("/", proxyToService(cfg, "message-service:8092"))
	chatGroup.Post("/create-group", proxyToService(cfg, "message-service:8092"))
	chatGroup.Post("/create-channel", proxyToService(cfg, "message-service:8092"))
	chatGroup.Get("/:chatId", proxyToService(cfg, "message-service:8092"))
	chatGroup.Put("/:chatId", proxyToService(cfg, "message-service:8092"))
	chatGroup.Get("/:chatId/members", proxyToService(cfg, "message-service:8092"))
	chatGroup.Post("/:chatId/members", proxyToService(cfg, "message-service:8092"))
	chatGroup.Delete("/:chatId/members/:userId", proxyToService(cfg, "message-service:8092"))
	chatGroup.Post("/:chatId/invite-link", proxyToService(cfg, "message-service:8092"))
	chatGroup.Post("/:chatId/join", proxyToService(cfg, "message-service:8092"))
	chatGroup.Post("/:chatId/leave", proxyToService(cfg, "message-service:8092"))
	chatGroup.Post("/:chatId/export", proxyToService(cfg, "message-service:8092"))

	// Message routes
	msgGroup := chatGroup
	msgGroup.Get("/:chatId/messages", proxyToService(cfg, "message-service:8092"))
	msgGroup.Post("/:chatId/messages", proxyToService(cfg, "message-service:8092"))
	msgGroup.Put("/:chatId/messages/:msgId", proxyToService(cfg, "message-service:8092"))
	msgGroup.Delete("/:chatId/messages/:msgId", proxyToService(cfg, "message-service:8092"))
	msgGroup.Post("/:chatId/messages/:msgId/react", proxyToService(cfg, "message-service:8092"))
	msgGroup.Post("/:chatId/messages/:msgId/pin", proxyToService(cfg, "message-service:8092"))
	msgGroup.Post("/:chatId/messages/read", proxyToService(cfg, "message-service:8092"))
	msgGroup.Get("/:chatId/messages/search", proxyToService(cfg, "message-service:8092"))
	msgGroup.Post("/:chatId/messages/:msgId/forward", proxyToService(cfg, "message-service:8092"))
	msgGroup.Post("/:chatId/messages/:msgId/schedule", proxyToService(cfg, "message-service:8092"))
	msgGroup.Post("/:chatId/messages/:msgId/auto-delete", proxyToService(cfg, "message-service:8092"))

	// Media routes
	mediaGroup := api.Group("/media", middleware.JWTAuth(cfg.JWT.AccessSecret))
	mediaGroup.Post("/upload", proxyToService(cfg, "media-service:8093"))
	mediaGroup.Get("/:mediaId", proxyToService(cfg, "media-service:8093"))
	mediaGroup.Delete("/:mediaId", proxyToService(cfg, "media-service:8093"))

	// Call routes
	callGroup := api.Group("/calls", middleware.JWTAuth(cfg.JWT.AccessSecret))
	callGroup.Post("/initiate", proxyToService(cfg, "websocket-gateway:8095"))
	callGroup.Post("/:callId/accept", proxyToService(cfg, "websocket-gateway:8095"))
	callGroup.Post("/:callId/reject", proxyToService(cfg, "websocket-gateway:8095"))
	callGroup.Post("/:callId/end", proxyToService(cfg, "websocket-gateway:8095"))
	callGroup.Get("/:callId/ice-servers", proxyToService(cfg, "websocket-gateway:8095"))

	// Bot routes
	botGroup := api.Group("/bot")
	botGroup.Post("/:token/sendMessage", proxyToService(cfg, "bot-service:8096"))
	botGroup.Post("/:token/sendPhoto", proxyToService(cfg, "bot-service:8096"))
	botGroup.Post("/:token/sendVideo", proxyToService(cfg, "bot-service:8096"))
	botGroup.Post("/:token/sendAudio", proxyToService(cfg, "bot-service:8096"))
	botGroup.Post("/:token/sendDocument", proxyToService(cfg, "bot-service:8096"))
	botGroup.Post("/:token/sendSticker", proxyToService(cfg, "bot-service:8096"))
	botGroup.Post("/:token/sendPoll", proxyToService(cfg, "bot-service:8096"))
	botGroup.Post("/:token/sendLocation", proxyToService(cfg, "bot-service:8096"))
	botGroup.Post("/:token/sendContact", proxyToService(cfg, "bot-service:8096"))
	botGroup.Post("/:token/answerCallbackQuery", proxyToService(cfg, "bot-service:8096"))
	botGroup.Post("/:token/editMessageText", proxyToService(cfg, "bot-service:8096"))
	botGroup.Post("/:token/editMessageCaption", proxyToService(cfg, "bot-service:8096"))
	botGroup.Post("/:token/deleteMessage", proxyToService(cfg, "bot-service:8096"))
	botGroup.Post("/:token/sendChatAction", proxyToService(cfg, "bot-service:8096"))
	botGroup.Get("/:token/getMe", proxyToService(cfg, "bot-service:8096"))
	botGroup.Post("/:token/setWebhook", proxyToService(cfg, "bot-service:8096"))
	botGroup.Post("/:token/setMyCommands", proxyToService(cfg, "bot-service:8096"))
	botGroup.Post("/:token/answerInlineQuery", proxyToService(cfg, "bot-service:8096"))

	// AI routes
	aiGroup := api.Group("/ai", middleware.JWTAuth(cfg.JWT.AccessSecret))
	aiGroup.Post("/translate", proxyToService(cfg, "ai-service:8097"))
	aiGroup.Post("/moderate", proxyToService(cfg, "ai-service:8097"))
	aiGroup.Post("/suggest-reply", proxyToService(cfg, "ai-service:8097"))
	aiGroup.Post("/summarize", proxyToService(cfg, "ai-service:8097"))
	aiGroup.Post("/complete", proxyToService(cfg, "ai-service:8097"))

	// WebSocket upgrade
	api.Get("/ws", proxyToService(cfg, "websocket-gateway:8095"))

	// Graceful shutdown
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		logger.Info("shutting down api gateway")
		app.ShutdownWithTimeout(cfg.Server.GracefulTimeout)
	}()

	logger.Info("api gateway starting", zap.String("addr", cfg.Addr()))
	if err := app.Listen(cfg.Addr()); err != nil {
		logger.Fatal("server error", zap.Error(err))
	}
}

func errorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
	}

	logging.Get().Error("request error",
		zap.String("path", c.Path()),
		zap.Int("status", code),
		zap.Error(err),
	)

	return c.Status(code).JSON(fiber.Map{
		"ok": false,
		"error": fiber.Map{
			"code":    code,
			"message": err.Error(),
		},
	})
}

func proxyToService(cfg *config.Config, target string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		logging.Get().Info("proxy request",
			zap.String("target", target),
			zap.String("method", c.Method()),
			zap.String("path", c.Path()),
		)

		client := &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 100,
				IdleConnTimeout:     90 * time.Second,
			},
		}

		req, err := http.NewRequestWithContext(
			c.Context(),
			c.Method(),
			"http://"+target+c.OriginalURL(),
			c.Request().Body(),
		)
		if err != nil {
			return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
				"ok": false, "error": fiber.Map{"code": 502, "message": "proxy error"},
			})
		}

		c.Request().Header.VisitAll(func(key, value []byte) {
			req.Header.Set(string(key), string(value))
		})

		resp, err := client.Do(req)
		if err != nil {
			logging.Get().Error("proxy request failed",
				zap.String("target", target),
				zap.Error(err),
			)
			return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
				"ok": false, "error": fiber.Map{"code": 502, "message": "upstream unavailable"},
			})
		}
		defer resp.Body.Close()

		c.Status(resp.StatusCode)
		for k, vs := range resp.Header {
			for _, v := range vs {
				c.Set(k, v)
			}
		}
		return c.SendStream(resp.Body)
	}
}

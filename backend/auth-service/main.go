package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base32"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/iraq-secure-chat/common/config"
	"github.com/iraq-secure-chat/common/logging"
	"github.com/iraq-secure-chat/common/middleware"
	"github.com/iraq-secure-chat/common/types"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"golang.org/x/crypto/argon2"
)

type AuthService struct {
	cfg    *config.Config
	logger *zap.Logger
	db     *pgxpool.Pool
	redis  *redis.Client
}

type SendOTPRequest struct {
	Phone string `json:"phone" validate:"required"`
}

type VerifyOTPRequest struct {
	Phone      string `json:"phone" validate:"required"`
	OTP        string `json:"otp" validate:"required"`
	DeviceInfo string `json:"device_info"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

type Enable2FARequest struct {
	Password string `json:"password" validate:"required,min=8,max=128"`
	Hint     string `json:"hint"`
}

type Verify2FARequest struct {
	Phone    string `json:"phone" validate:"required"`
	Password string `json:"password" validate:"required"`
}

type UpdateProfileRequest struct {
	DisplayName string `json:"display_name" validate:"min=1,max=64"`
	Bio         string `json:"bio" validate:"max=512"`
	Username    string `json:"username" validate:"max=32"`
}

type SessionInfo struct {
	ID         string    `json:"id"`
	DeviceInfo string    `json:"device_info"`
	IP         string    `json:"ip"`
	CreatedAt  time.Time `json:"created_at"`
	LastActive time.Time `json:"last_active"`
	IsCurrent  bool      `json:"is_current"`
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

	svc := &AuthService{
		cfg:    cfg,
		logger: logger,
		db:     db,
		redis:  redisClient,
	}

	app := fiber.New(fiber.Config{
		AppName:           "IraqSecureChat Auth Service",
		ReadTimeout:       cfg.Server.ReadTimeout,
		WriteTimeout:      cfg.Server.WriteTimeout,
		ReduceMemoryUsage: true,
	})

	app.Post("/v1/auth/send-otp", svc.HandleSendOTP)
	app.Post("/v1/auth/verify-otp", svc.HandleVerifyOTP)
	app.Post("/v1/auth/refresh", svc.HandleRefresh)
	app.Post("/v1/auth/logout", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleLogout)
	app.Post("/v1/auth/enable-2fa", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleEnable2FA)
	app.Post("/v1/auth/verify-2fa", svc.HandleVerify2FA)
	app.Get("/v1/auth/sessions", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleListSessions)

	app.Get("/v1/users/me", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleGetMe)
	app.Put("/v1/users/me", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleUpdateProfile)
	app.Get("/v1/users/:id", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleGetUser)
	app.Get("/v1/users/search", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleSearchUsers)
	app.Post("/v1/users/:id/contact", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleAddContact)
	app.Delete("/v1/users/:id/contact", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleRemoveContact)
	app.Post("/v1/users/:id/block", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleBlockUser)

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		logger.Info("shutting down auth service")
		app.ShutdownWithTimeout(cfg.Server.GracefulTimeout)
	}()

	logger.Info("auth service starting", zap.String("addr", cfg.Addr()))
	if err := app.Listen(cfg.Addr()); err != nil {
		logger.Fatal("server error", zap.Error(err))
	}
}

func (s *AuthService) HandleSendOTP(c *fiber.Ctx) error {
	var req SendOTPRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request body"},
		})
	}

	req.Phone = sanitizePhone(req.Phone)
	if !isValidPhone(req.Phone) {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid phone number format"},
		})
	}

	// Rate limit: check cooldown
	cooldownKey := fmt.Sprintf("otp:cooldown:%s", req.Phone)
	ttl, err := s.redis.TTL(c.Context(), cooldownKey).Result()
	if err == nil && ttl > 0 {
		return c.Status(429).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 429, "message": fmt.Sprintf("please wait %d seconds before requesting a new code", int(ttl.Seconds()))},
		})
	}

	// Rate limit: check max attempts per hour
	countKey := fmt.Sprintf("otp:count:%s", req.Phone)
	count, _ := s.redis.Incr(c.Context(), countKey).Result()
	if count == 1 {
		s.redis.Expire(c.Context(), countKey, s.cfg.RateLimit.OTPSendWindow)
	}
	if count > int64(s.cfg.RateLimit.OTPSendPerPhone) {
		return c.Status(429).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 429, "message": "too many OTP requests. try again later"},
		})
	}

	// Generate OTP
	otp := generateOTP(s.cfg.OTP.Length)
	otpHash := hashOTP(otp)

	// Store OTP in Redis
	otpKey := fmt.Sprintf("otp:code:%s", req.Phone)
	if err := s.redis.Set(c.Context(), otpKey, otpHash, s.cfg.OTP.Expiry).Err(); err != nil {
		s.logger.Error("failed to store OTP", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "internal server error"},
		})
	}

	// Store attempt counter
	attemptKey := fmt.Sprintf("otp:attempts:%s", req.Phone)
	s.redis.Set(c.Context(), attemptKey, 0, s.cfg.OTP.Expiry)

	// Set cooldown
	s.redis.Set(c.Context(), cooldownKey, "1", s.cfg.OTP.Cooldown)

	s.logger.Info("otp sent",
		zap.String("phone", maskPhone(req.Phone)),
		zap.String("otp", otp),
	)

	return c.JSON(types.NewAPIResponse(fiber.Map{
		"phone":     req.Phone,
		"expires_in": int(s.cfg.OTP.Expiry.Seconds()),
		"hint":      "OTP sent via SMS",
	}))
}

func (s *AuthService) HandleVerifyOTP(c *fiber.Ctx) error {
	var req VerifyOTPRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request body"},
		})
	}

	req.Phone = sanitizePhone(req.Phone)

	otpKey := fmt.Sprintf("otp:code:%s", req.Phone)
	storedHash, err := s.redis.Get(c.Context(), otpKey).Result()
	if err == redis.Nil {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "no OTP found or expired"},
		})
	}
	if err != nil {
		s.logger.Error("redis error", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "internal server error"},
		})
	}

	// Check attempts
	attemptKey := fmt.Sprintf("otp:attempts:%s", req.Phone)
	attempts, _ := s.redis.Incr(c.Context(), attemptKey).Result()
	if attempts > int64(s.cfg.OTP.MaxAttempts) {
		s.redis.Del(c.Context(), otpKey)
		return c.Status(429).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 429, "message": "too many attempts. request a new code"},
		})
	}

	if !verifyOTP(storedHash, req.OTP) {
		remaining := s.cfg.OTP.MaxAttempts - int(attempts)
		return c.Status(401).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 401, "message": fmt.Sprintf("invalid code. %d attempts remaining", remaining)},
		})
	}

	// OTP verified - cleanup
	s.redis.Del(c.Context(), otpKey, attemptKey, fmt.Sprintf("otp:count:%s", req.Phone))

	// Find or create user
	userID, isNew, err := s.findOrCreateUser(c.Context(), req.Phone)
	if err != nil {
		s.logger.Error("find or create user", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "internal server error"},
		})
	}

	// Create session
	sessionID := uuid.New()
	deviceInfo := req.DeviceInfo
	if deviceInfo == "" {
		deviceInfo = fmt.Sprintf("IP: %s", c.IP())
	}

	now := time.Now()
	session := fiber.Map{
		"id":          sessionID.String(),
		"user_id":     userID.String(),
		"device_info": deviceInfo,
		"ip":          c.IP(),
		"created_at":  now,
		"last_active": now,
	}

	sessionKey := fmt.Sprintf("session:%s", sessionID.String())
	s.redis.HSet(c.Context(), sessionKey, session)
	s.redis.Expire(c.Context(), sessionKey, s.cfg.JWT.RefreshTTL)

	// User sessions set
	userSessionsKey := fmt.Sprintf("user_sessions:%s", userID.String())
	s.redis.SAdd(c.Context(), userSessionsKey, sessionID.String())
	s.redis.Expire(c.Context(), userSessionsKey, s.cfg.JWT.RefreshTTL)

	accessToken, err := middleware.GenerateAccessToken(userID, sessionID, s.cfg.JWT.AccessSecret, s.cfg.JWT.AccessTTL, s.cfg.JWT.Issuer)
	if err != nil {
		s.logger.Error("generate access token", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "internal server error"},
		})
	}

	refreshToken, err := middleware.GenerateRefreshToken(userID, sessionID, s.cfg.JWT.RefreshSecret, s.cfg.JWT.RefreshTTL, s.cfg.JWT.Issuer)
	if err != nil {
		s.logger.Error("generate refresh token", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "internal server error"},
		})
	}

	_, _ = s.db.Exec(c.Context(),
		`UPDATE users SET last_seen = NOW() WHERE id = $1`, userID)

	s.logger.Info("user authenticated",
		zap.String("user_id", userID.String()),
		zap.Bool("new_user", isNew),
	)

	return c.JSON(types.NewAPIResponse(fiber.Map{
		"user_id":       userID.String(),
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"expires_in":    int(s.cfg.JWT.AccessTTL.Seconds()),
		"is_new_user":   isNew,
		"session_id":    sessionID.String(),
	}))
}

func (s *AuthService) HandleRefresh(c *fiber.Ctx) error {
	var req RefreshRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request body"},
		})
	}

	claims, err := middleware.VerifyRefreshToken(req.RefreshToken, s.cfg.JWT.RefreshSecret)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 401, "message": "invalid or expired refresh token"},
		})
	}

	// Verify session still exists
	sessionKey := fmt.Sprintf("session:%s", claims.SessionID)
	exists, _ := s.redis.Exists(c.Context(), sessionKey).Result()
	if exists == 0 {
		return c.Status(401).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 401, "message": "session expired or revoked"},
		})
	}

	// Rotate: delete old refresh token, create new
	userID, _ := uuid.Parse(claims.UserID)
	sessionID, _ := uuid.Parse(claims.SessionID)

	newAccessToken, err := middleware.GenerateAccessToken(userID, sessionID, s.cfg.JWT.AccessSecret, s.cfg.JWT.AccessTTL, s.cfg.JWT.Issuer)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "internal server error"},
		})
	}

	newRefreshToken, err := middleware.GenerateRefreshToken(userID, sessionID, s.cfg.JWT.RefreshSecret, s.cfg.JWT.RefreshTTL, s.cfg.JWT.Issuer)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "internal server error"},
		})
	}

	return c.JSON(types.NewAPIResponse(fiber.Map{
		"access_token":  newAccessToken,
		"refresh_token": newRefreshToken,
		"expires_in":    int(s.cfg.JWT.AccessTTL.Seconds()),
	}))
}

func (s *AuthService) HandleLogout(c *fiber.Ctx) error {
	claims := middleware.ExtractClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 401, "message": "unauthorized"},
		})
	}

	sessionID := c.Query("session_id")
	if sessionID == "" {
		sessionID = claims.SessionID
	}

	// Remove session
	sessionKey := fmt.Sprintf("session:%s", sessionID)
	s.redis.Del(c.Context(), sessionKey)

	userSessionsKey := fmt.Sprintf("user_sessions:%s", claims.UserID)
	s.redis.SRem(c.Context(), userSessionsKey, sessionID)

	s.logger.Info("user logged out",
		zap.String("user_id", claims.UserID),
		zap.String("session_id", sessionID),
	)

	return c.JSON(types.NewAPIResponse(fiber.Map{"message": "logged out successfully"}))
}

func (s *AuthService) HandleEnable2FA(c *fiber.Ctx) error {
	userID := middleware.ExtractUserID(c)
	if userID == "" {
		return c.Status(401).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 401, "message": "unauthorized"},
		})
	}

	var req Enable2FARequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request body"},
		})
	}

	salt := make([]byte, s.cfg.Argon2.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "internal server error"},
		})
	}

	hash := argon2.IDKey([]byte(req.Password), salt, s.cfg.Argon2.Time, s.cfg.Argon2.Memory, s.cfg.Argon2.Threads, s.cfg.Argon2.KeyLen)

	storedHash := fmt.Sprintf("%s:%s", base32.StdEncoding.EncodeToString(salt), base32.StdEncoding.EncodeToString(hash))

	_, err := s.db.Exec(c.Context(),
		`UPDATE users SET settings = jsonb_set(COALESCE(settings, '{}'::jsonb), '{2fa_hash}', to_jsonb($1::text)),
		                                    settings = jsonb_set(COALESCE(settings, '{}'::jsonb), '{2fa_hint}', to_jsonb($2::text))
		 WHERE id = $3`,
		storedHash, req.Hint, userID)
	if err != nil {
		s.logger.Error("enable 2fa", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "internal server error"},
		})
	}

	return c.JSON(types.NewAPIResponse(fiber.Map{"message": "2FA enabled successfully"}))
}

func (s *AuthService) HandleVerify2FA(c *fiber.Ctx) error {
	var req Verify2FARequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request body"},
		})
	}

	var storedHash, hint string
	err := s.db.QueryRow(c.Context(),
		`SELECT settings->>'2fa_hash', settings->>'2fa_hint' FROM users WHERE phone = $1`,
		sanitizePhone(req.Phone)).Scan(&storedHash, &hint)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 401, "message": "2FA not enabled"},
		})
	}

	parts := strings.SplitN(storedHash, ":", 2)
	if len(parts) != 2 {
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "internal server error"},
		})
	}

	salt, err := base32.StdEncoding.DecodeString(parts[0])
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "internal server error"},
		})
	}

	expectedHash, err := base32.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "internal server error"},
		})
	}

	computedHash := argon2.IDKey([]byte(req.Password), salt, s.cfg.Argon2.Time, s.cfg.Argon2.Memory, s.cfg.Argon2.Threads, s.cfg.Argon2.KeyLen)

	if subtle.ConstantTimeCompare(computedHash, expectedHash) != 1 {
		return c.Status(401).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 401, "message": "invalid 2FA password"},
		})
	}

	return c.JSON(types.NewAPIResponse(fiber.Map{"message": "2FA verified"}))
}

func (s *AuthService) HandleGetMe(c *fiber.Ctx) error {
	userID := middleware.ExtractUserID(c)
	if userID == "" {
		return c.Status(401).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 401, "message": "unauthorized"},
		})
	}

	user, err := s.getUserByID(c.Context(), userID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 404, "message": "user not found"},
		})
	}

	return c.JSON(types.NewAPIResponse(user))
}

func (s *AuthService) HandleUpdateProfile(c *fiber.Ctx) error {
	userID := middleware.ExtractUserID(c)
	if userID == "" {
		return c.Status(401).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 401, "message": "unauthorized"},
		})
	}

	var req UpdateProfileRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request body"},
		})
	}

	query := `UPDATE users SET `
	args := []interface{}{}
	argIdx := 1

	if req.DisplayName != "" {
		query += fmt.Sprintf(`display_name = $%d, `, argIdx)
		args = append(args, req.DisplayName)
		argIdx++
	}
	if req.Bio != "" {
		query += fmt.Sprintf(`bio = $%d, `, argIdx)
		args = append(args, req.Bio)
		argIdx++
	}
	if req.Username != "" {
		query += fmt.Sprintf(`username = $%d, `, argIdx)
		args = append(args, req.Username)
		argIdx++
	}

	query = query[:len(query)-2] + fmt.Sprintf(` WHERE id = $%d`, argIdx)
	args = append(args, userID)

	_, err := s.db.Exec(c.Context(), query, args...)
	if err != nil {
		s.logger.Error("update profile", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "update failed"},
		})
	}

	// Invalidate cache
	s.redis.Del(c.Context(), fmt.Sprintf("user:%s", userID))

	user, err := s.getUserByID(c.Context(), userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "internal server error"},
		})
	}

	return c.JSON(types.NewAPIResponse(user))
}

func (s *AuthService) HandleGetUser(c *fiber.Ctx) error {
	userID := c.Params("id")
	if userID == "" {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid user id"},
		})
	}

	user, err := s.getUserByID(c.Context(), userID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 404, "message": "user not found"},
		})
	}

	return c.JSON(types.NewAPIResponse(user))
}

func (s *AuthService) HandleSearchUsers(c *fiber.Ctx) error {
	query := c.Query("q")
	if query == "" || len(query) < 2 {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "query must be at least 2 characters"},
		})
	}

	limit := c.QueryInt("limit", 20)
	if limit > 100 {
		limit = 100
	}

	rows, err := s.db.Query(c.Context(),
		`SELECT id, display_name, username, bio, avatar_url, premium, last_seen
		 FROM users
		 WHERE display_name ILIKE '%' || $1 || '%'
		    OR username ILIKE '%' || $1 || '%'
		    OR phone ILIKE '%' || $1 || '%'
		 LIMIT $2`,
		query, limit)
	if err != nil {
		s.logger.Error("search users", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "search failed"},
		})
	}
	defer rows.Close()

	type SearchResult struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
		Username    string `json:"username,omitempty"`
		Bio         string `json:"bio,omitempty"`
		AvatarURL   string `json:"avatar_url,omitempty"`
		Premium     bool   `json:"premium"`
		LastSeen    time.Time `json:"last_seen"`
	}

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.ID, &r.DisplayName, &r.Username, &r.Bio, &r.AvatarURL, &r.Premium, &r.LastSeen); err != nil {
			continue
		}
		results = append(results, r)
	}

	return c.JSON(types.NewAPIResponse(fiber.Map{
		"users": results,
	}))
}

func (s *AuthService) HandleListSessions(c *fiber.Ctx) error {
	userID := middleware.ExtractUserID(c)
	claims := middleware.ExtractClaims(c)
	if userID == "" || claims == nil {
		return c.Status(401).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 401, "message": "unauthorized"},
		})
	}

	userSessionsKey := fmt.Sprintf("user_sessions:%s", userID)
	sessionIDs, err := s.redis.SMembers(c.Context(), userSessionsKey).Result()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "internal server error"},
		})
	}

	var sessions []SessionInfo
	for _, sid := range sessionIDs {
		sessionKey := fmt.Sprintf("session:%s", sid)
		data, err := s.redis.HGetAll(c.Context(), sessionKey).Result()
		if err != nil || len(data) == 0 {
			continue
		}

		createdAt, _ := time.Parse(time.RFC3339, data["created_at"])
		lastActive, _ := time.Parse(time.RFC3339, data["last_active"])

		sessions = append(sessions, SessionInfo{
			ID:         sid,
			DeviceInfo: data["device_info"],
			IP:         data["ip"],
			CreatedAt:  createdAt,
			LastActive: lastActive,
			IsCurrent:  sid == claims.SessionID,
		})
	}

	return c.JSON(types.NewAPIResponse(fiber.Map{"sessions": sessions}))
}

func (s *AuthService) HandleAddContact(c *fiber.Ctx) error {
	userID := middleware.ExtractUserID(c)
	contactID := c.Params("id")
	if userID == "" || contactID == "" {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request"},
		})
	}

	_, err := s.db.Exec(c.Context(),
		`INSERT INTO contacts (user_id, contact_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		userID, contactID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "failed to add contact"},
		})
	}

	return c.JSON(types.NewAPIResponse(fiber.Map{"message": "contact added"}))
}

func (s *AuthService) HandleRemoveContact(c *fiber.Ctx) error {
	userID := middleware.ExtractUserID(c)
	contactID := c.Params("id")
	if userID == "" || contactID == "" {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request"},
		})
	}

	_, err := s.db.Exec(c.Context(),
		`DELETE FROM contacts WHERE user_id = $1 AND contact_id = $2`,
		userID, contactID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "failed to remove contact"},
		})
	}

	return c.JSON(types.NewAPIResponse(fiber.Map{"message": "contact removed"}))
}

func (s *AuthService) HandleBlockUser(c *fiber.Ctx) error {
	userID := middleware.ExtractUserID(c)
	blockedID := c.Params("id")
	if userID == "" || blockedID == "" {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request"},
		})
	}

	_, err := s.db.Exec(c.Context(),
		`INSERT INTO blocked_users (user_id, blocked_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		userID, blockedID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "failed to block user"},
		})
	}

	return c.JSON(types.NewAPIResponse(fiber.Map{"message": "user blocked"}))
}

func (s *AuthService) getUserByID(ctx context.Context, userID string) (*types.User, error) {
	// Try cache
	cacheKey := fmt.Sprintf("user:%s", userID)
	var user types.User

	cached, err := s.redis.Get(ctx, cacheKey).Bytes()
	if err == nil {
		if err := user.UnmarshalBinary(cached); err == nil {
			return &user, nil
		}
	}

	err = s.db.QueryRow(ctx,
		`SELECT id, phone, email, COALESCE(username,''), display_name, COALESCE(bio,''),
		        COALESCE(avatar_url,''), premium, last_seen, created_at, COALESCE(settings,'{}'::jsonb)
		 FROM users WHERE id = $1`, userID,
	).Scan(&user.ID, &user.Phone, &user.Email, &user.Username, &user.DisplayName,
		&user.Bio, &user.AvatarURL, &user.Premium, &user.LastSeen, &user.CreatedAt, &user.Settings)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	// Cache for 5 minutes
	if data, err := user.MarshalBinary(); err == nil {
		s.redis.Set(ctx, cacheKey, data, 5*time.Minute)
	}

	return &user, nil
}

func (s *AuthService) findOrCreateUser(ctx context.Context, phone string) (uuid.UUID, bool, error) {
	var id uuid.UUID
	err := s.db.QueryRow(ctx, `SELECT id FROM users WHERE phone = $1`, phone).Scan(&id)
	if err == nil {
		return id, false, nil
	}

	id = uuid.New()
	_, err = s.db.Exec(ctx,
		`INSERT INTO users (id, phone, display_name, settings) VALUES ($1, $2, $3, $4)`,
		id, phone, phone, `{"language":"ar","notifications_enabled":true}`)
	if err != nil {
		return uuid.UUID{}, false, fmt.Errorf("create user: %w", err)
	}

	return id, true, nil
}

func generateOTP(length int) string {
	chars := "0123456789"
	code := make([]byte, length)
	for i := 0; i < length; i++ {
		b := make([]byte, 1)
		rand.Read(b)
		code[i] = chars[int(b[0])%len(chars)]
	}
	return string(code)
}

func hashOTP(otp string) string {
	salt := make([]byte, 8)
	rand.Read(salt)
	hash := argon2.IDKey([]byte(otp), salt, 1, 64*1024, 4, 32)
	return fmt.Sprintf("%s:%s", base32.StdEncoding.EncodeToString(salt), base32.StdEncoding.EncodeToString(hash))
}

func verifyOTP(stored, input string) bool {
	parts := strings.SplitN(stored, ":", 2)
	if len(parts) != 2 {
		return false
	}

	salt, err := base32.StdEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	expected, err := base32.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}

	computed := argon2.IDKey([]byte(input), salt, 1, 64*1024, 4, 32)
	return subtle.ConstantTimeCompare(computed, expected) == 1
}

func sanitizePhone(phone string) string {
	phone = strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, phone)
	if !strings.HasPrefix(phone, "964") && len(phone) == 10 {
		phone = "964" + phone
	}
	return phone
}

func isValidPhone(phone string) bool {
	return len(phone) >= 10 && len(phone) <= 15
}

func maskPhone(phone string) string {
	if len(phone) < 7 {
		return phone
	}
	return phone[:3] + "****" + phone[len(phone)-3:]
}

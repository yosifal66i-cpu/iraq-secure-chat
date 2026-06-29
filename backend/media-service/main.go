package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/iraq-secure-chat/common/config"
	"github.com/iraq-secure-chat/common/logging"
	"github.com/iraq-secure-chat/common/middleware"
	"github.com/iraq-secure-chat/common/types"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

type MediaService struct {
	cfg    *config.Config
	logger *zap.Logger
	db     *pgxpool.Pool
	redis  *redis.Client
	kafka  *kafka.Writer

	uploadDir string
}

type MediaRecord struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	Type         string    `json:"type"`
	MimeType     string    `json:"mime_type"`
	FileName     string    `json:"file_name"`
	FileSize     int64     `json:"file_size"`
	URL          string    `json:"url"`
	ThumbnailURL string    `json:"thumbnail_url,omitempty"`
	Width        int       `json:"width,omitempty"`
	Height       int       `json:"height,omitempty"`
	Duration     int       `json:"duration,omitempty"`
	Status       string    `json:"status"` // uploading, processing, ready, failed
	CreatedAt    time.Time `json:"created_at"`
}

var allowedMimeTypes = map[string]string{
	"image/jpeg":      "photo",
	"image/png":       "photo",
	"image/gif":       "gif",
	"image/webp":      "photo",
	"video/mp4":       "video",
	"video/quicktime": "video",
	"video/x-matroska": "video",
	"audio/mpeg":      "audio",
	"audio/ogg":       "audio",
	"audio/wav":       "audio",
	"audio/webm":      "audio",
	"application/pdf": "file",
	"application/zip": "file",
	"text/plain":      "file",
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

	uploadDir := "/data/uploads"
	if cfg.Environment == "development" {
		uploadDir = "./uploads"
	}
	os.MkdirAll(uploadDir, 0755)

	svc := &MediaService{
		cfg:       cfg,
		logger:    logger,
		db:        db,
		redis:     redisClient,
		uploadDir: uploadDir,
	}

	app := fiber.New(fiber.Config{
		AppName:      "IraqSecureChat Media Service",
		ReadTimeout:  5 * time.Minute, // Long timeout for uploads
		WriteTimeout: 5 * time.Minute,
		BodyLimit:    int(cfg.S3.MaxUploadSize),
	})

	app.Post("/v1/media/upload", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleUpload)
	app.Get("/v1/media/:mediaId", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleGetMedia)
	app.Delete("/v1/media/:mediaId", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleDeleteMedia)

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		logger.Info("shutting down media service")
		app.ShutdownWithTimeout(cfg.Server.GracefulTimeout)
	}()

	logger.Info("media service starting", zap.String("addr", cfg.Addr()))
	if err := app.Listen(cfg.Addr()); err != nil {
		logger.Fatal("server error", zap.Error(err))
	}
}

func (s *MediaService) HandleUpload(c *fiber.Ctx) error {
	userID := middleware.ExtractUserID(c)
	if userID == "" {
		return c.Status(401).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 401, "message": "unauthorized"},
		})
	}

	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "file is required"},
		})
	}

	if file.Size > s.cfg.S3.MaxUploadSize {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400,
				"message": fmt.Sprintf("file too large. max size: %d bytes", s.cfg.S3.MaxUploadSize)},
		})
	}

	// Read first 512 bytes to detect MIME type by content (magic bytes)
	src, err := file.Open()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "failed to read file"},
		})
	}
	defer src.Close()

	buf := make([]byte, 512)
	_, err = src.Read(buf)
	if err != nil && err != io.EOF {
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "failed to read file header"},
		})
	}

	mimeType := http.DetectContentType(buf)
	mediaType, allowed := allowedMimeTypes[mimeType]
	if !allowed {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": fmt.Sprintf("file type %s not allowed", mimeType)},
		})
	}

	// Reset file pointer
	src.Seek(0, 0)

	mediaID := uuid.New().String()
	ext := filepath.Ext(file.Filename)
	safeFileName := mediaID + ext
	filePath := filepath.Join(s.uploadDir, safeFileName)

	// Save file
	dst, err := os.Create(filePath)
	if err != nil {
		s.logger.Error("create file failed", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "failed to save file"},
		})
	}
	defer dst.Close()

	written, err := io.Copy(dst, src)
	if err != nil {
		s.logger.Error("write file failed", zap.Error(err))
		os.Remove(filePath)
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "failed to save file"},
		})
	}

	width, height := 0, 0
	if mediaType == "photo" {
		src.Seek(0, 0)
		img, _, err := image.Decode(bytes.NewReader(buf))
		if err == nil {
			width = img.Bounds().Dx()
			height = img.Bounds().Dy()
		}
	}

	// Store metadata
	now := time.Now()
	_, err = s.db.Exec(c.Context(),
		`INSERT INTO media (id, user_id, type, mime_type, file_name, file_size, url, width, height, status, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'ready', $10)`,
		mediaID, userID, mediaType, mimeType, file.Filename, written,
		fmt.Sprintf("/media/%s", mediaID), width, height, now)
	if err != nil {
		s.logger.Error("save media metadata failed", zap.Error(err))
		os.Remove(filePath)
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "failed to save metadata"},
		})
	}

	s.logger.Info("file uploaded",
		zap.String("media_id", mediaID),
		zap.String("type", mediaType),
		zap.Int64("size", written),
		zap.String("user_id", userID),
	)

	return c.JSON(types.NewAPIResponse(fiber.Map{
		"media_id": mediaID,
		"type":     mediaType,
		"mime_type": mimeType,
		"file_name": file.Filename,
		"file_size": written,
		"url":      fmt.Sprintf("/media/%s", mediaID),
		"width":    width,
		"height":   height,
	}))
}

func (s *MediaService) HandleGetMedia(c *fiber.Ctx) error {
	mediaID := c.Params("mediaId")
	if mediaID == "" {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid media id"},
		})
	}

	var media MediaRecord
	err := s.db.QueryRow(c.Context(),
		`SELECT id, user_id, type, mime_type, file_name, file_size, url,
		        COALESCE(thumbnail_url,''), width, height, duration, status, created_at
		 FROM media WHERE id = $1`, mediaID,
	).Scan(&media.ID, &media.UserID, &media.Type, &media.MimeType, &media.FileName,
		&media.FileSize, &media.URL, &media.ThumbnailURL, &media.Width, &media.Height,
		&media.Duration, &media.Status, &media.CreatedAt)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 404, "message": "media not found"},
		})
	}

	return c.JSON(types.NewAPIResponse(media))
}

func (s *MediaService) HandleDeleteMedia(c *fiber.Ctx) error {
	userID := middleware.ExtractUserID(c)
	mediaID := c.Params("mediaId")
	if userID == "" || mediaID == "" {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request"},
		})
	}

	var ownerID string
	err := s.db.QueryRow(c.Context(),
		`SELECT user_id FROM media WHERE id = $1`, mediaID).Scan(&ownerID)
	if err != nil || ownerID != userID {
		return c.Status(403).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 403, "message": "can only delete your own media"},
		})
	}

	_, err = s.db.Exec(c.Context(), `DELETE FROM media WHERE id = $1`, mediaID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "delete failed"},
		})
	}

	// Clean up files
	extensions := []string{".jpg", ".png", ".gif", ".webp", ".mp4", ".pdf"}
	for _, ext := range extensions {
		os.Remove(filepath.Join(s.uploadDir, mediaID+ext))
	}

	return c.JSON(types.NewAPIResponse(fiber.Map{"message": "media deleted"}))
}

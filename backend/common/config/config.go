package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	ServiceName string
	Environment string // development, staging, production
	LogLevel    string

	Server   ServerConfig
	Database DatabaseConfig
	Cache    CacheConfig
	Kafka    KafkaConfig
	Search   SearchConfig
	S3       S3Config

	JWT     JWTConfig
	OTP     OTPConfig
	Argon2  Argon2Config
	CORS    CORSConfig
	RateLimit RateLimitConfig

	AI       AIConfig
	WebRTC   WebRTCConfig
	Push     PushConfig
}

type ServerConfig struct {
	Host            string
	Port            int
	GracefulTimeout time.Duration
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	BodyLimit       int // MB
}

type DatabaseConfig struct {
	PostgresDSN  string
	CassandraHosts []string
	CassandraKeyspace string
	MaxOpenConns int
	MaxIdleConns int
	ConnMaxLifetime time.Duration
}

type CacheConfig struct {
	RedisURL      string
	RedisPassword string
	RedisDB       int
	PoolSize      int
	MinIdleConns  int
}

type KafkaConfig struct {
	Brokers     []string
	TopicPrefix string
	ConsumerGroup string
	BatchSize   int
	BatchTimeout time.Duration
}

type SearchConfig struct {
	ElasticsearchURLs []string
	MeilisearchURL    string
	MeilisearchAPIKey string
}

type S3Config struct {
	Endpoint     string
	Region       string
	Bucket       string
	AccessKey    string
	SecretKey    string
	UseSSL       bool
	PresignedExpiry time.Duration
	MaxUploadSize  int64 // bytes
}

type JWTConfig struct {
	AccessSecret      string
	RefreshSecret     string
	AccessTTL         time.Duration
	RefreshTTL        time.Duration
	Issuer            string
}

type OTPConfig struct {
	Length       int
	Expiry       time.Duration
	MaxAttempts  int
	Cooldown     time.Duration
	ResendDelay  time.Duration
}

type Argon2Config struct {
	Time    uint32
	Memory  uint32
	Threads uint8
	KeyLen  uint32
	SaltLen int
}

type CORSConfig struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	MaxAge           int
}

type RateLimitConfig struct {
	OTPSendPerPhone   int
	OTPSendWindow     time.Duration
	AuthAttemptsPerIP int
	AuthWindow        time.Duration
	MsgSendPerUser    int
	MsgSendWindow     time.Duration
	MediaUploadPerUser int
	MediaUploadWindow  time.Duration
	APICallsPerUser   int
	APICallsWindow    time.Duration
}

type AIConfig struct {
	Enabled        bool
	ModelPath      string
	Provider       string // local, openai, anthropic
	APIKey         string
	APIURL         string
	MaxTokens      int
	Temperature    float64
	ModerationEnabled bool
	NSFWThreshold  float64
	SpamThreshold  float64
	SmartReplyEnabled bool
	TranslationEnabled bool
}

type WebRTCConfig struct {
	STUNServers  []string
	TURNURL      string
	TURNUsername string
	TURNCredential string
}

type PushConfig struct {
	FCMKey     string
	APNsKey    string
	APNsTeamID string
	APNsKeyID  string
	VAPIDPublic  string
	VAPIDPrivate string
}

func Load() *Config {
	return &Config{
		ServiceName: getEnv("SERVICE_NAME", "unknown"),
		Environment: getEnv("ENVIRONMENT", "development"),
		LogLevel:    getEnv("LOG_LEVEL", "info"),
		Server: ServerConfig{
			Host:            getEnv("SERVER_HOST", "0.0.0.0"),
			Port:            getEnvInt("SERVER_PORT", 8080),
			GracefulTimeout: getEnvDuration("GRACEFUL_TIMEOUT", 30),
			ReadTimeout:     getEnvDuration("READ_TIMEOUT", 15),
			WriteTimeout:    getEnvDuration("WRITE_TIMEOUT", 30),
			BodyLimit:       getEnvInt("BODY_LIMIT_MB", 10),
		},
		Database: DatabaseConfig{
			PostgresDSN:     getEnv("POSTGRES_DSN", "postgres://user:pass@localhost:5432/iraqchat?sslmode=require"),
			CassandraHosts:  getEnvSlice("CASSANDRA_HOSTS", []string{"localhost:9042"}),
			CassandraKeyspace: getEnv("CASSANDRA_KEYSPACE", "iraqchat"),
			MaxOpenConns:    getEnvInt("DB_MAX_OPEN_CONNS", 50),
			MaxIdleConns:    getEnvInt("DB_MAX_IDLE_CONNS", 10),
			ConnMaxLifetime: getEnvDuration("DB_CONN_MAX_LIFETIME", 3600),
		},
		Cache: CacheConfig{
			RedisURL:      getEnv("REDIS_URL", "redis://localhost:6379"),
			RedisPassword: getEnv("REDIS_PASSWORD", ""),
			RedisDB:       getEnvInt("REDIS_DB", 0),
			PoolSize:      getEnvInt("REDIS_POOL_SIZE", 100),
			MinIdleConns:  getEnvInt("REDIS_MIN_IDLE", 10),
		},
		Kafka: KafkaConfig{
			Brokers:       getEnvSlice("KAFKA_BROKERS", []string{"localhost:9092"}),
			TopicPrefix:   getEnv("KAFKA_TOPIC_PREFIX", "iraqchat"),
			ConsumerGroup: getEnv("KAFKA_CONSUMER_GROUP", "iraqchat-group"),
			BatchSize:     getEnvInt("KAFKA_BATCH_SIZE", 100),
			BatchTimeout:  getEnvDuration("KAFKA_BATCH_TIMEOUT", 1),
		},
		Search: SearchConfig{
			ElasticsearchURLs: getEnvSlice("ELASTICSEARCH_URLS", []string{"http://localhost:9200"}),
			MeilisearchURL:    getEnv("MEILISEARCH_URL", "http://localhost:7700"),
			MeilisearchAPIKey: getEnv("MEILISEARCH_API_KEY", ""),
		},
		S3: S3Config{
			Endpoint:        getEnv("S3_ENDPOINT", "http://localhost:9000"),
			Region:          getEnv("S3_REGION", "me-south-1"),
			Bucket:          getEnv("S3_BUCKET", "iraqchat-media"),
			AccessKey:       getEnv("S3_ACCESS_KEY", "minioadmin"),
			SecretKey:       getEnv("S3_SECRET_KEY", "minioadmin"),
			UseSSL:          getEnvBool("S3_USE_SSL", false),
			PresignedExpiry: getEnvDuration("S3_PRESIGNED_EXPIRY", 3600),
			MaxUploadSize:   getEnvInt64("S3_MAX_UPLOAD_SIZE", 2147483648), // 2GB
		},
		JWT: JWTConfig{
			AccessSecret:  getEnv("JWT_ACCESS_SECRET", "change-me-access-secret-32-chars!"),
			RefreshSecret: getEnv("JWT_REFRESH_SECRET", "change-me-refresh-secret-32-chars!"),
			AccessTTL:     getEnvDuration("JWT_ACCESS_TTL", 15),
			RefreshTTL:    getEnvDuration("JWT_REFRESH_TTL", 43200), // 30 days
			Issuer:        getEnv("JWT_ISSUER", "iraq-secure-chat"),
		},
		OTP: OTPConfig{
			Length:       getEnvInt("OTP_LENGTH", 6),
			Expiry:       getEnvDuration("OTP_EXPIRY", 3),
			MaxAttempts:  getEnvInt("OTP_MAX_ATTEMPTS", 5),
			Cooldown:     getEnvDuration("OTP_COOLDOWN", 60),
			ResendDelay:  getEnvDuration("OTP_RESEND_DELAY", 30),
		},
		Argon2: Argon2Config{
			Time:    uint32(getEnvInt("ARGON2_TIME", 3)),
			Memory:  uint32(getEnvInt("ARGON2_MEMORY", 65536)),
			Threads: uint8(getEnvInt("ARGON2_THREADS", 4)),
			KeyLen:  uint32(getEnvInt("ARGON2_KEY_LEN", 32)),
			SaltLen: getEnvInt("ARGON2_SALT_LEN", 16),
		},
		CORS: CORSConfig{
			AllowedOrigins: getEnvSlice("CORS_ORIGINS", []string{"*"}),
			AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
			AllowedHeaders: []string{"Authorization", "Content-Type", "X-Idempotency-Key", "X-Device-Info"},
			MaxAge:         86400,
		},
		RateLimit: RateLimitConfig{
			OTPSendPerPhone:     getEnvInt("RATE_LIMIT_OTP_PER_PHONE", 3),
			OTPSendWindow:       getEnvDuration("RATE_LIMIT_OTP_WINDOW", 3600),
			AuthAttemptsPerIP:   getEnvInt("RATE_LIMIT_AUTH_PER_IP", 10),
			AuthWindow:          getEnvDuration("RATE_LIMIT_AUTH_WINDOW", 900),
			MsgSendPerUser:      getEnvInt("RATE_LIMIT_MSG_PER_USER", 60),
			MsgSendWindow:       getEnvDuration("RATE_LIMIT_MSG_WINDOW", 60),
			MediaUploadPerUser:  getEnvInt("RATE_LIMIT_MEDIA_PER_USER", 20),
			MediaUploadWindow:   getEnvDuration("RATE_LIMIT_MEDIA_WINDOW", 3600),
			APICallsPerUser:     getEnvInt("RATE_LIMIT_API_PER_USER", 1000),
			APICallsWindow:      getEnvDuration("RATE_LIMIT_API_WINDOW", 60),
		},
		AI: AIConfig{
			Enabled:            getEnvBool("AI_ENABLED", false),
			ModelPath:          getEnv("AI_MODEL_PATH", "/models/llama"),
			Provider:           getEnv("AI_PROVIDER", "local"),
			APIKey:             getEnv("AI_API_KEY", ""),
			APIURL:             getEnv("AI_API_URL", "http://localhost:11434/api/generate"),
			MaxTokens:          getEnvInt("AI_MAX_TOKENS", 2048),
			Temperature:        getEnvFloat("AI_TEMPERATURE", 0.7),
			ModerationEnabled:  getEnvBool("AI_MODERATION_ENABLED", true),
			NSFWThreshold:      getEnvFloat("AI_NSFW_THRESHOLD", 0.85),
			SpamThreshold:      getEnvFloat("AI_SPAM_THRESHOLD", 0.9),
			SmartReplyEnabled:  getEnvBool("AI_SMART_REPLY_ENABLED", true),
			TranslationEnabled: getEnvBool("AI_TRANSLATION_ENABLED", true),
		},
		WebRTC: WebRTCConfig{
			STUNServers:    getEnvSlice("STUN_SERVERS", []string{"stun:stun.l.google.com:19302"}),
			TURNURL:        getEnv("TURN_URL", "turn:turn.iraqchat.gov.iq:3478"),
			TURNUsername:   getEnv("TURN_USERNAME", "iraqchat"),
			TURNCredential: getEnv("TURN_CREDENTIAL", ""),
		},
		Push: PushConfig{
			FCMKey:       getEnv("FCM_SERVER_KEY", ""),
			APNsKey:      getEnv("APNS_KEY", ""),
			APNsTeamID:   getEnv("APNS_TEAM_ID", ""),
			APNsKeyID:    getEnv("APNS_KEY_ID", ""),
			VAPIDPublic:  getEnv("VAPID_PUBLIC_KEY", ""),
			VAPIDPrivate: getEnv("VAPID_PRIVATE_KEY", ""),
		},
	}
}

func (c *Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvInt64(key string, fallback int64) int64 {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.ParseInt(v, 10, 64); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

func getEnvFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}

func getEnvDuration(key string, fallback int) time.Duration {
	return time.Duration(getEnvInt(key, fallback)) * time.Second
}

func getEnvSlice(key string, fallback []string) []string {
	if v := os.Getenv(key); v != "" {
		return splitAndTrim(v, ",")
	}
	return fallback
}

func splitAndTrim(s, sep string) []string {
	var result []string
	for _, part := range splitAndTrimFunc(s, sep) {
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func splitAndTrimFunc(s, sep string) []string {
	var result []string
	current := ""
	for _, c := range s {
		if string(c) == sep {
			result = append(result, current)
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

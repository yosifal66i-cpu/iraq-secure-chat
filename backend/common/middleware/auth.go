package middleware

import (
	"context"
	"crypto/subtle"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type Claims struct {
	UserID    string   `json:"user_id"`
	SessionID string   `json:"session_id"`
	Roles     []string `json:"roles,omitempty"`
	jwt.RegisteredClaims
}

type ContextKey string

const (
	ClaimsKey ContextKey = "claims"
	UserIDKey ContextKey = "user_id"
)

func JWTAuth(accessSecret string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"ok": false, "error": fiber.Map{"code": 401, "message": "missing authorization header"},
			})
		}

		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenStr == authHeader {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"ok": false, "error": fiber.Map{"code": 401, "message": "invalid authorization format"},
			})
		}

		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(accessSecret), nil
		})

		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"ok": false, "error": fiber.Map{"code": 401, "message": "invalid or expired token"},
			})
		}

		if !token.Valid {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"ok": false, "error": fiber.Map{"code": 401, "message": "invalid token"},
			})
		}

		c.Locals(string(UserIDKey), claims.UserID)
		c.Locals(string(ClaimsKey), claims)
		return c.Next()
	}
}

func ExtractUserID(c *fiber.Ctx) string {
	if uid, ok := c.Locals(string(UserIDKey)).(string); ok {
		return uid
	}
	return ""
}

func ExtractClaims(c *fiber.Ctx) *Claims {
	if cl, ok := c.Locals(string(ClaimsKey)).(*Claims); ok {
		return cl
	}
	return nil
}

func GenerateAccessToken(userID uuid.UUID, sessionID uuid.UUID, secret string, ttl time.Duration, issuer string) (string, error) {
	now := time.Now()
	claims := &Claims{
		UserID:    userID.String(),
		SessionID: sessionID.String(),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    issuer,
			ID:        uuid.New().String(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func GenerateRefreshToken(userID uuid.UUID, sessionID uuid.UUID, secret string, ttl time.Duration, issuer string) (string, error) {
	now := time.Now()
	claims := &Claims{
		UserID:    userID.String(),
		SessionID: sessionID.String(),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    issuer,
			ID:        uuid.New().String(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func VerifyRefreshToken(tokenStr, refreshSecret string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(refreshSecret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("verify refresh token: %w", err)
	}
	if !token.Valid {
		return nil, fmt.Errorf("invalid refresh token")
	}
	return claims, nil
}

func SecureCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func CtxWithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, UserIDKey, userID)
}

func GetUserIDFromCtx(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(UserIDKey).(string)
	return id, ok
}

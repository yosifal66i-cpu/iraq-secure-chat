package middleware

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

type RateLimiter struct {
	client    *redis.Client
	prefix    string
	max       int
	window    time.Duration
}

func NewRateLimiter(client *redis.Client, prefix string, max int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		client: client,
		prefix: prefix,
		max:    max,
		window: window,
	}
}

func (rl *RateLimiter) Handler(c *fiber.Ctx) error {
	key := fmt.Sprintf("%s:%s", rl.prefix, c.IP())

	ctx := context.Background()
	now := time.Now().UnixMilli()
	windowStart := now - rl.window.Milliseconds()

	// Remove old entries
	rl.client.ZRemRangeByScore(ctx, key, "0", strconv.FormatInt(windowStart, 10))

	// Count current window entries
	count, err := rl.client.ZCard(ctx, key).Result()
	if err != nil {
		return c.Next()
	}

	if int(count) >= rl.max {
		ttl := rl.client.TTL(ctx, key).Val()
		c.Set("X-RateLimit-Limit", strconv.Itoa(rl.max))
		c.Set("X-RateLimit-Remaining", "0")
		c.Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(ttl).Unix(), 10))

		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 429, "message": "rate limit exceeded"},
		})
	}

	// Add current request
	member := &redis.Z{
		Score:  float64(now),
		Member: now,
	}
	rl.client.ZAdd(ctx, key, *member)
	rl.client.Expire(ctx, key, rl.window)

	remaining := rl.max - int(count) - 1
	c.Set("X-RateLimit-Limit", strconv.Itoa(rl.max))
	c.Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
	if remaining > 0 {
		c.Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(rl.window).Unix(), 10))
	}

	return c.Next()
}

type UserRateLimiter struct {
	client *redis.Client
	prefix string
	max    int
	window time.Duration
}

func NewUserRateLimiter(client *redis.Client, prefix string, max int, window time.Duration) *UserRateLimiter {
	return &UserRateLimiter{
		client: client,
		prefix: prefix,
		max:    max,
		window: window,
	}
}

func (rl *UserRateLimiter) Handler(c *fiber.Ctx) error {
	userID := ExtractUserID(c)
	if userID == "" {
		return c.Next()
	}

	key := fmt.Sprintf("%s:%s", rl.prefix, userID)

	ctx := context.Background()
	now := time.Now().UnixMilli()
	windowStart := now - rl.window.Milliseconds()

	pipe := rl.client.Pipeline()
	pipe.ZRemRangeByScore(ctx, key, "0", strconv.FormatInt(windowStart, 10))
	pipe.ZCard(ctx, key)
	cmders, err := pipe.Exec(ctx)
	if err != nil {
		return c.Next()
	}

	count := cmders[1].(*redis.IntCmd).Val()
	if int(count) >= rl.max {
		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 429, "message": "rate limit exceeded"},
		})
	}

	rl.client.ZAdd(ctx, key, &redis.Z{Score: float64(now), Member: now})
	rl.client.Expire(ctx, key, rl.window)

	return c.Next()
}

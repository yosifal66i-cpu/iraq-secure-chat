package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/iraq-secure-chat/common/config"
	"github.com/iraq-secure-chat/common/logging"
	"github.com/iraq-secure-chat/common/middleware"
	"github.com/iraq-secure-chat/common/types"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type AIService struct {
	cfg    *config.Config
	logger *zap.Logger
	redis  *redis.Client
	http   *http.Client
}

type ModerationRequest struct {
	Text     string `json:"text"`
	UserID   string `json:"user_id"`
	ChatID   string `json:"chat_id"`
}

type ModerationResult struct {
	IsSpam       bool    `json:"is_spam"`
	IsNSFW       bool    `json:"is_nsfw"`
	IsToxic      bool    `json:"is_toxic"`
	SpamScore    float64 `json:"spam_score"`
	NSFWScore    float64 `json:"nsfw_score"`
	ToxicityScore float64 `json:"toxicity_score"`
	Action       string  `json:"action"` // allow, flag, block
}

type TranslateRequest struct {
	Text       string `json:"text"`
	TargetLang string `json:"target_lang"`
	SourceLang string `json:"source_lang,omitempty"`
}

type TranslateResult struct {
	TranslatedText string `json:"translated_text"`
	SourceLang     string `json:"source_lang"`
	TargetLang     string `json:"target_lang"`
}

type SuggestReplyRequest struct {
	Messages []string `json:"messages"`
	Count    int      `json:"count,omitempty"`
}

type SuggestReplyResult struct {
	Replies []string `json:"replies"`
}

type SummarizeRequest struct {
	Text     string `json:"text"`
	MaxWords int    `json:"max_words,omitempty"`
}

type SummarizeResult struct {
	Summary string `json:"summary"`
}

type CompleteRequest struct {
	Prompt  string `json:"prompt"`
	MaxTokens int  `json:"max_tokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
}

type CompleteResult struct {
	Text string `json:"text"`
}

type OllamaRequest struct {
	Model    string `json:"model"`
	Prompt   string `json:"prompt"`
	Stream   bool   `json:"stream"`
	Options  map[string]interface{} `json:"options,omitempty"`
}

type OllamaResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
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

	svc := &AIService{
		cfg:   cfg,
		logger: logger,
		redis:  redisClient,
		http: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        50,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     60 * time.Second,
			},
		},
	}

	app := fiber.New(fiber.Config{
		AppName: "IraqSecureChat AI Service",
	})

	app.Post("/v1/ai/translate", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleTranslate)
	app.Post("/v1/ai/moderate", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleModerate)
	app.Post("/v1/ai/suggest-reply", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleSuggestReply)
	app.Post("/v1/ai/summarize", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleSummarize)
	app.Post("/v1/ai/complete", middleware.JWTAuth(cfg.JWT.AccessSecret), svc.HandleComplete)

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		logger.Info("shutting down ai service")
		app.ShutdownWithTimeout(cfg.Server.GracefulTimeout)
	}()

	logger.Info("ai service starting", zap.String("addr", cfg.Addr()))
	if err := app.Listen(cfg.Addr()); err != nil {
		logger.Fatal("server error", zap.Error(err))
	}
}

func (s *AIService) HandleTranslate(c *fiber.Ctx) error {
	var req TranslateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request body"},
		})
	}

	if req.Text == "" || req.TargetLang == "" {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "text and target_lang are required"},
		})
	}

	if !s.cfg.AI.TranslationEnabled {
		return c.Status(503).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 503, "message": "translation service disabled"},
		})
	}

	prompt := fmt.Sprintf("Translate the following text to %s. Return ONLY the translated text, no explanations.\n\nSource: %s", req.TargetLang, req.Text)

	result, err := s.callAI(c.Context(), prompt, 500, 0.3)
	if err != nil {
		s.logger.Error("translation failed", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "translation failed"},
		})
	}

	sourceLang := req.SourceLang
	if sourceLang == "" {
		sourceLang = detectLanguage(req.Text)
	}

	return c.JSON(types.NewAPIResponse(TranslateResult{
		TranslatedText: strings.TrimSpace(result),
		SourceLang:     sourceLang,
		TargetLang:     req.TargetLang,
	}))
}

func (s *AIService) HandleModerate(c *fiber.Ctx) error {
	var req ModerationRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request body"},
		})
	}

	if req.Text == "" {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "text is required"},
		})
	}

	if !s.cfg.AI.ModerationEnabled {
		return c.JSON(types.NewAPIResponse(ModerationResult{
			Action: "allow",
		}))
	}

	prompt := fmt.Sprintf(`Analyze this message for moderation. Return a JSON object with these fields:
- is_spam: bool
- is_nsfw: bool
- is_toxic: bool
- spam_score: float (0.0-1.0)
- nsfw_score: float (0.0-1.0)
- toxicity_score: float (0.0-1.0)

Message: "%s"`, req.Text)

	result, err := s.callAI(c.Context(), prompt, 500, 0.1)
	if err != nil {
		s.logger.Error("moderation failed", zap.Error(err))
		return c.JSON(types.NewAPIResponse(ModerationResult{Action: "allow"}))
	}

	var modResult ModerationResult
	if err := json.Unmarshal([]byte(result), &modResult); err != nil {
		modResult = ModerationResult{Action: "allow"}
	}

	// Determine action based on scores
	modResult.Action = "allow"
	if modResult.SpamScore > s.cfg.AI.SpamThreshold {
		modResult.Action = "block"
	} else if modResult.NSFWScore > s.cfg.AI.NSFWThreshold {
		modResult.Action = "block"
	} else if modResult.ToxicityScore > 0.8 {
		modResult.Action = "flag"
	}

	s.logger.Warn("moderation result",
		zap.String("text", req.Text),
		zap.Float64("spam_score", modResult.SpamScore),
		zap.Float64("nsfw_score", modResult.NSFWScore),
		zap.String("action", modResult.Action),
	)

	return c.JSON(types.NewAPIResponse(modResult))
}

func (s *AIService) HandleSuggestReply(c *fiber.Ctx) error {
	var req SuggestReplyRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request body"},
		})
	}

	if len(req.Messages) == 0 {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "messages array is required"},
		})
	}

	if !s.cfg.AI.SmartReplyEnabled {
		return c.JSON(types.NewAPIResponse(SuggestReplyResult{
			Replies: []string{},
		}))
	}

	count := req.Count
	if count < 1 || count > 5 {
		count = 3
	}

	conversation := strings.Join(req.Messages, "\n")
	prompt := fmt.Sprintf(`Based on this conversation, suggest %d short, contextually appropriate replies.
Return ONLY a JSON array of strings, no explanations.

Conversation:
%s

Replies:`, count, conversation)

	result, err := s.callAI(c.Context(), prompt, 300, 0.7)
	if err != nil {
		s.logger.Error("smart reply failed", zap.Error(err))
		return c.JSON(types.NewAPIResponse(SuggestReplyResult{
			Replies: []string{},
		}))
	}

	var replies []string
	if err := json.Unmarshal([]byte(result), &replies); err != nil {
		replies = extractLines(result)
		if len(replies) == 0 {
			replies = []string{}
		}
	}

	if len(replies) > count {
		replies = replies[:count]
	}

	return c.JSON(types.NewAPIResponse(SuggestReplyResult{
		Replies: replies,
	}))
}

func (s *AIService) HandleSummarize(c *fiber.Ctx) error {
	var req SummarizeRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request body"},
		})
	}

	if req.Text == "" {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "text is required"},
		})
	}

	maxWords := req.MaxWords
	if maxWords < 10 || maxWords > 500 {
		maxWords = 100
	}

	prompt := fmt.Sprintf(`Summarize the following text in at most %d words. Focus on key points.
Return ONLY the summary, no explanations.

Text: %s`, maxWords, req.Text)

	result, err := s.callAI(c.Context(), prompt, 1000, 0.3)
	if err != nil {
		s.logger.Error("summarization failed", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "summarization failed"},
		})
	}

	return c.JSON(types.NewAPIResponse(SummarizeResult{
		Summary: strings.TrimSpace(result),
	}))
}

func (s *AIService) HandleComplete(c *fiber.Ctx) error {
	var req CompleteRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "invalid request body"},
		})
	}

	if req.Prompt == "" {
		return c.Status(400).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 400, "message": "prompt is required"},
		})
	}

	maxTokens := req.MaxTokens
	if maxTokens < 10 || maxTokens > 4096 {
		maxTokens = s.cfg.AI.MaxTokens
	}

	temp := req.Temperature
	if temp <= 0 || temp > 2.0 {
		temp = s.cfg.AI.Temperature
	}

	result, err := s.callAI(c.Context(), req.Prompt, maxTokens, temp)
	if err != nil {
		s.logger.Error("completion failed", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{
			"ok": false, "error": fiber.Map{"code": 500, "message": "completion failed"},
		})
	}

	return c.JSON(types.NewAPIResponse(CompleteResult{
		Text: strings.TrimSpace(result),
	}))
}

func (s *AIService) callAI(ctx context.Context, prompt string, maxTokens int, temperature float64) (string, error) {
	switch s.cfg.AI.Provider {
	case "ollama", "local":
		return s.callOllama(ctx, prompt)
	case "openai":
		return s.callOpenAI(ctx, prompt, maxTokens, temperature)
	case "anthropic":
		return s.callAnthropic(ctx, prompt, maxTokens, temperature)
	default:
		return s.callOllama(ctx, prompt)
	}
}

func (s *AIService) callOllama(ctx context.Context, prompt string) (string, error) {
	reqBody := OllamaRequest{
		Model:  "llama3",
		Prompt: prompt,
		Stream: false,
		Options: map[string]interface{}{
			"temperature": 0.7,
		},
	}

	data, _ := json.Marshal(reqBody)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", s.cfg.AI.APIURL, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.http.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	var ollamaResp OllamaResponse
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	return ollamaResp.Response, nil
}

func (s *AIService) callOpenAI(ctx context.Context, prompt string, maxTokens int, temperature float64) (string, error) {
	reqBody := map[string]interface{}{
		"model":       "gpt-4",
		"messages":    []map[string]string{{"role": "user", "content": prompt}},
		"max_tokens":  maxTokens,
		"temperature": temperature,
	}

	data, _ := json.Marshal(reqBody)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", s.cfg.AI.APIURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+s.cfg.AI.APIKey)

	resp, err := s.http.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("openai request: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return "", fmt.Errorf("no response from openai")
	}

	choice := choices[0].(map[string]interface{})
	message := choice["message"].(map[string]interface{})
	content, _ := message["content"].(string)

	return content, nil
}

func (s *AIService) callAnthropic(ctx context.Context, prompt string, maxTokens int, temperature float64) (string, error) {
	reqBody := map[string]interface{}{
		"model":             "claude-3-opus-20240229",
		"max_tokens":        maxTokens,
		"temperature":       temperature,
		"messages":          []map[string]string{{"role": "user", "content": prompt}},
	}

	data, _ := json.Marshal(reqBody)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", s.cfg.AI.APIURL+"/v1/messages", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", s.cfg.AI.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := s.http.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("anthropic request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(body, &result)

	content, ok := result["content"].([]interface{})
	if !ok || len(content) == 0 {
		return "", fmt.Errorf("no response from anthropic")
	}

	block := content[0].(map[string]interface{})
	text, _ := block["text"].(string)

	return text, nil
}

func detectLanguage(text string) string {
	arabicChars := 0
	totalChars := 0
	for _, r := range text {
		if r >= 0x0600 && r <= 0x06FF {
			arabicChars++
		}
		totalChars++
	}
	if totalChars > 0 && float64(arabicChars)/float64(totalChars) > 0.3 {
		return "ar"
	}
	return "en"
}

func extractLines(result string) []string {
	var lines []string
	for _, line := range strings.Split(result, "\n") {
		line = strings.TrimSpace(line)
		line = strings.Trim(line, "\"- ")
		if line != "" && !strings.HasPrefix(line, "{") && !strings.HasPrefix(line, "[") {
			lines = append(lines, line)
		}
	}
	return lines
}

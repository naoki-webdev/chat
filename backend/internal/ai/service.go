package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Message is the small conversation shape Orbit AI needs. Keeping this
// separate from the HTTP/domain packages makes the provider replaceable.
type Message struct {
	Author string
	Body   string
}

type Service interface {
	Stream(context.Context, []Message, string, func(string) error) (string, error)
}

type providerConfig struct {
	provider string
	apiKey   string
	model    string
	baseURL  string
}

func NewFromEnv() Service {
	config := providerConfigFromEnv()
	if config.provider == "mock" || (config.provider != "ollama" && config.apiKey == "") {
		return NewMock()
	}
	if config.baseURL == "" || config.model == "" {
		return NewMock()
	}
	return &openAIService{
		apiKey:  config.apiKey,
		model:   config.model,
		baseURL: config.baseURL,
		client:  &http.Client{Timeout: 90 * time.Second},
	}
}

// ValidateProductionConfig rejects the silent mock fallback when the server
// is running outside an explicitly local/test environment. Mock is valid only
// when the operator opts into it with AI_PROVIDER=mock.
func ValidateProductionConfig() error {
	explicitProvider := strings.ToLower(strings.TrimSpace(os.Getenv("AI_PROVIDER")))
	if explicitProvider == "mock" {
		return nil
	}
	switch explicitProvider {
	case "", "openai", "gemini", "ollama", "openrouter", "bedrock":
	default:
		return fmt.Errorf("unsupported AI_PROVIDER %q", explicitProvider)
	}
	config := providerConfigFromEnv()
	if config.provider == "mock" {
		return errors.New("AI provider configuration is incomplete; set an API key or explicitly use AI_PROVIDER=mock")
	}
	if config.provider != "ollama" && config.apiKey == "" {
		return fmt.Errorf("API key is required for AI_PROVIDER=%s", config.provider)
	}
	if config.baseURL == "" || config.model == "" {
		return fmt.Errorf("AI provider %s requires both model and base URL", config.provider)
	}
	return nil
}

func providerConfigFromEnv() providerConfig {
	explicitProvider := strings.ToLower(strings.TrimSpace(os.Getenv("AI_PROVIDER")))
	provider := explicitProvider
	apiKey := firstNonEmptyEnv("AI_API_KEY", "OPENAI_API_KEY")
	model := firstNonEmptyEnv("AI_MODEL")
	baseURL := firstNonEmptyEnv("AI_BASE_URL")

	if provider == "" {
		model = firstNonEmptyEnv("AI_MODEL", "OPENAI_MODEL")
		baseURL = firstNonEmptyEnv("AI_BASE_URL", "OPENAI_BASE_URL")
		provider = inferProvider(baseURL)
	}
	if provider == "" {
		provider = "openai"
	}

	switch provider {
	case "openai":
		if model == "" {
			model = firstNonEmptyEnv("OPENAI_MODEL")
		}
		if model == "" {
			model = "gpt-4o-mini"
		}
		if baseURL == "" {
			baseURL = firstNonEmptyEnv("OPENAI_BASE_URL")
		}
		if baseURL == "" {
			baseURL = "https://api.openai.com/v1/chat/completions"
		}
	case "gemini":
		apiKey = firstNonEmptyEnv("AI_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY", "OPENAI_API_KEY")
		model = firstNonEmptyEnv("AI_MODEL", "GEMINI_MODEL")
		if model == "" {
			model = "gemini-3.7-flash"
		}
		if baseURL == "" {
			baseURL = "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions"
		}
	case "ollama":
		apiKey = firstNonEmptyEnv("AI_API_KEY")
		model = firstNonEmptyEnv("AI_MODEL", "OLLAMA_MODEL")
		if model == "" {
			model = "qwen3:8b"
		}
		if baseURL == "" {
			baseURL = "http://127.0.0.1:11434/v1/chat/completions"
		}
		if apiKey == "" {
			apiKey = "ollama"
		}
	case "openrouter":
		apiKey = firstNonEmptyEnv("AI_API_KEY", "OPENROUTER_API_KEY", "OPENAI_API_KEY")
		model = firstNonEmptyEnv("AI_MODEL", "OPENROUTER_MODEL")
		if model == "" {
			model = "openai/gpt-4o-mini"
		}
		if baseURL == "" {
			baseURL = "https://openrouter.ai/api/v1/chat/completions"
		}
	case "bedrock":
		apiKey = firstNonEmptyEnv("AI_API_KEY", "BEDROCK_API_KEY", "OPENAI_API_KEY")
		model = firstNonEmptyEnv("AI_MODEL", "BEDROCK_MODEL")
		if model == "" {
			model = "openai.gpt-oss-120b"
		}
		if baseURL == "" {
			region := firstNonEmptyEnv("AI_REGION", "AWS_REGION", "AWS_DEFAULT_REGION")
			if region == "" {
				region = "ap-northeast-1"
			}
			baseURL = "https://bedrock-mantle." + region + ".api.aws/v1/chat/completions"
		}
	case "mock":
		return providerConfig{provider: "mock"}
	default:
		return providerConfig{provider: "mock"}
	}

	return providerConfig{provider: provider, apiKey: strings.TrimSpace(apiKey), model: strings.TrimSpace(model), baseURL: normalizeChatCompletionsURL(baseURL)}
}

func normalizeChatCompletionsURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" || strings.HasSuffix(baseURL, "/chat/completions") {
		return baseURL
	}
	return baseURL + "/chat/completions"
}

func inferProvider(baseURL string) string {
	lowerURL := strings.ToLower(baseURL)
	switch {
	case strings.Contains(lowerURL, "generativelanguage.googleapis.com"):
		return "gemini"
	case strings.Contains(lowerURL, "127.0.0.1:11434"), strings.Contains(lowerURL, "localhost:11434"):
		return "ollama"
	case strings.Contains(lowerURL, "openrouter.ai"):
		return "openrouter"
	case strings.Contains(lowerURL, "bedrock-mantle."):
		return "bedrock"
	default:
		return ""
	}
}

func firstNonEmptyEnv(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

type mockService struct{}

func NewMock() Service { return mockService{} }

func (mockService) Stream(ctx context.Context, history []Message, prompt string, onDelta func(string) error) (string, error) {
	_ = history
	response := fmt.Sprintf("Orbit AI（デモ）: 「%s」へのデモ回答です。AIプロバイダーが未設定のため、会話の内容に基づく回答はまだ行っていません。", strings.TrimSpace(prompt))
	runes := []rune(response)
	var builder strings.Builder
	for index := 0; index < len(runes); index += 4 {
		end := index + 4
		if end > len(runes) {
			end = len(runes)
		}
		select {
		case <-ctx.Done():
			return builder.String(), ctx.Err()
		case <-time.After(12 * time.Millisecond):
		}
		chunk := string(runes[index:end])
		if err := onDelta(chunk); err != nil {
			return builder.String(), err
		}
		builder.WriteString(chunk)
	}
	return builder.String(), nil
}

type openAIService struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

func (s *openAIService) Stream(ctx context.Context, history []Message, prompt string, onDelta func(string) error) (string, error) {
	conversation := []chatMessage{{
		Role:    "system",
		Content: "あなたはOrbit AIです。チームチャットに参加する短く実用的なアシスタントとして、日本語で回答してください。会話にない事実は断定しないでください。",
	}}
	for _, message := range history {
		body := strings.TrimSpace(message.Body)
		if body == "" {
			continue
		}
		role := "user"
		if message.Author == "Orbit AI" {
			role = "assistant"
		}
		conversation = append(conversation, chatMessage{Role: role, Content: message.Author + ": " + body})
	}
	conversation = append(conversation, chatMessage{Role: "user", Content: strings.TrimSpace(prompt)})

	body, err := json.Marshal(chatRequest{Model: s.model, Messages: conversation, Stream: true})
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+s.apiKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		errorBody, _ := io.ReadAll(io.LimitReader(response.Body, 4<<10))
		return "", fmt.Errorf("AI provider returned %s: %s", response.Status, strings.TrimSpace(string(errorBody)))
	}

	var builder strings.Builder
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 1024), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return builder.String(), err
		}
		if len(chunk.Choices) == 0 || chunk.Choices[0].Delta.Content == "" {
			continue
		}
		delta := chunk.Choices[0].Delta.Content
		if err := onDelta(delta); err != nil {
			return builder.String(), err
		}
		builder.WriteString(delta)
	}
	if err := scanner.Err(); err != nil {
		return builder.String(), err
	}
	if builder.Len() == 0 {
		return "", errors.New("AI provider returned an empty response")
	}
	return builder.String(), nil
}

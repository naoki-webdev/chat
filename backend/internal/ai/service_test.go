package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMockStreamEmitsCompleteResponse(t *testing.T) {
	var chunks []string
	response, err := NewMock().Stream(context.Background(), nil, "今日の会話をまとめて", func(delta string) error {
		chunks = append(chunks, delta)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected streaming chunks, got %d", len(chunks))
	}
	if strings.Join(chunks, "") != response {
		t.Fatalf("chunks do not reconstruct response: %q != %q", strings.Join(chunks, ""), response)
	}
	if !strings.Contains(response, "Orbit AI（デモ）") {
		t.Fatalf("unexpected mock response: %q", response)
	}
}

func TestMockStreamHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewMock().Stream(ctx, nil, "hello", func(string) error { return nil })
	if err == nil {
		t.Fatal("expected cancellation error")
	}
}

func TestProviderConfigSupportsCloudAndLocalBackends(t *testing.T) {
	t.Setenv("AI_PROVIDER", "gemini")
	t.Setenv("AI_API_KEY", "gemini-key")
	t.Setenv("AI_MODEL", "gemini-test")
	t.Setenv("AI_BASE_URL", "")
	gemini := providerConfigFromEnv()
	if gemini.provider != "gemini" || gemini.apiKey != "gemini-key" || gemini.model != "gemini-test" || !strings.Contains(gemini.baseURL, "generativelanguage.googleapis.com") {
		t.Fatalf("unexpected Gemini config: %+v", gemini)
	}

	t.Setenv("AI_PROVIDER", "ollama")
	t.Setenv("AI_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "legacy-key")
	t.Setenv("AI_MODEL", "llama-test")
	t.Setenv("AI_BASE_URL", "http://127.0.0.1:11434/v1")
	ollama := providerConfigFromEnv()
	if ollama.provider != "ollama" || ollama.apiKey != "ollama" || ollama.model != "llama-test" || ollama.baseURL != "http://127.0.0.1:11434/v1/chat/completions" {
		t.Fatalf("unexpected Ollama config: %+v", ollama)
	}

	t.Setenv("AI_PROVIDER", "bedrock")
	t.Setenv("AI_API_KEY", "bedrock-key")
	t.Setenv("AI_MODEL", "bedrock-test")
	t.Setenv("AI_BASE_URL", "")
	t.Setenv("AI_REGION", "ap-northeast-1")
	bedrock := providerConfigFromEnv()
	if bedrock.provider != "bedrock" || bedrock.apiKey != "bedrock-key" || bedrock.model != "bedrock-test" || !strings.Contains(bedrock.baseURL, "bedrock-mantle.ap-northeast-1.api.aws") {
		t.Fatalf("unexpected Bedrock config: %+v", bedrock)
	}
}

func TestValidateProductionConfigRequiresExplicitMockOrProviderCredentials(t *testing.T) {
	t.Setenv("AI_PROVIDER", "")
	t.Setenv("AI_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	if err := ValidateProductionConfig(); err == nil {
		t.Fatal("missing production AI credentials should fail validation")
	}

	t.Setenv("AI_PROVIDER", "mock")
	if err := ValidateProductionConfig(); err != nil {
		t.Fatalf("explicit mock provider should be allowed: %v", err)
	}

	t.Setenv("AI_PROVIDER", "gemini")
	t.Setenv("AI_API_KEY", "gemini-key")
	if err := ValidateProductionConfig(); err != nil {
		t.Fatalf("configured Gemini provider should be valid: %v", err)
	}
}

func TestOpenAIServiceParsesSSEStream(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("authorization header = %q", request.Header.Get("Authorization"))
		}
		var payload struct {
			Model  string `json:"model"`
			Stream bool   `json:"stream"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Model != "test-model" || !payload.Stream {
			t.Errorf("unexpected request payload: %+v", payload)
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = writer.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n"))
		_, _ = writer.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\" Orbit\"}}]}\n\n"))
		_, _ = writer.Write([]byte("data: [DONE]\n\n"))
	}))
	defer provider.Close()

	service := &openAIService{apiKey: "test-key", model: "test-model", baseURL: provider.URL, client: provider.Client()}
	var chunks []string
	response, err := service.Stream(context.Background(), []Message{{Author: "Taro", Body: "Hi"}}, "続けて", func(delta string) error {
		chunks = append(chunks, delta)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if response != "Hello Orbit" || strings.Join(chunks, "") != response {
		t.Fatalf("unexpected streamed response: %q / %#v", response, chunks)
	}
}

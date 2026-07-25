package test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/redisqueue"
	runtimeexecutor "github.com/router-for-me/CLIProxyAPI/v7/internal/runtime/executor"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
)

func TestCodexExecutorRecordsUsageWithoutDoubleCountingInQueue(t *testing.T) {
	model := fmt.Sprintf("gpt-5.4-codex-usage-%d", time.Now().UnixNano())
	source := fmt.Sprintf("codex-usage-%d@example.com", time.Now().UnixNano())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("path = %q, want /responses", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"type":"response.completed","response":{"id":"resp_usage","model":"` + model + `","usage":{"input_tokens":100,"output_tokens":20,"input_tokens_details":{"cached_tokens":30},"output_tokens_details":{"reasoning_tokens":9}},"output":[]}}` + "\n\n"))
	}))
	defer server.Close()

	executor := runtimeexecutor.NewCodexExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{
		Provider: "codex",
		Attributes: map[string]string{
			"api_key":  "test-upstream-key",
			"base_url": server.URL,
		},
		Metadata: map[string]any{
			"email": source,
		},
	}

	configureUsageQueueForTest(t)

	_, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   model,
		Payload: []byte(`{"model":"` + model + `","input":"hi"}`),
	}, cliproxyexecutor.Options{
		SourceFormat:    sdktranslator.FormatOpenAIResponse,
		OriginalRequest: []byte(`{"model":"` + model + `","input":"hi"}`),
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	got := waitForQueuedUsagePayload(t, "codex", model)
	if got.Failed {
		t.Fatalf("payload failed = true, want false")
	}
	if got.Tokens.TotalTokens != 120 {
		t.Fatalf("payload total tokens = %d, want 120", got.Tokens.TotalTokens)
	}
}

func TestCodexExecutorRecordsSuccessfulZeroUsageInQueue(t *testing.T) {
	model := fmt.Sprintf("gpt-5.4-codex-zero-usage-%d", time.Now().UnixNano())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"type":"response.completed","response":{"id":"resp_zero","model":"` + model + `","output":[]}}` + "\n\n"))
	}))
	defer server.Close()

	executor := runtimeexecutor.NewCodexExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Provider: "codex", Attributes: map[string]string{
		"api_key":  "test-upstream-key",
		"base_url": server.URL,
	}}

	configureUsageQueueForTest(t)

	_, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   model,
		Payload: []byte(`{"model":"` + model + `","input":"hi"}`),
	}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FormatOpenAIResponse})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	got := waitForQueuedUsagePayload(t, "codex", model)
	if got.Failed || got.Tokens.TotalTokens != 0 {
		t.Fatalf("queued payload = %+v, want successful zero usage", got)
	}
}

func configureUsageQueueForTest(t *testing.T) {
	t.Helper()
	prevQueueEnabled := redisqueue.Enabled()
	prevUsageEnabled := redisqueue.UsageStatisticsEnabled()
	redisqueue.SetEnabled(false)
	redisqueue.SetEnabled(true)
	redisqueue.SetUsageStatisticsEnabled(true)
	t.Cleanup(func() {
		redisqueue.SetEnabled(false)
		redisqueue.SetEnabled(prevQueueEnabled)
		redisqueue.SetUsageStatisticsEnabled(prevUsageEnabled)
	})
}

func TestGeminiExecutorRecordsSuccessfulZeroUsageInQueue(t *testing.T) {
	model := fmt.Sprintf("gemini-2.5-flash-zero-usage-%d", time.Now().UnixNano())
	source := fmt.Sprintf("zero-usage-%d@example.com", time.Now().UnixNano())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantPath := "/v1beta/models/" + model + ":generateContent"
		if r.URL.Path != wantPath {
			t.Fatalf("path = %q, want %q", r.URL.Path, wantPath)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":0,"candidatesTokenCount":0,"totalTokenCount":0}}`))
	}))
	defer server.Close()

	executor := runtimeexecutor.NewGeminiExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{
		Provider: "gemini",
		Attributes: map[string]string{
			"api_key":  "test-upstream-key",
			"base_url": server.URL,
		},
		Metadata: map[string]any{
			"email": source,
		},
	}

	prevQueueEnabled := redisqueue.Enabled()
	prevUsageEnabled := redisqueue.UsageStatisticsEnabled()
	redisqueue.SetEnabled(false)
	redisqueue.SetEnabled(true)
	redisqueue.SetUsageStatisticsEnabled(true)
	t.Cleanup(func() {
		redisqueue.SetEnabled(false)
		redisqueue.SetEnabled(prevQueueEnabled)
		redisqueue.SetUsageStatisticsEnabled(prevUsageEnabled)
	})

	_, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   model,
		Payload: []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`),
	}, cliproxyexecutor.Options{
		SourceFormat:    sdktranslator.FormatGemini,
		OriginalRequest: []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`),
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	waitForQueuedUsageModelTotalTokens(t, "gemini", model, 0)
}

func waitForQueuedUsageModelTotalTokens(t *testing.T, wantProvider, wantModel string, wantTokens int64) {
	t.Helper()
	got := waitForQueuedUsagePayload(t, wantProvider, wantModel)
	if got.Failed {
		t.Fatalf("payload failed = true, want false")
	}
	if got.Tokens.TotalTokens != wantTokens {
		t.Fatalf("payload total tokens = %d, want %d", got.Tokens.TotalTokens, wantTokens)
	}
}

func waitForQueuedUsagePayload(t *testing.T, wantProvider, wantModel string) queuedUsagePayload {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		items := redisqueue.PopOldest(10)
		for _, item := range items {
			got, ok := parseQueuedUsagePayload(t, item)
			if !ok {
				continue
			}
			if got.Provider != wantProvider || got.Model != wantModel {
				continue
			}
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for queued usage payload for provider=%q model=%q", wantProvider, wantModel)
	return queuedUsagePayload{}
}

type queuedUsagePayload struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Failed   bool   `json:"failed"`
	Tokens   struct {
		TotalTokens int64 `json:"total_tokens"`
	} `json:"tokens"`
}

func parseQueuedUsagePayload(t *testing.T, payload []byte) (queuedUsagePayload, bool) {
	t.Helper()

	var parsed queuedUsagePayload
	if len(payload) == 0 {
		return parsed, false
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return parsed, false
	}
	if parsed.Provider == "" || parsed.Model == "" {
		return parsed, false
	}
	return parsed, true
}

package executor

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/translator"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/usage"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
)

func TestCodexExecutorExecuteCompletionWithoutUsageRecordsZeroSuccess(t *testing.T) {
	server := newCodexUsageTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeCodexSSE(t, w, `{"type":"response.completed","response":{"id":"resp_zero","model":"gpt-5.4","output":[]}}`)
	})
	defer server.Close()

	model := uniqueCodexUsageModel("nonstream-zero")
	records := captureCodexExecutorUsage()
	executor := NewCodexExecutor(&config.Config{})
	_, err := executor.Execute(context.Background(), codexUsageTestAuth(server.URL, model), codexUsageTestRequest(model), codexUsageTestOptions(false))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	record := waitForCodexExecutorUsage(t, records, model)
	assertCodexUsageRecord(t, record, false, 0, 0, 0, 0)
	assertNoDuplicateCodexExecutorUsage(t, records, model)
}

func TestCodexExecutorExecuteStreamCompletionWithoutUsageRecordsZeroSuccess(t *testing.T) {
	server := newCodexUsageTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeCodexSSE(t, w, `{"type":"response.completed","response":{"id":"resp_zero_stream","model":"gpt-5.4","output":[]}}`)
	})
	defer server.Close()

	model := uniqueCodexUsageModel("stream-zero")
	records := captureCodexExecutorUsage()
	executor := NewCodexExecutor(&config.Config{})
	result, err := executor.ExecuteStream(context.Background(), codexUsageTestAuth(server.URL, model), codexUsageTestRequest(model), codexUsageTestOptions(true))
	if err != nil {
		t.Fatalf("ExecuteStream() error = %v", err)
	}
	for chunk := range result.Chunks {
		if chunk.Err != nil {
			t.Fatalf("stream chunk error = %v", chunk.Err)
		}
	}

	record := waitForCodexExecutorUsage(t, records, model)
	assertCodexUsageRecord(t, record, false, 0, 0, 0, 0)
	assertNoDuplicateCodexExecutorUsage(t, records, model)
}

func TestCodexExecutorExecuteStreamIncompleteRecordsTerminalUsage(t *testing.T) {
	server := newCodexUsageTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeCodexSSE(t, w, `{"type":"response.incomplete","response":{"id":"resp_incomplete","model":"gpt-5.4","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"usage":{"input_tokens":8,"output_tokens":2,"total_tokens":10},"output":[]}}`)
	})
	defer server.Close()

	model := uniqueCodexUsageModel("stream-incomplete")
	records := captureCodexExecutorUsage()
	executor := NewCodexExecutor(&config.Config{})
	result, err := executor.ExecuteStream(context.Background(), codexUsageTestAuth(server.URL, model), codexUsageTestRequest(model), codexUsageTestOptions(true))
	if err != nil {
		t.Fatalf("ExecuteStream() error = %v", err)
	}
	for chunk := range result.Chunks {
		if chunk.Err != nil {
			t.Fatalf("stream chunk error = %v", chunk.Err)
		}
	}

	record := waitForCodexExecutorUsage(t, records, model)
	assertCodexUsageRecord(t, record, false, 8, 2, 0, 10)
	assertNoDuplicateCodexExecutorUsage(t, records, model)
}

func TestCodexExecutorExecuteStreamCompletionWithoutTotalUsesInputPlusOutput(t *testing.T) {
	server := newCodexUsageTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeCodexSSE(t, w, `{"type":"response.completed","response":{"id":"resp_usage","model":"gpt-5.4","usage":{"input_tokens":100,"output_tokens":20,"input_tokens_details":{"cached_tokens":30},"output_tokens_details":{"reasoning_tokens":9}},"output":[]}}`)
	})
	defer server.Close()

	model := uniqueCodexUsageModel("stream-no-total")
	records := captureCodexExecutorUsage()
	executor := NewCodexExecutor(&config.Config{})
	result, err := executor.ExecuteStream(context.Background(), codexUsageTestAuth(server.URL, model), codexUsageTestRequest(model), codexUsageTestOptions(true))
	if err != nil {
		t.Fatalf("ExecuteStream() error = %v", err)
	}
	for chunk := range result.Chunks {
		if chunk.Err != nil {
			t.Fatalf("stream chunk error = %v", chunk.Err)
		}
	}

	record := waitForCodexExecutorUsage(t, records, model)
	assertCodexUsageRecord(t, record, false, 100, 20, 9, 120)
	if record.Detail.CacheReadTokens != 30 || record.Detail.CachedTokens != 30 {
		t.Fatalf("cache usage = %+v, want cached/read 30", record.Detail)
	}
	assertNoDuplicateCodexExecutorUsage(t, records, model)
}

func TestCodexExecutorExecuteStreamEOFBeforeCompletionRecordsFailureAndReturns408(t *testing.T) {
	server := newCodexUsageTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeCodexSSE(t, w, `{"type":"response.created","response":{"id":"resp_eof","model":"gpt-5.4"}}`)
	})
	defer server.Close()

	model := uniqueCodexUsageModel("stream-eof")
	records := captureCodexExecutorUsage()
	executor := NewCodexExecutor(&config.Config{})
	result, err := executor.ExecuteStream(context.Background(), codexUsageTestAuth(server.URL, model), codexUsageTestRequest(model), codexUsageTestOptions(true))
	if err != nil {
		t.Fatalf("ExecuteStream() error = %v", err)
	}
	var streamErr error
	for chunk := range result.Chunks {
		if chunk.Err != nil {
			streamErr = chunk.Err
		}
	}
	if streamErr == nil {
		t.Fatal("missing stream error")
	}
	if got := statusCodeFromTestError(t, streamErr); got != http.StatusRequestTimeout {
		t.Fatalf("stream error status = %d, want %d", got, http.StatusRequestTimeout)
	}

	record := waitForCodexExecutorUsage(t, records, model)
	assertCodexUsageRecord(t, record, true, 0, 0, 0, 0)
	assertNoDuplicateCodexExecutorUsage(t, records, model)
}

func TestCodexExecutorExecuteStreamCancellationBeforeCompletionRecordsFailure(t *testing.T) {
	requestSeen := make(chan struct{})
	server := newCodexUsageTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeCodexSSE(t, w, `{"type":"response.created","response":{"id":"resp_cancel","model":"gpt-5.4"}}`)
		close(requestSeen)
		<-r.Context().Done()
	})
	defer server.Close()

	model := uniqueCodexUsageModel("stream-cancel")
	records := captureCodexExecutorUsage()
	ctx, cancel := context.WithCancel(context.Background())
	executor := NewCodexExecutor(&config.Config{})
	result, err := executor.ExecuteStream(ctx, codexUsageTestAuth(server.URL, model), codexUsageTestRequest(model), codexUsageTestOptions(true))
	if err != nil {
		t.Fatalf("ExecuteStream() error = %v", err)
	}
	select {
	case <-requestSeen:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for upstream stream")
	}
	cancel()
	for range result.Chunks {
	}

	record := waitForCodexExecutorUsage(t, records, model)
	assertCodexUsageRecord(t, record, true, 0, 0, 0, 0)
	if !strings.Contains(record.Fail.Body, "context canceled") {
		t.Fatalf("failure body = %q, want context canceled", record.Fail.Body)
	}
	assertNoDuplicateCodexExecutorUsage(t, records, model)
}

func TestCodexExecutorExecuteStreamCompletionThenCancellationStaysSuccessful(t *testing.T) {
	server := newCodexUsageTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeCodexSSE(t, w, `{"type":"response.completed","response":{"id":"resp_cancel_after","model":"gpt-5.4","usage":{"input_tokens":8,"output_tokens":2},"output":[]}}`)
	})
	defer server.Close()

	model := uniqueCodexUsageModel("stream-complete-cancel")
	records := captureCodexExecutorUsage()
	ctx, cancel := context.WithCancel(context.Background())
	executor := NewCodexExecutor(&config.Config{})
	result, err := executor.ExecuteStream(ctx, codexUsageTestAuth(server.URL, model), codexUsageTestRequest(model), codexUsageTestOptions(true))
	if err != nil {
		t.Fatalf("ExecuteStream() error = %v", err)
	}
	record := waitForCodexExecutorUsage(t, records, model)
	cancel()
	for range result.Chunks {
	}

	assertCodexUsageRecord(t, record, false, 8, 2, 0, 10)
	assertNoDuplicateCodexExecutorUsage(t, records, model)
}

func TestCodexExecutorExecuteStreamFailurePreservesObservedUsage(t *testing.T) {
	server := newCodexUsageTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeCodexSSE(t, w, `{"type":"response.in_progress","response":{"usage":{"input_tokens":12,"output_tokens":3,"output_tokens_details":{"reasoning_tokens":2}}}}`)
		writeCodexSSE(t, w, `{"type":"error","error":{"type":"usage_limit_reached","message":"usage limit reached","resets_in_seconds":60}}`)
	})
	defer server.Close()

	model := uniqueCodexUsageModel("stream-partial")
	records := captureCodexExecutorUsage()
	executor := NewCodexExecutor(&config.Config{})
	result, err := executor.ExecuteStream(context.Background(), codexUsageTestAuth(server.URL, model), codexUsageTestRequest(model), codexUsageTestOptions(true))
	if err != nil {
		t.Fatalf("ExecuteStream() error = %v", err)
	}
	for range result.Chunks {
	}

	record := waitForCodexExecutorUsage(t, records, model)
	assertCodexUsageRecord(t, record, true, 12, 3, 2, 15)
	assertNoDuplicateCodexExecutorUsage(t, records, model)
}

func newCodexUsageTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		handler(w, r)
	}))
}

func writeCodexSSE(t *testing.T, w http.ResponseWriter, payload string) {
	t.Helper()
	if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
		t.Fatalf("write SSE: %v", err)
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func uniqueCodexUsageModel(prefix string) string {
	return fmt.Sprintf("gpt-5.4-%s-%d", prefix, time.Now().UnixNano())
}

func codexUsageTestAuth(baseURL, model string) *cliproxyauth.Auth {
	return &cliproxyauth.Auth{
		ID:       model + "-auth",
		Provider: "codex",
		Attributes: map[string]string{
			"base_url": baseURL,
			"api_key":  "test-upstream-key",
		},
	}
}

func codexUsageTestRequest(model string) cliproxyexecutor.Request {
	return cliproxyexecutor.Request{
		Model:   model,
		Payload: []byte(`{"model":"` + model + `","input":"hello"}`),
	}
}

func codexUsageTestOptions(stream bool) cliproxyexecutor.Options {
	return cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FormatOpenAIResponse,
		Stream:       stream,
	}
}

type codexUsageCapturePlugin struct {
	records chan usage.Record
}

func (p *codexUsageCapturePlugin) HandleUsage(_ context.Context, record usage.Record) {
	if p == nil {
		return
	}
	select {
	case p.records <- record:
	default:
	}
}

func captureCodexExecutorUsage() <-chan usage.Record {
	records := make(chan usage.Record, 128)
	usage.RegisterNamedPlugin("codex-usage-accounting-capture", &codexUsageCapturePlugin{records: records})
	return records
}

func waitForCodexExecutorUsage(t *testing.T, records <-chan usage.Record, model string) usage.Record {
	t.Helper()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case record := <-records:
			if record.Provider == "codex" && record.Model == model {
				return record
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for Codex usage record model=%q", model)
		}
	}
}

func assertNoDuplicateCodexExecutorUsage(t *testing.T, records <-chan usage.Record, model string) {
	t.Helper()
	timer := time.NewTimer(150 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case record := <-records:
			if record.Provider == "codex" && record.Model == model {
				t.Fatalf("received duplicate Codex usage record: %+v", record)
			}
		case <-timer.C:
			return
		}
	}
}

func assertCodexUsageRecord(t *testing.T, record usage.Record, failed bool, input, output, reasoning, total int64) {
	t.Helper()
	if record.Failed != failed {
		t.Fatalf("record failed = %v, want %v; record=%+v", record.Failed, failed, record)
	}
	if record.Detail.InputTokens != input || record.Detail.OutputTokens != output || record.Detail.ReasoningTokens != reasoning || record.Detail.TotalTokens != total {
		t.Fatalf("record detail = %+v, want input=%d output=%d reasoning=%d total=%d", record.Detail, input, output, reasoning, total)
	}
}

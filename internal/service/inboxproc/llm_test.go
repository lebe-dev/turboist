package inboxproc

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const testAPIKey = "sk-secret-do-not-leak"

type capturedCall struct {
	path  string
	auth  string
	title string
	body  map[string]any
}

func newTestClient(t *testing.T, handler func(w http.ResponseWriter, r *http.Request, body map[string]any)) (*OpenAIClient, *[]capturedCall) {
	t.Helper()
	calls := &[]capturedCall{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		*calls = append(*calls, capturedCall{path: r.URL.Path, auth: r.Header.Get("Authorization"), title: r.Header.Get("X-Title"), body: body})
		handler(w, r, body)
	}))
	t.Cleanup(srv.Close)
	return NewOpenAIClient(OpenAIClientConfig{
		APIURL:  srv.URL + "/api/v1",
		APIKey:  testAPIKey,
		Model:   "test/model",
		Referer: "https://todo.example.com",
		Timeout: 2 * time.Second,
	}), calls
}

func writeCompletion(w http.ResponseWriter, content string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"model": "test/model-2026",
		"choices": []any{
			map[string]any{"message": map[string]any{"role": "assistant", "content": content}},
		},
		"usage": map[string]any{"prompt_tokens": 120, "completion_tokens": 30},
	})
}

func TestOpenAIClient_Success(t *testing.T) {
	client, calls := newTestClient(t, func(w http.ResponseWriter, _ *http.Request, _ map[string]any) {
		writeCompletion(w, `{"action":"keep"}`)
	})
	got, err := client.Classify(context.Background(), "system text", "user text")
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if got.Content != `{"action":"keep"}` || got.Model != "test/model-2026" || got.PromptTokens != 120 || got.CompletionTokens != 30 {
		t.Errorf("completion: got %+v", got)
	}
	if len(*calls) != 1 {
		t.Fatalf("calls: got %d, want 1", len(*calls))
	}
	c := (*calls)[0]
	if c.path != "/api/v1/chat/completions" {
		t.Errorf("path: got %q", c.path)
	}
	if c.auth != "Bearer "+testAPIKey || c.title != "Turboist" {
		t.Errorf("headers: auth=%q title=%q", c.auth, c.title)
	}
	if c.body["model"] != "test/model" || c.body["temperature"] != float64(0) {
		t.Errorf("body: got %v", c.body)
	}
	if rf, ok := c.body["response_format"].(map[string]any); !ok || rf["type"] != "json_object" {
		t.Errorf("response_format: got %v", c.body["response_format"])
	}
	msgs, _ := c.body["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("messages: got %v", c.body["messages"])
	}
	if m := msgs[0].(map[string]any); m["role"] != "system" || m["content"] != "system text" {
		t.Errorf("system message: got %v", m)
	}
	if m := msgs[1].(map[string]any); m["role"] != "user" || m["content"] != "user text" {
		t.Errorf("user message: got %v", m)
	}
}

func TestOpenAIClient_RateLimited(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request, _ map[string]any) {
		http.Error(w, `{"error":{"message":"slow down"}}`, http.StatusTooManyRequests)
	})
	_, err := client.Classify(context.Background(), "s", "u")
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("got %v, want ErrRateLimited", err)
	}
	assertNoKey(t, err)
}

func TestOpenAIClient_ServerError(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request, _ map[string]any) {
		http.Error(w, "upstream "+testAPIKey, http.StatusBadGateway)
	})
	_, err := client.Classify(context.Background(), "s", "u")
	if !errors.Is(err, ErrProvider) {
		t.Fatalf("got %v, want ErrProvider", err)
	}
	assertNoKey(t, err)
}

func TestOpenAIClient_Unauthorized(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request, _ map[string]any) {
		http.Error(w, `{"error":{"message":"invalid key"}}`, http.StatusUnauthorized)
	})
	_, err := client.Classify(context.Background(), "s", "u")
	if !errors.Is(err, ErrProvider) {
		t.Fatalf("got %v, want ErrProvider for a rejected key", err)
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should carry the status: %v", err)
	}
	assertNoKey(t, err)
}

func TestOpenAIClient_ConnectionDropped(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request, _ map[string]any) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("hijack unsupported")
		}
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	})
	_, err := client.Classify(context.Background(), "s", "u")
	if !errors.Is(err, ErrProvider) {
		t.Fatalf("got %v, want ErrProvider", err)
	}
	assertNoKey(t, err)
}

func TestOpenAIClient_Timeout(t *testing.T) {
	release := make(chan struct{})
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request, _ map[string]any) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	defer close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := client.Classify(ctx, "s", "u")
	if !errors.Is(err, ErrProvider) {
		t.Fatalf("got %v, want ErrProvider on timeout", err)
	}
}

func TestOpenAIClient_BadResponse(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request, _ map[string]any) {
		_, _ = w.Write([]byte(`<html>not json</html>`))
	})
	if _, err := client.Classify(context.Background(), "s", "u"); !errors.Is(err, ErrBadResponse) {
		t.Fatalf("got %v, want ErrBadResponse", err)
	}
}

func TestOpenAIClient_EmptyChoices(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request, _ map[string]any) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	})
	if _, err := client.Classify(context.Background(), "s", "u"); !errors.Is(err, ErrBadResponse) {
		t.Fatalf("got %v, want ErrBadResponse", err)
	}
}

func TestOpenAIClient_RetriesWithoutResponseFormat(t *testing.T) {
	var n atomic.Int32
	client, calls := newTestClient(t, func(w http.ResponseWriter, _ *http.Request, body map[string]any) {
		n.Add(1)
		if _, ok := body["response_format"]; ok {
			http.Error(w, `{"error":{"message":"response_format is not supported by this model"}}`, http.StatusBadRequest)
			return
		}
		writeCompletion(w, `{"action":"keep"}`)
	})
	got, err := client.Classify(context.Background(), "s", "u")
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if got.Content != `{"action":"keep"}` {
		t.Errorf("content: got %q", got.Content)
	}
	if len(*calls) != 2 {
		t.Fatalf("calls: got %d, want 2", len(*calls))
	}
	if _, ok := (*calls)[1].body["response_format"]; ok {
		t.Error("retry must drop response_format")
	}
}

func TestOpenAIClient_OtherBadRequestIsNotRetried(t *testing.T) {
	client, calls := newTestClient(t, func(w http.ResponseWriter, _ *http.Request, _ map[string]any) {
		http.Error(w, `{"error":{"message":"model not found"}}`, http.StatusBadRequest)
	})
	_, err := client.Classify(context.Background(), "s", "u")
	if !errors.Is(err, ErrProvider) {
		t.Fatalf("got %v, want ErrProvider", err)
	}
	if len(*calls) != 1 {
		t.Errorf("calls: got %d, want 1", len(*calls))
	}
}

func assertNoKey(t *testing.T, err error) {
	t.Helper()
	if err != nil && strings.Contains(err.Error(), testAPIKey) {
		t.Errorf("error leaks the API key: %v", err)
	}
}

func TestOpenAIClient_ContextTooLongIsRejectedPerTask(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request, _ map[string]any) {
		http.Error(w, `{"error":{"message":"This model's maximum context length is 8192 tokens"}}`, http.StatusBadRequest)
	})
	_, err := client.Classify(context.Background(), "s", "u")
	if !errors.Is(err, ErrRejected) {
		t.Fatalf("got %v, want ErrRejected", err)
	}
}

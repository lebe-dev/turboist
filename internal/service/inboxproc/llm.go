package inboxproc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/lebe-dev/turboist/internal/logging"
)

var (
	// ErrRateLimited is a 429 from the provider. The whole run backs off.
	ErrRateLimited = errors.New("inboxproc: provider rate limit")
	// ErrProvider covers everything that is not about one particular task: 5xx,
	// network failures, timeouts, a rejected key or an unknown model. The whole
	// run backs off and no task is charged an attempt.
	ErrProvider = errors.New("inboxproc: provider unavailable")
	// ErrRejected is a request the provider refused because of this task (for
	// example it does not fit the model's context). Only that task fails.
	ErrRejected = errors.New("inboxproc: request rejected")
	// ErrBadResponse is an answer that is not a usable completion.
	ErrBadResponse = errors.New("inboxproc: bad provider response")
)

// Classifier sends one system + user message pair to a chat model.
type Classifier interface {
	Classify(ctx context.Context, system, user string) (Completion, error)
}

// Completion is the model's answer plus what it cost.
type Completion struct {
	Content          string
	Model            string
	PromptTokens     int
	CompletionTokens int
}

// OpenAIClientConfig configures an OpenAI-compatible chat completions client.
type OpenAIClientConfig struct {
	// APIURL is the API base; "/chat/completions" is appended.
	APIURL string
	APIKey string
	Model  string
	// Referer is sent as HTTP-Referer, the attribution header OpenRouter reads.
	Referer string
	// Timeout bounds one HTTP exchange when the caller's context does not.
	Timeout time.Duration
}

// OpenAIClient talks to any OpenAI-compatible chat completions endpoint
// (OpenRouter by default). It uses net/http only.
type OpenAIClient struct {
	cfg  OpenAIClientConfig
	http *http.Client
}

func NewOpenAIClient(cfg OpenAIClientConfig) *OpenAIClient {
	cfg.APIURL = strings.TrimRight(cfg.APIURL, "/")
	return &OpenAIClient{cfg: cfg, http: &http.Client{Timeout: cfg.Timeout}}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	Temperature    float64         `json:"temperature"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type chatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

// maxErrorBody caps how much of a refusal is read and quoted in an error.
const maxErrorBody = 512

// Classify asks the model for a JSON answer. Some models on OpenRouter reject
// response_format; a 400 that names it is retried once without the field.
func (c *OpenAIClient) Classify(ctx context.Context, system, user string) (Completion, error) {
	req := chatRequest{
		Model: c.cfg.Model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature:    0,
		ResponseFormat: &responseFormat{Type: "json_object"},
	}
	out, status, body, err := c.do(ctx, req)
	if err == nil {
		return out, nil
	}
	if status == http.StatusBadRequest && strings.Contains(strings.ToLower(body), "response_format") {
		logging.FromContext(ctx).DebugContext(ctx, "inbox processing: retrying without response_format",
			"op", "inboxproc.OpenAIClient.Classify")
		req.ResponseFormat = nil
		out, _, _, err = c.do(ctx, req)
	}
	return out, err
}

func (c *OpenAIClient) do(ctx context.Context, payload chatRequest) (Completion, int, string, error) {
	const op = "inboxproc.OpenAIClient.do"
	raw, err := json.Marshal(payload)
	if err != nil {
		return Completion{}, 0, "", fmt.Errorf("%w: encode request: %v", ErrBadResponse, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.APIURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return Completion{}, 0, "", fmt.Errorf("%w: build request: %v", ErrProvider, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	req.Header.Set("X-Title", "Turboist")
	if c.cfg.Referer != "" {
		req.Header.Set("HTTP-Referer", c.cfg.Referer)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		// *url.Error carries the URL, never headers, so the key cannot leak here.
		return Completion{}, 0, "", fmt.Errorf("%w: %s", ErrProvider, c.redact(err.Error()))
	}
	defer logging.LogClose(ctx, op+".body", resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
		body := c.redact(strings.TrimSpace(string(snippet)))
		return Completion{}, resp.StatusCode, body, classifyStatus(resp.StatusCode, body)
	}

	var decoded chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return Completion{}, resp.StatusCode, "", fmt.Errorf("%w: decode response: %v", ErrBadResponse, err)
	}
	if len(decoded.Choices) == 0 {
		return Completion{}, resp.StatusCode, "", fmt.Errorf("%w: response has no choices", ErrBadResponse)
	}
	model := decoded.Model
	if model == "" {
		model = c.cfg.Model
	}
	return Completion{
		Content:          decoded.Choices[0].Message.Content,
		Model:            model,
		PromptTokens:     decoded.Usage.PromptTokens,
		CompletionTokens: decoded.Usage.CompletionTokens,
	}, resp.StatusCode, "", nil
}

// classifyStatus maps a refusal to the error class that decides whether the run
// backs off or only this task fails.
func classifyStatus(status int, body string) error {
	switch {
	case status == http.StatusTooManyRequests:
		return fmt.Errorf("%w: status %d: %s", ErrRateLimited, status, body)
	case status == http.StatusRequestEntityTooLarge:
		return fmt.Errorf("%w: status %d: %s", ErrRejected, status, body)
	case status == http.StatusBadRequest && mentionsContextLimit(body):
		return fmt.Errorf("%w: status %d: %s", ErrRejected, status, body)
	default:
		return fmt.Errorf("%w: status %d: %s", ErrProvider, status, body)
	}
}

func mentionsContextLimit(body string) bool {
	lower := strings.ToLower(body)
	return strings.Contains(lower, "context length") ||
		strings.Contains(lower, "maximum context") ||
		strings.Contains(lower, "too long")
}

// redact removes the key from provider text before it reaches an error, a log
// line or the status endpoint. Providers occasionally echo the header back.
func (c *OpenAIClient) redact(s string) string {
	if c.cfg.APIKey == "" {
		return s
	}
	return strings.ReplaceAll(s, c.cfg.APIKey, "[redacted]")
}

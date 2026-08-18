package glm

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
)

type Client struct {
	Endpoint string
	APIKey   string
	Model    string
	HTTP     *http.Client

	Retries       int
	RetryInterval time.Duration
}

type Request struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type Message struct {
	Role    string `json:"role"`
	Content []Part `json:"content"`
}

type Part struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

type ImageURL struct {
	URL string `json:"url"`
}

type Response struct {
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *APIError `json:"error,omitempty"`
}

type APIError struct {
	Code    any    `json:"code"`
	Message string `json:"message"`
	Type    string `json:"type"`
}

func (e APIError) Error() string {
	if e.Type != "" {
		return fmt.Sprintf("GLM API error: type=%s code=%v message=%s", e.Type, e.Code, e.Message)
	}
	return fmt.Sprintf("GLM API error: code=%v message=%s", e.Code, e.Message)
}

func (c Client) Complete(ctx context.Context, prompt, imageDataURL string) (string, error) {
	for attempt := 0; ; attempt++ {
		answer, err := c.complete(ctx, prompt, imageDataURL)
		if err == nil {
			return answer, nil
		}
		if attempt >= c.Retries || !shouldRetry(err) {
			return "", err
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(c.retryInterval()):
		}
	}
}
func (c Client) complete(ctx context.Context, prompt, imageDataURL string) (string, error) {
	req := Request{
		Model: c.Model,
		Messages: []Message{{
			Role: "user",
			Content: []Part{
				{Type: "image_url", ImageURL: &ImageURL{URL: imageDataURL}},
				{Type: "text", Text: prompt},
			},
		}},
	}
	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("encode GLM request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create GLM request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("call GLM API: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return "", fmt.Errorf("read GLM response: %w", err)
	}
	var parsed Response
	if err := json.Unmarshal(data, &parsed); err != nil {
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return "", fmt.Errorf("GLM API error: status=%d body=%s", resp.StatusCode, truncate(string(data), 1024))
		}
		return "", fmt.Errorf("decode GLM response: %w", err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		err := &APIError{Code: parsed.Error.Code, Message: parsed.Error.Message, Type: parsed.Error.Type}
		return "", fmt.Errorf("%w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("GLM API error: status=%d message=%s", resp.StatusCode, parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("GLM response contains no choices: %s", truncate(string(data), 1024))
	}
	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if content == "" {
		return "", fmt.Errorf("GLM response contains no content")
	}
	return content, nil
}

func shouldRetry(err error) bool {
	if err == nil {
		return false
	}
	var apiError *APIError
	if errors.As(err, &apiError) {
		return fmt.Sprint(apiError.Code) == "1305"
	}
	return true
}

func (c Client) retryInterval() time.Duration {
	if c.RetryInterval > 0 {
		return c.RetryInterval
	}
	return time.Second
}
func truncate(value string, length int) string {
	if len(value) <= length {
		return value
	}
	return value[:length] + "..."
}

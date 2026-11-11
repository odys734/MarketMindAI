package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"marketmind-ai/internal/config"
)

type Client struct {
	BaseURL    string
	Model      string
	APIKey     string
	HTTPClient *http.Client
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

func NewClient(cfg config.Config) *Client {
	return &Client{
		BaseURL: strings.TrimRight(cfg.AIBaseURL, "/"),
		Model:   cfg.AIModel,
		APIKey:  cfg.AIAPIKey,
		HTTPClient: &http.Client{
			Timeout: time.Duration(cfg.AITimeout) * time.Second,
		},
	}
}

func (c *Client) Chat(
	ctx context.Context,
	systemPrompt string,
	userPrompt string,
) (string, error) {
	if c.BaseURL == "" || c.Model == "" {
		return "", fmt.Errorf("AI base URL or model is not configured")
	}

	payload := chatRequest{
		Model: c.Model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.2,
		MaxTokens:   1200,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode AI request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.BaseURL+"/chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		return "", fmt.Errorf("create AI request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("AI gateway request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errorBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf(
			"AI gateway returned HTTP %d: %s",
			resp.StatusCode,
			string(errorBody),
		)
	}

	var result chatResponse

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode AI response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("AI gateway returned no choices")
	}

	content := strings.TrimSpace(result.Choices[0].Message.Content)
	if content == "" {
		return "", fmt.Errorf("AI gateway returned empty content")
	}

	return content, nil
}

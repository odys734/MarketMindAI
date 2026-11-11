package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestChatSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing authorization header")
		}

		var request chatRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("invalid request JSON: %v", err)
		}

		if request.Model != "test-model" {
			t.Errorf("unexpected model: %s", request.Model)
		}

		if len(request.Messages) != 2 {
			t.Errorf("expected 2 messages, got %d", len(request.Messages))
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"Historical analysis complete."}}]}`)
	}))
	defer server.Close()

	client := &Client{
		BaseURL:    server.URL + "/v1",
		Model:      "test-model",
		APIKey:     "test-key",
		HTTPClient: server.Client(),
	}

	result, err := client.Chat(context.Background(), "system prompt", "user prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "Historical analysis complete." {
		t.Errorf("unexpected response: %s", result)
	}
}

func TestChatRejectsInvalidResponses(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		body      string
		wantError string
	}{
		{
			name:      "HTTP error",
			status:    http.StatusTooManyRequests,
			body:      `{"error":"rate limit"}`,
			wantError: "HTTP 429",
		},
		{
			name:      "invalid JSON",
			status:    http.StatusOK,
			body:      `{invalid`,
			wantError: "decode AI response",
		},
		{
			name:      "no choices",
			status:    http.StatusOK,
			body:      `{"choices":[]}`,
			wantError: "no choices",
		},
		{
			name:      "empty content",
			status:    http.StatusOK,
			body:      `{"choices":[{"message":{"content":"  "}}]}`,
			wantError: "empty content",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(test.status)
				fmt.Fprint(w, test.body)
			}))
			defer server.Close()

			client := &Client{
				BaseURL:    server.URL,
				Model:      "test-model",
				HTTPClient: server.Client(),
			}

			_, err := client.Chat(context.Background(), "system", "user")

			if err == nil {
				t.Fatal("expected an error")
			}

			if !strings.Contains(err.Error(), test.wantError) {
				t.Errorf("expected error containing %q, got %q", test.wantError, err.Error())
			}
		})
	}
}

func TestChatRejectsMissingConfiguration(t *testing.T) {
	client := &Client{}

	_, err := client.Chat(context.Background(), "system", "user")

	if err == nil {
		t.Fatal("expected configuration error")
	}
}

package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"resty.dev/v3"
)

func TestClient_AnswerMeanings(t *testing.T) {
	tests := []struct {
		name              string
		request           inference.AnswerMeaningsRequest
		mockServerHandler func(t *testing.T, w http.ResponseWriter, r *http.Request)

		wantResponse    inference.AnswerMeaningsResponse
		wantError       bool
		wantErrorString string
	}{
		{
			name: "Success with single context and usage",
			request: inference.AnswerMeaningsRequest{
				Expressions: []inference.Expression{
					{
						Expression: "run",
						Meaning:    "to move quickly",
						Contexts: []inference.Context{
							{
								Context: "I need to run to the store.",
								Usage:   "run",
							},
						},
						IsExpressionInput: false,
					},
				},
			},
			mockServerHandler: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				// Verify request
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/chat/completions", r.URL.Path)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				var reqBody ChatCompletionRequest
				err := json.NewDecoder(r.Body).Decode(&reqBody)
				require.NoError(t, err)
				assert.Equal(t, "gpt-4", reqBody.Model)
				assert.NotEmpty(t, reqBody.Messages)

				// Return mock response
				mockResponse := ChatCompletionResponse{
					ID:      "chatcmpl-123",
					Object:  "chat.completion",
					Created: 1677652288,
					Model:   "gpt-4",
					Choices: []Choice{
						{
							Index: 0,
							Message: ChoiceMessage{
								Role: RoleAssistant,
								Content: `[{
									"expression": "run",
									"is_expression_input": false,
									"meaning": "to move quickly",
									"answers": [
										{"correct": true, "context": "I need to run to the store."}
									]
								}]`,
							},
							FinishReason: "stop",
						},
					},
					Usage: Usage{
						PromptTokens:     100,
						CompletionTokens: 50,
						TotalTokens:      150,
					},
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(mockResponse)
			},
			wantResponse: inference.AnswerMeaningsResponse{
				Answers: []inference.AnswerMeaning{
					{
						Expression: "run",
						Meaning:    "to move quickly",
						AnswersForContext: []inference.AnswersForContext{
							{Correct: true, Context: "I need to run to the store."},
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "Multiple contexts with different usage forms",
			request: inference.AnswerMeaningsRequest{
				Expressions: []inference.Expression{
					{
						Expression: "run",
						Meaning:    "to move quickly",
						Contexts: []inference.Context{
							{
								Context: "I need to run to the store.",
								Usage:   "run",
							},
							{
								Context: "She runs a successful company.",
								Usage:   "runs",
							},
						},
						IsExpressionInput: false,
					},
				},
			},
			mockServerHandler: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				// Verify request body contains usage field
				var reqBody ChatCompletionRequest
				err := json.NewDecoder(r.Body).Decode(&reqBody)
				require.NoError(t, err)

				var userMessage string
				for _, msg := range reqBody.Messages {
					if msg.Role == RoleUser {
						userMessage = msg.Content
						break
					}
				}
				assert.Contains(t, userMessage, "usage")

				mockResponse := ChatCompletionResponse{
					ID:      "chatcmpl-456",
					Object:  "chat.completion",
					Created: 1677652288,
					Model:   "gpt-4",
					Choices: []Choice{
						{
							Index: 0,
							Message: ChoiceMessage{
								Role: RoleAssistant,
								Content: `[{
									"expression": "run",
									"is_expression_input": false,
									"meaning": "to move quickly; to operate",
									"answers": [
										{"correct": true, "context": "I need to run to the store."},
										{"correct": false, "context": "She runs a successful company."}
									]
								}]`,
							},
							FinishReason: "stop",
						},
					},
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(mockResponse)
			},

			wantResponse: inference.AnswerMeaningsResponse{
				Answers: []inference.AnswerMeaning{
					{
						Expression: "run",
						Meaning:    "to move quickly; to operate",
						AnswersForContext: []inference.AnswersForContext{
							{Correct: true, Context: "I need to run to the store."},
							{Correct: false, Context: "She runs a successful company."},
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "Empty expressions - no HTTP request",
			request: inference.AnswerMeaningsRequest{
				Expressions: []inference.Expression{},
			},
			mockServerHandler: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				t.Error("HTTP request should not be made for empty expressions")
			},
			wantResponse: inference.AnswerMeaningsResponse{},
		},
		{
			name: "HTTP 500 error",
			request: inference.AnswerMeaningsRequest{
				Expressions: []inference.Expression{
					{
						Expression: "test",
						Meaning:    "a test",
						Contexts: []inference.Context{
							{Context: "This is a test.", Usage: "test"},
						},
					},
				},
			},
			mockServerHandler: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error": {"message": "Internal server error"}}`))
			},
			wantError: true,
		},
		{
			name: "Invalid JSON response",
			request: inference.AnswerMeaningsRequest{
				Expressions: []inference.Expression{
					{
						Expression: "test",
						Meaning:    "a test",
						Contexts: []inference.Context{
							{Context: "This is a test.", Usage: "test"},
						},
					},
				},
			},
			mockServerHandler: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				mockResponse := ChatCompletionResponse{
					ID:      "chatcmpl-789",
					Object:  "chat.completion",
					Created: 1677652288,
					Model:   "gpt-4",
					Choices: []Choice{
						{
							Index: 0,
							Message: ChoiceMessage{
								Role:    RoleAssistant,
								Content: `invalid json content`,
							},
							FinishReason: "stop",
						},
					},
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(mockResponse)
			},

			wantError:       true,
			wantErrorString: "json.Unmarshal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock HTTP server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tt.mockServerHandler(t, w, r)
			}))
			defer server.Close()

			// Create client with mock server
			client := &Client{
				httpClient:       resty.New().SetBaseURL(server.URL),
				model:            "gpt-4",
				maxRetryAttempts: 1,
			}

			// Execute test
			ctx := context.Background()
			gotResponse, gotErr := client.AnswerMeanings(ctx, tt.request)

			// Assert error expectations
			if tt.wantError {
				require.Error(t, gotErr)
				if tt.wantErrorString != "" {
					assert.Contains(t, gotErr.Error(), tt.wantErrorString)
				}
				return
			}

			require.NoError(t, gotErr)
			require.Equal(t, tt.wantResponse, gotResponse)
		})
	}
}

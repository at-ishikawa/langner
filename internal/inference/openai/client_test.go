package openai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/at-ishikawa/langner/internal/inference"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"resty.dev/v3"
)

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "json unmarshal error",
			err:  errors.New("json.Unmarshal failed"),
			want: true,
		},
		{
			name: "unexpected end of JSON input",
			err:  errors.New("unexpected end of JSON input"),
			want: true,
		},
		{
			name: "connection refused",
			err:  errors.New("connection refused"),
			want: true,
		},
		{
			name: "i/o timeout",
			err:  errors.New("i/o timeout"),
			want: true,
		},
		{
			name: "server error 500",
			err:  errors.New("response error 500"),
			want: true,
		},
		{
			name: "server error 503",
			err:  errors.New("response error 503"),
			want: true,
		},
		{
			name: "rate limiting 429",
			err:  errors.New("response error 429"),
			want: true,
		},
		{
			name: "client error 400 - not retryable",
			err:  errors.New("response error 400: bad request"),
			want: false,
		},
		{
			name: "generic error - not retryable",
			err:  errors.New("something went wrong"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRetryableError(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestClient_getRequestBody(t *testing.T) {
	tests := []struct {
		name    string
		model   string
		args    inference.AnswerMeaningsRequest
		wantErr bool
	}{
		{
			name:  "single expression with context",
			model: "gpt-4",
			args: inference.AnswerMeaningsRequest{
				Expressions: []inference.Expression{
					{
						Expression: "break the ice",
						Meaning:    "to initiate conversation in an awkward situation",
						Contexts: []inference.Context{
							{
								Context:             "She told a joke to break the ice at the party.",
								ReferenceDefinition: "to make people feel more relaxed",
								Usage:               "break the ice",
							},
						},
						IsExpressionInput: false,
					},
				},
			},
		},
		{
			name:  "multiple expressions",
			model: "gpt-4o-mini",
			args: inference.AnswerMeaningsRequest{
				Expressions: []inference.Expression{
					{
						Expression: "piece of cake",
						Meaning:    "something easy",
						Contexts: []inference.Context{
							{Context: "The test was a piece of cake.", Usage: "piece of cake"},
						},
					},
					{
						Expression: "hit the road",
						Meaning:    "to leave",
						Contexts: []inference.Context{
							{Context: "We should hit the road before it gets dark.", Usage: "hit the road"},
						},
					},
				},
			},
		},
		{
			name:  "expression with is_expression_input true",
			model: "gpt-4",
			args: inference.AnswerMeaningsRequest{
				Expressions: []inference.Expression{
					{
						Expression:    "runing",
						Meaning:       "to move quickly on foot",
						Contexts:      []inference.Context{{Context: "I was runing in the park."}},
						IsExpressionInput: true,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{model: tt.model}

			got, err := client.getRequestBody(tt.args)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Verify model
			assert.Equal(t, tt.model, got.Model)

			// Verify temperature
			assert.InDelta(t, 0.3, got.Temperature, 0.001)

			// Verify messages structure: system + few-shot examples (pairs) + user request
			assert.NotEmpty(t, got.Messages)
			assert.Equal(t, RoleSystem, got.Messages[0].Role)

			// Last message should be the user request
			lastMsg := got.Messages[len(got.Messages)-1]
			assert.Equal(t, RoleUser, lastMsg.Role)

			// Verify user request contains expressions
			for _, expr := range tt.args.Expressions {
				assert.Contains(t, lastMsg.Content, expr.Expression)
			}
		})
	}
}

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

func TestClient_ValidateWordForm(t *testing.T) {
	tests := []struct {
		name              string
		request           inference.ValidateWordFormRequest
		mockServerHandler func(t *testing.T, w http.ResponseWriter, r *http.Request)
		wantResponse      inference.ValidateWordFormResponse
		wantError         bool
		wantErrorString   string
	}{
		{
			name: "same_word classification",
			request: inference.ValidateWordFormRequest{
				Expected:   "run",
				UserAnswer: "ran",
				Meaning:    "to move quickly on foot",
				Context:    "I need to run to the store.",
			},
			mockServerHandler: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/chat/completions", r.URL.Path)

				mockResponse := ChatCompletionResponse{
					Choices: []Choice{
						{
							Message: ChoiceMessage{
								Role:    RoleAssistant,
								Content: `{"classification": "same_word", "reason": "ran is the past tense of run"}`,
							},
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(mockResponse)
			},
			wantResponse: inference.ValidateWordFormResponse{
				Classification: inference.ClassificationSameWord,
				Reason:         "ran is the past tense of run",
			},
		},
		{
			name: "synonym classification",
			request: inference.ValidateWordFormRequest{
				Expected:   "happy",
				UserAnswer: "joyful",
				Meaning:    "feeling pleasure",
			},
			mockServerHandler: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				mockResponse := ChatCompletionResponse{
					Choices: []Choice{
						{
							Message: ChoiceMessage{
								Role:    RoleAssistant,
								Content: `{"classification": "synonym", "reason": "joyful is a synonym of happy"}`,
							},
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(mockResponse)
			},
			wantResponse: inference.ValidateWordFormResponse{
				Classification: inference.ClassificationSynonym,
				Reason:         "joyful is a synonym of happy",
			},
		},
		{
			name: "wrong classification",
			request: inference.ValidateWordFormRequest{
				Expected:   "happy",
				UserAnswer: "sad",
				Meaning:    "feeling pleasure",
			},
			mockServerHandler: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				mockResponse := ChatCompletionResponse{
					Choices: []Choice{
						{
							Message: ChoiceMessage{
								Role:    RoleAssistant,
								Content: `{"classification": "wrong", "reason": "sad is an antonym of happy"}`,
							},
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(mockResponse)
			},
			wantResponse: inference.ValidateWordFormResponse{
				Classification: inference.ClassificationWrong,
				Reason:         "sad is an antonym of happy",
			},
		},
		{
			name: "HTTP error",
			request: inference.ValidateWordFormRequest{
				Expected:   "test",
				UserAnswer: "test",
				Meaning:    "a trial",
			},
			mockServerHandler: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error": "server error"}`))
			},
			wantError: true,
		},
		{
			name: "empty choices in response",
			request: inference.ValidateWordFormRequest{
				Expected:   "test",
				UserAnswer: "test",
				Meaning:    "a trial",
			},
			mockServerHandler: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				mockResponse := ChatCompletionResponse{
					Choices: []Choice{},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(mockResponse)
			},
			wantError:       true,
			wantErrorString: "empty response body or choices",
		},
		{
			name: "empty content in response",
			request: inference.ValidateWordFormRequest{
				Expected:   "test",
				UserAnswer: "test",
				Meaning:    "a trial",
			},
			mockServerHandler: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				mockResponse := ChatCompletionResponse{
					Choices: []Choice{
						{
							Message: ChoiceMessage{
								Role:    RoleAssistant,
								Content: "",
							},
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(mockResponse)
			},
			wantError:       true,
			wantErrorString: "empty response content",
		},
		{
			name: "invalid JSON in response content",
			request: inference.ValidateWordFormRequest{
				Expected:   "test",
				UserAnswer: "test",
				Meaning:    "a trial",
			},
			mockServerHandler: func(t *testing.T, w http.ResponseWriter, r *http.Request) {
				mockResponse := ChatCompletionResponse{
					Choices: []Choice{
						{
							Message: ChoiceMessage{
								Role:    RoleAssistant,
								Content: "not valid json",
							},
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(mockResponse)
			},
			wantError:       true,
			wantErrorString: "json.Unmarshal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tt.mockServerHandler(t, w, r)
			}))
			defer server.Close()

			client := &Client{
				httpClient:       resty.New().SetBaseURL(server.URL),
				model:            "gpt-4",
				maxRetryAttempts: 0,
			}

			ctx := context.Background()
			got, err := client.ValidateWordForm(ctx, tt.request)

			if tt.wantError {
				require.Error(t, err)
				if tt.wantErrorString != "" {
					assert.Contains(t, err.Error(), tt.wantErrorString)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantResponse, got)
		})
	}
}

func TestNewClient(t *testing.T) {
	client := NewClient("test-key", "gpt-4", 3)
	assert.NotNil(t, client)
	assert.Equal(t, "gpt-4", client.GetModel())
	assert.Equal(t, uint(3), client.maxRetryAttempts)
}

func TestClient_GetModel(t *testing.T) {
	client := &Client{model: "gpt-4o-mini"}
	assert.Equal(t, "gpt-4o-mini", client.GetModel())
}

package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

const (
	openAIEndpoint = "https://api.openai.com/v1/chat/completions"
	defaultTimeout = 30 * time.Second
)

// openAIRequest represents the request body for OpenAI API
type openAIRequest struct {
	Model          string          `json:"model"`
	Messages       []openAIMessage `json:"messages"`
	Temperature    float64         `json:"temperature"`
	MaxTokens      int             `json:"max_tokens"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

// openAIMessage represents a single message in the conversation (system, user, assistant)
type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// responseFormat specifies what OpenAI should return in the response (JSON format)
type responseFormat struct {
	Type string `json:"type"`
}

// openAIResponse represents the response from OpenAI API
type openAIResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

// Client represents an LLM client for credit evaluation
type Client struct {
	apiKey     string
	model      string
	logger     *zap.Logger
	httpClient *http.Client
	timeout    time.Duration
}

// New creates a new LLM client
func New(apiKey, model string, logger *zap.Logger) *Client {
	return &Client{
		apiKey:     apiKey,
		model:      model,
		logger:     logger,
		httpClient: &http.Client{Timeout: defaultTimeout},
		timeout:    defaultTimeout,
	}
}

// EvaluateCredit main function that evaluates a credit request using the LLM and returns a response
func (client *Client) EvaluateCredit(
	ctx context.Context,
	input *CreditEvaluationRequest,
) (*CreditEvaluationResponse, error) {
	// Validates the openAI API key
	if client.apiKey == "" {
		return nil, ErrAPIKeyMissing
	}

	// Validates the input
	if input == nil {
		return nil, ErrInvalidInput
	}

	if err := client.validateInput(input); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Builds the prompt
	userPrompt, err := buildPrompt(input)
	if err != nil {
		return nil, fmt.Errorf("failed to build prompt: %w", err)
	}

	// Calls to OpenAI API
	responseText, err := client.callOpenAI(ctx, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("openai api call failed: %w", err)
	}

	// Parses the OpenAI response
	decision, err := client.parseResponse(responseText)
	if err != nil {
		client.logger.Error("failed to parse llm response",
			zap.Error(err),
			zap.String("response", responseText),
		)

		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Logs the info of the credit evaluation and returns the response
	client.logger.Info("credit evaluation completed",
		zap.String("user_id", input.UserProfile.UserID),
		zap.String("decision", decision.Decision),
		zap.Float64("amount", input.Request.Amount),
		zap.String("currency", input.Request.Currency),
	)

	return decision, nil
}

// validateInput is a helper that validates the credit evaluation input
func (client *Client) validateInput(input *CreditEvaluationRequest) error {
	if input.Request.Amount <= 0 {
		return fmt.Errorf("%w: amount must be positive", ErrInvalidInput)
	}

	if input.Request.Currency == "" {
		return fmt.Errorf("%w: currency is required", ErrInvalidInput)
	}

	if input.Request.DurationMonths <= 0 {
		return fmt.Errorf("%w: duration must be positive", ErrInvalidInput)
	}

	if input.UserProfile.UserID == "" {
		return fmt.Errorf("%w: user_id is required", ErrInvalidInput)
	}

	if input.PoolStatus.Currency == "" {
		return fmt.Errorf("%w: pool currency is required", ErrInvalidInput)
	}

	return nil
}

// callOpenAI is a helper that makes an HTTP request to OpenAI API
func (client *Client) callOpenAI(ctx context.Context, userPrompt string) (string, error) {
	// Prepares request body
	reqBody := openAIRequest{
		Model: client.model,
		Messages: []openAIMessage{
			{
				Role:    "system",
				Content: getSystemPrompt(),
			},
			{
				Role:    "user",
				Content: userPrompt,
			},
		},
		Temperature:    0.3, // Lower temperature for more consistent evaluation
		MaxTokens:      2000,
		ResponseFormat: &responseFormat{Type: "json_object"},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Creates the POST request
	req, err := http.NewRequestWithContext(ctx, "POST", openAIEndpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+client.apiKey)

	// Executes the request
	resp, err := client.httpClient.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", ErrAPITimeout
		}

		return "", fmt.Errorf("http request failed: %w", err)
	}

	defer resp.Body.Close()

	// Reads the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Parses response
	var apiResp openAIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		client.logger.Error("failed to parse openai response",
			zap.Error(err),
			zap.String("body", string(body)),
		)

		return "", fmt.Errorf("failed to parse api response: %w", err)
	}

	// Check for API errors
	if apiResp.Error != nil {
		if strings.Contains(apiResp.Error.Type, "rate_limit") {
			return "", ErrRateLimited
		}
		return "", fmt.Errorf("openai api error: %s", apiResp.Error.Message)
	}

	// Checks for the errors on the response status
	if resp.StatusCode != http.StatusOK {
		client.logger.Error("openai api returned non-200 status",
			zap.Int("status_code", resp.StatusCode),
			zap.String("body", string(body)),
		)

		return "", fmt.Errorf("openai api returned status %d", resp.StatusCode)
	}

	// Extracts the content from the response
	if len(apiResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in openai response")
	}

	// Debugs calls params and returns the content extracted
	content := apiResp.Choices[0].Message.Content

	client.logger.Debug("openai api call successful",
		zap.Int("prompt_tokens", apiResp.Usage.PromptTokens),
		zap.Int("completion_tokens", apiResp.Usage.CompletionTokens),
		zap.Int("total_tokens", apiResp.Usage.TotalTokens),
	)

	return content, nil
}

// parseResponse is a helper that parses the LLM response into a CreditEvaluationResponse
func (client *Client) parseResponse(responseText string) (*CreditEvaluationResponse, error) {
	// PArses the OpenAI response in a CreditEvaluationResponse
	var decision CreditEvaluationResponse
	if err := json.Unmarshal([]byte(responseText), &decision); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidResponse, err)
	}

	// Validates that the credit has been approved or rejected
	if decision.Decision != "APPROVED" && decision.Decision != "REJECTED" {
		return nil, fmt.Errorf("%w: decision must be APPROVED or REJECTED", ErrInvalidResponse)
	}

	// Validates if the credit has the rest of needed params
	switch decision.Decision {
	case "REJECTED":
		if decision.RejectionReason == nil || *decision.RejectionReason == "" {
			return nil, fmt.Errorf(
				"%w: rejection reason is required for REJECTED decision",
				ErrInvalidResponse,
			)
		}

	case "APPROVED":
		if decision.CreditTerms == nil {
			return nil, fmt.Errorf("%w: credit terms are required for APPROVED decision", ErrInvalidResponse)
		}

		if len(decision.RepaymentSchedule) == 0 {
			return nil, fmt.Errorf("%w: repayment schedule is required for APPROVED decision", ErrInvalidResponse)
		}

		if decision.TotalRepayment == nil || decision.TotalInterest == nil {
			return nil, fmt.Errorf("%w: total repayment and interest are required for APPROVED decision", ErrInvalidResponse)
		}
	}

	return &decision, nil
}

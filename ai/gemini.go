package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time" 
)

const geminiBaseURL = "https://generativelanguage.googleapis.com/v1beta"

// GeminiClient communicates with Google's Gemini AI API
type GeminiClient struct {
	APIKey     string
	HTTPClient *http.Client
	Model      string
}

// Gemini API request/response structures
type GeminiRequest struct {
	Contents          []GeminiContent        `json:"contents"`
	SystemInstruction *GeminiContent         `json:"systemInstruction,omitempty"`
	GenerationConfig  GeminiGenerationConfig `json:"generationConfig,omitempty"`
}

type GeminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []GeminiPart `json:"parts"`
}

type GeminiPart struct {
	Text string `json:"text"`
}

type GeminiGenerationConfig struct {
	Temperature     float32 `json:"temperature"`
	TopK            int     `json:"topK"`
	TopP            float32 `json:"topP"`
	MaxOutputTokens int     `json:"maxOutputTokens"`
}

type GeminiResponse struct {
	Candidates []GeminiCandidate `json:"candidates"`
	Error      *GeminiError      `json:"error,omitempty"`
}

type GeminiCandidate struct {
	Content GeminiResponseContent `json:"content"`
}

type GeminiResponseContent struct {
	Parts []GeminiPart `json:"parts"`
	Role  string       `json:"role"`
}

type GeminiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// Global client instance
var geminiClient *GeminiClient

// GetGeminiClient returns singleton Gemini client
func GetGeminiClient() (*GeminiClient, error) {
	if geminiClient != nil {
		return geminiClient, nil
	}

	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY environment variable not set")
	}

	geminiClient = &GeminiClient{
		APIKey: apiKey,
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		Model: "gemini-1.5-flash", // Fast and efficient model
	}

	return geminiClient, nil
}

// Chat sends conversation to Gemini and returns AI response
func (c *GeminiClient) Chat(messages []Message, systemPrompt string) (string, error) {
	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s",
		geminiBaseURL, c.Model, c.APIKey)

	// Build contents from messages
	var contents []GeminiContent
	for _, msg := range messages {
		role := msg.Role
		// Gemini uses "user" and "model" (not "assistant")
		if role == "assistant" {
			role = "model"
		}

		contents = append(contents, GeminiContent{
			Role:  role,
			Parts: []GeminiPart{{Text: msg.Content}},
		})
	}

	reqBody := GeminiRequest{
		Contents: contents,
		GenerationConfig: GeminiGenerationConfig{
			Temperature:     0.7,
			TopK:            40,
			TopP:            0.95,
			MaxOutputTokens: 2048,
		},
	}

	// Add system instruction if provided
	if systemPrompt != "" {
		reqBody.SystemInstruction = &GeminiContent{
			Parts: []GeminiPart{{Text: systemPrompt}},
		}
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var response GeminiResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	// Check for API error in response
	if response.Error != nil {
		return "", fmt.Errorf("Gemini API error: %s", response.Error.Message)
	}

	if len(response.Candidates) == 0 || len(response.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("empty response from Gemini API")
	}

	return response.Candidates[0].Content.Parts[0].Text, nil
}

// ChatSimple is a convenience method for single-turn chat
func (c *GeminiClient) ChatSimple(userMessage string, systemPrompt string) (string, error) {
	messages := []Message{
		{Role: "user", Content: userMessage},
	}
	return c.Chat(messages, systemPrompt)
}

// ChatWithContext sends message with conversation history
func (c *GeminiClient) ChatWithContext(history []Message, newMessage string, systemPrompt string) (string, error) {
	messages := append(history, Message{Role: "user", Content: newMessage})
	return c.Chat(messages, systemPrompt)
}

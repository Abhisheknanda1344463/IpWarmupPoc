package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
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
		Model: "gemini-2.0-flash", // Fast and efficient model
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

// DetectIntent detects user intent from message
// Returns: "change_domain", "proceed", "cancel", or "other"
func (c *GeminiClient) DetectIntent(userMessage string) string {
	prompt := `You are an intent classifier. Analyze the user's message and respond with ONLY ONE of these exact words:

CHANGE_DOMAIN - if user wants to check/verify/test a different domain, change domain, try another domain, go back, start over, recheck, modify their choice
PROCEED - if user wants to continue, says yes, confirms, agrees, wants to proceed with current action
CANCEL - if user wants to stop, exit, cancel, says no, declines
OTHER - if none of the above, user is asking a question or saying something else

User message: "` + userMessage + `"

Respond with ONLY the intent word (CHANGE_DOMAIN, PROCEED, CANCEL, or OTHER). Nothing else.`

	response, err := c.ChatSimple(prompt, "You are a strict intent classifier. Only respond with one word.")
	if err != nil {
		return "other"
	}

	// Clean and normalize response
	response = strings.TrimSpace(strings.ToLower(response))
	
	switch {
	case strings.Contains(response, "change_domain"):
		return "change_domain"
	case strings.Contains(response, "proceed"):
		return "proceed"
	case strings.Contains(response, "cancel"):
		return "cancel"
	default:
		return "other"
	}
}

// Intent types for navigation
type UserIntent string

const (
	IntentChangeDomain   UserIntent = "change_domain"
	IntentChangeVolume   UserIntent = "change_volume"
	IntentChangeDays     UserIntent = "change_days"
	IntentGoBack         UserIntent = "go_back"
	IntentProceed        UserIntent = "proceed"
	IntentCancel         UserIntent = "cancel"
	IntentOther          UserIntent = "other"
)

// DetectUserIntent uses quick keyword check first, then AI fallback
func DetectUserIntent(userMessage string) UserIntent {
	lower := strings.ToLower(userMessage)

	// STEP 1: Quick keyword check for common patterns (instant response)
	
	// Check for domain change keywords
	domainKeywords := []string{"domain", "website", "site", "url", "another", "different", "new", "other", "start over", "reset", "restart"}
	changeWords := []string{"change", "modify", "switch", "try", "check", "verify", "test"}
	
	hasDomainWord := false
	for _, kw := range domainKeywords {
		if strings.Contains(lower, kw) {
			hasDomainWord = true
			break
		}
	}
	
	hasChangeWord := false
	for _, kw := range changeWords {
		if strings.Contains(lower, kw) {
			hasChangeWord = true
			break
		}
	}
	
	// Domain change detection
	if hasDomainWord && hasChangeWord {
		return IntentChangeDomain
	}
	if strings.Contains(lower, "start over") || strings.Contains(lower, "reset") || strings.Contains(lower, "restart") {
		return IntentChangeDomain
	}
	
	// Volume change detection
	volumeKeywords := []string{"volume", "email", "target", "emails"}
	hasVolumeWord := false
	for _, kw := range volumeKeywords {
		if strings.Contains(lower, kw) {
			hasVolumeWord = true
			break
		}
	}
	// Also detect "need more emails", "want higher volume", "increase target"
	volumeActionWords := []string{"change", "modify", "more", "less", "increase", "decrease", "need", "want", "different", "adjust", "higher", "lower"}
	hasVolumeAction := false
	for _, kw := range volumeActionWords {
		if strings.Contains(lower, kw) {
			hasVolumeAction = true
			break
		}
	}
	if hasVolumeWord && (hasChangeWord || hasVolumeAction) {
		return IntentChangeVolume
	}
	
	// Days change detection
	daysKeywords := []string{"days", "day", "warmup", "duration", "period"}
	hasDaysWord := false
	for _, kw := range daysKeywords {
		if strings.Contains(lower, kw) {
			hasDaysWord = true
			break
		}
	}
	// Also detect "need more days", "want more days", "increase days"
	daysActionWords := []string{"change", "modify", "more", "less", "increase", "decrease", "need", "want", "different", "adjust"}
	hasDaysAction := false
	for _, kw := range daysActionWords {
		if strings.Contains(lower, kw) {
			hasDaysAction = true
			break
		}
	}
	if hasDaysWord && (hasChangeWord || hasDaysAction) {
		return IntentChangeDays
	}
	
	// Go back detection
	backKeywords := []string{"go back", "back", "previous", "undo"}
	for _, kw := range backKeywords {
		if strings.Contains(lower, kw) {
			return IntentGoBack
		}
	}

	// STEP 2: AI fallback for complex sentences
	client, err := GetGeminiClient()
	if err != nil {
		return IntentOther
	}

	prompt := `Classify this user message for a domain warmup chatbot. Respond with ONLY one word:

CHANGE_DOMAIN - wants different domain/website/site
CHANGE_VOLUME - wants to change email volume/target
CHANGE_DAYS - wants to change warmup days
GO_BACK - wants to go back
OTHER - anything else

Message: "` + userMessage + `"

One word only:`

	response, err := client.ChatSimple(prompt, "Respond with exactly one word.")
	if err != nil {
		return IntentOther
	}

	response = strings.TrimSpace(strings.ToUpper(response))

	switch {
	case strings.Contains(response, "CHANGE_DOMAIN"):
		return IntentChangeDomain
	case strings.Contains(response, "CHANGE_VOLUME"):
		return IntentChangeVolume
	case strings.Contains(response, "CHANGE_DAYS"):
		return IntentChangeDays
	case strings.Contains(response, "GO_BACK"):
		return IntentGoBack
	default:
		return IntentOther
	}
}

// DetectChangeDomainIntent checks if user wants to change/check another domain
func DetectChangeDomainIntent(userMessage string) bool {
	intent := DetectUserIntent(userMessage)
	return intent == IntentChangeDomain
}

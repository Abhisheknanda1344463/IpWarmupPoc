package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// getBaseURL returns the base URL for internal API calls
func getBaseURL() string {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	return fmt.Sprintf("http://localhost:%s", port)
}

// ============================================================================
// REQUEST/RESPONSE TYPES
// ============================================================================

type ChatRequest struct {
	SessionID string `json:"session_id"`
	Message   string `json:"message"`
}

type ChatResponse struct {
	SessionID  string `json:"session_id"`
	Reply      string `json:"reply"`
	Stage      string `json:"stage"`
	WaitingFor string `json:"waiting_for,omitempty"` // what input we expect next
	DomainData any    `json:"domain_data,omitempty"` // vetting result if available
	WarmupPlan any    `json:"warmup_plan,omitempty"` // warmup plan if generated
	CanProceed bool   `json:"can_proceed"`           // can proceed with warmup?
	Error      string `json:"error,omitempty"`
}

// ============================================================================
// SESSION MANAGEMENT (in-memory for POC)
// ============================================================================

type Session struct {
	ID           string
	Stage        string
	Messages     []Message
	Domain       string
	VettingData  map[string]any
	Score        int
	ScoreLabel   string
	WarmupDays   int
	CreatedAt    time.Time
	LastActivity time.Time
}

var (
	sessions   = make(map[string]*Session)
	sessionsMu sync.RWMutex
)

func getOrCreateSession(id string) *Session {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()

	if sess, ok := sessions[id]; ok {
		sess.LastActivity = time.Now()
		return sess
	}

	sess := &Session{
		ID:           id,
		Stage:        "greeting",
		Messages:     []Message{},
		CreatedAt:    time.Now(),
		LastActivity: time.Now(),
	}
	sessions[id] = sess
	return sess
}

// ============================================================================
// MAIN CHAT HANDLER
// ============================================================================

func ChatHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != "POST" {
		sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Generate session ID if not provided
	if req.SessionID == "" {
		req.SessionID = fmt.Sprintf("sess_%d", time.Now().UnixNano())
	}

	session := getOrCreateSession(req.SessionID)
	response := processChat(session, req.Message)

	json.NewEncoder(w).Encode(response)
}

// StartChatHandler - Initialize a new chat session
func StartChatHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	sessionID := fmt.Sprintf("sess_%d", time.Now().UnixNano())
	session := getOrCreateSession(sessionID)

	// Get greeting from backend
	greeting := GetStageQuestion("greeting")

	session.Messages = append(session.Messages, Message{
		Role:    "assistant",
		Content: greeting,
	})

	response := ChatResponse{
		SessionID:  sessionID,
		Reply:      greeting,
		Stage:      "greeting",
		WaitingFor: "domain",
		CanProceed: true,
	}

	json.NewEncoder(w).Encode(response)
}

// ============================================================================
// CHAT PROCESSING LOGIC (Backend-Driven)
// ============================================================================

func processChat(session *Session, userMessage string) ChatResponse {
	// Add user message to history
	session.Messages = append(session.Messages, Message{
		Role:    "user",
		Content: userMessage,
	})

	var response ChatResponse
	response.SessionID = session.ID

	switch session.Stage {
	case "greeting":
		// User should provide domain
		response = handleDomainInput(session, userMessage)

	case "domain_analyzed":
		// User responds to domain analysis - check if they want warmup
		response = handleWarmupConfirmation(session, userMessage)

	case "warmup_days":
		// User provides warmup days
		response = handleWarmupDays(session, userMessage)

	case "plan_generated":
		// Follow-up questions after plan
		response = handleFollowup(session, userMessage)

	default:
		response = handleFollowup(session, userMessage)
	}

	// Add assistant response to history
	session.Messages = append(session.Messages, Message{
		Role:    "assistant",
		Content: response.Reply,
	})

	return response
}

// ============================================================================
// STAGE HANDLERS
// ============================================================================

func handleDomainInput(session *Session, userMessage string) ChatResponse {
	domain := extractDomain(userMessage)
	if domain == "" {
		return ChatResponse{
			SessionID:  session.ID,
			Reply:      "I couldn't identify a valid domain. Please enter a domain like 'example.com' or 'mail.example.com'.",
			Stage:      "greeting",
			WaitingFor: "domain",
			CanProceed: true,
		}
	}

	// Check if domain exists (DNS lookup)
	if !isDomainValid(domain) {
		return ChatResponse{
			SessionID:  session.ID,
			Reply:      fmt.Sprintf("❌ **'%s' is not a valid domain.** This domain doesn't exist or has no DNS records. Please enter a real, active domain.", domain),
			Stage:      "greeting",
			WaitingFor: "domain",
			CanProceed: true,
		}
	}

	session.Domain = domain

	// Check if user also provided days upfront
	daysProvided := extractDays(userMessage)

	// Call vetting API
	vettingData, err := callVettingAPI(domain)
	if err != nil {
		return ChatResponse{
			SessionID:  session.ID,
			Reply:      fmt.Sprintf("❌ **Unable to check '%s'**. The domain might be unreachable or our service is temporarily unavailable. Please try again.", domain),
			Stage:      "greeting",
			WaitingFor: "domain",
			CanProceed: true,
			Error:      err.Error(),
		}
	}

	session.VettingData = vettingData

	// Extract score from vetting data
	if summary, ok := vettingData["summary"].(map[string]any); ok {
		if score, ok := summary["score"].(float64); ok {
			session.Score = int(score)
		}
		if label, ok := summary["level"].(string); ok {
			session.ScoreLabel = label
		}
	}

	if session.ScoreLabel == "" {
		session.ScoreLabel = ScoreInterpretation(session.Score)
	}

	canProceed := CanProceedWithWarmup(session.Score)

	// FAST PATH: If user provided days AND domain can proceed → directly generate warmup plan
	if daysProvided > 0 && canProceed {
		session.WarmupDays = daysProvided

		// Get quick domain summary + warmup plan together
		warmupData, err := callWarmupAPI(daysProvided)
		if err != nil {
			// Fallback to AI-generated plan
			plan := generateCombinedResponse(session, vettingData)
			session.Stage = "plan_generated"
			return ChatResponse{
				SessionID:  session.ID,
				Reply:      plan,
				Stage:      session.Stage,
				WaitingFor: "freetext",
				DomainData: vettingData,
				WarmupPlan: buildWarmupPlanData(session),
				CanProceed: true,
			}
		}

		// Generate combined analysis + warmup plan
		plan := generateCombinedResponseWithData(session, vettingData, warmupData)
		session.Stage = "plan_generated"

		return ChatResponse{
			SessionID:  session.ID,
			Reply:      plan,
			Stage:      session.Stage,
			WaitingFor: "freetext",
			DomainData: vettingData,
			WarmupPlan: warmupData,
			CanProceed: true,
		}
	}

	// SLOW PATH: Normal flow - show analysis first
	aiResponse := getAIAnalysis(session, vettingData)
	session.Stage = "domain_analyzed"

	waitingFor := "confirmation"
	if !canProceed {
		waitingFor = "freetext"
		session.Stage = "plan_generated" // Skip warmup for bad domains
	}

	return ChatResponse{
		SessionID:  session.ID,
		Reply:      aiResponse,
		Stage:      session.Stage,
		WaitingFor: waitingFor,
		DomainData: vettingData,
		CanProceed: canProceed,
	}
}

func handleWarmupConfirmation(session *Session, userMessage string) ChatResponse {
	lower := strings.ToLower(userMessage)

	// Check if user wants to proceed
	positiveWords := []string{"yes", "yeah", "yep", "sure", "ok", "okay", "proceed", "continue", "warmup", "warm up", "plan", "haan", "ha", "ji"}
	negativeWords := []string{"no", "nope", "nah", "cancel", "stop", "exit", "nahi", "na"}

	isPositive := false
	isNegative := false

	for _, word := range positiveWords {
		if strings.Contains(lower, word) {
			isPositive = true
			break
		}
	}

	for _, word := range negativeWords {
		if strings.Contains(lower, word) {
			isNegative = true
			break
		}
	}

	if isNegative {
		session.Stage = "plan_generated"
		return ChatResponse{
			SessionID:  session.ID,
			Reply:      "No problem! Feel free to ask me anything else about your domain or email deliverability. 👋",
			Stage:      session.Stage,
			WaitingFor: "freetext",
			CanProceed: true,
		}
	}

	if isPositive || extractDays(userMessage) > 0 {
		// Check if they already mentioned days
		days := extractDays(userMessage)
		if days > 0 {
			return handleWarmupDays(session, userMessage)
		}

		session.Stage = "warmup_days"
		return ChatResponse{
			SessionID:  session.ID,
			Reply:      GetStageQuestion("warmup_days"),
			Stage:      session.Stage,
			WaitingFor: "days",
			CanProceed: true,
		}
	}

	// Unclear response - ask again
	return ChatResponse{
		SessionID:  session.ID,
		Reply:      "Would you like me to create a warmup plan for your domain? Just say 'yes' or 'no', or tell me how many days you'd like (e.g., '14 days').",
		Stage:      session.Stage,
		WaitingFor: "confirmation",
		CanProceed: true,
	}
}

func handleWarmupDays(session *Session, userMessage string) ChatResponse {
	days := extractDays(userMessage)

	if days <= 0 || days > 90 {
		return ChatResponse{
			SessionID:  session.ID,
			Reply:      "Please enter a valid number of days between 1 and 90. Common options are 14, 21, or 30 days.",
			Stage:      "warmup_days",
			WaitingFor: "days",
			CanProceed: true,
		}
	}

	session.WarmupDays = days

	// Call the actual warmup API with Excel formula
	warmupData, err := callWarmupAPI(days)
	if err != nil {
		// Fallback to AI-generated plan if API fails
		plan := generateWarmupPlan(session)
		session.Stage = "plan_generated"
		return ChatResponse{
			SessionID:  session.ID,
			Reply:      plan,
			Stage:      session.Stage,
			WaitingFor: "freetext",
			WarmupPlan: buildWarmupPlanData(session),
			CanProceed: true,
		}
	}

	// Format the warmup plan using AI with actual data
	plan := formatWarmupPlanWithAI(session, warmupData)

	session.Stage = "plan_generated"

	return ChatResponse{
		SessionID:  session.ID,
		Reply:      plan,
		Stage:      session.Stage,
		WaitingFor: "freetext",
		WarmupPlan: warmupData,
		CanProceed: true,
	}
}

func handleFollowup(session *Session, userMessage string) ChatResponse {
	// Check if user wants to check a NEW domain
	newDomain := extractDomain(userMessage)
	if newDomain != "" && newDomain != session.Domain {
		// User entered a new domain - reset session and process as new domain
		session.Domain = ""
		session.VettingData = nil
		session.Score = 0
		session.ScoreLabel = ""
		session.WarmupDays = 0
		session.Stage = "greeting"
		session.Messages = []Message{} // Clear history for fresh start

		return handleDomainInput(session, userMessage)
	}

	// Check for keywords that indicate user wants to check another domain
	lower := strings.ToLower(userMessage)
	resetKeywords := []string{"new domain", "another domain", "check another", "different domain", "naya domain", "dusra domain", "start over", "reset", "restart"}
	for _, keyword := range resetKeywords {
		if strings.Contains(lower, keyword) {
			session.Stage = "greeting"
			return ChatResponse{
				SessionID:  session.ID,
				Reply:      "Sure! Please enter the domain you'd like to check (e.g., example.com):",
				Stage:      "greeting",
				WaitingFor: "domain",
				CanProceed: true,
			}
		}
	}

	// Use Gemini for general follow-up questions
	aiResponse := getAIFollowup(session, userMessage)

	return ChatResponse{
		SessionID:  session.ID,
		Reply:      aiResponse,
		Stage:      session.Stage,
		WaitingFor: "freetext",
		CanProceed: true,
	}
}

// ============================================================================
// AI HELPERS (Using Simple Gemini Client)
// ============================================================================

func getAIAnalysis(session *Session, vettingData map[string]any) string {
	client, err := GetGeminiClient()
	if err != nil {
		return generateFallbackAnalysis(session)
	}

	// Format vetting data for prompt
	vettingJSON, _ := json.MarshalIndent(vettingData, "", "  ")
	prompt := fmt.Sprintf(StageAnalyzingPrompt, string(vettingJSON))

	// Build messages with context
	messages := []Message{
		{Role: "user", Content: prompt},
	}

	response, err := client.Chat(messages, SystemPrompt)
	if err != nil {
		return generateFallbackAnalysis(session)
	}

	return response
}

func getAIFollowup(session *Session, userMessage string) string {
	client, err := GetGeminiClient()
	if err != nil {
		return "I'm having trouble connecting to my AI backend. Please try again."
	}

	// Build context
	contextInfo := ""
	if session.Domain != "" {
		contextInfo = fmt.Sprintf("Context: Domain=%s, Score=%d/100 (%s), WarmupDays=%d\n\n",
			session.Domain, session.Score, session.ScoreLabel, session.WarmupDays)
	}

	// Build conversation history for Gemini
	var messages []Message
	for _, msg := range session.Messages {
		messages = append(messages, Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// Add context and followup prompt
	fullPrompt := contextInfo + StageFollowupPrompt + "\n\nUser's question: " + userMessage
	messages = append(messages, Message{Role: "user", Content: fullPrompt})

	response, err := client.Chat(messages, SystemPrompt)
	if err != nil {
		return "I'm having trouble processing your question. Could you try rephrasing it?"
	}

	return response
}

func generateWarmupPlan(session *Session) string {
	client, err := GetGeminiClient()
	if err != nil {
		return generateFallbackWarmupPlan(session)
	}

	prompt := fmt.Sprintf(StageGeneratePlanPrompt,
		session.Domain,
		session.Score,
		session.ScoreLabel,
		session.WarmupDays,
	)

	response, err := client.ChatSimple(prompt, SystemPrompt)
	if err != nil {
		return generateFallbackWarmupPlan(session)
	}

	return response
}

// callWarmupAPI calls the backend warmup API with Excel formula
func callWarmupAPI(days int) (map[string]any, error) {
	// Default target volume - can be made configurable
	targetVolume := 10000

	reqBody, _ := json.Marshal(map[string]int{
		"target_volume": targetVolume,
		"days":          days,
	})

	resp, err := http.Post(getBaseURL()+"/warmup", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("warmup API error: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

// formatWarmupPlanWithAI uses AI to present the warmup data nicely
func formatWarmupPlanWithAI(session *Session, warmupData map[string]any) string {
	client, err := GetGeminiClient()
	if err != nil {
		return formatWarmupPlanFallback(session, warmupData)
	}

	// Get the appropriate plan based on days
	var planToUse []any
	planKey := "plan_30_day"
	if session.WarmupDays < 30 {
		planKey = "plan_less_than_30"
	} else if session.WarmupDays > 30 {
		planKey = "plan_greater_than_30"
	}

	if plan, ok := warmupData[planKey].([]any); ok {
		// Limit to requested days
		if len(plan) > session.WarmupDays {
			planToUse = plan[:session.WarmupDays]
		} else {
			planToUse = plan
		}
	}

	planJSON, _ := json.MarshalIndent(planToUse, "", "  ")

	prompt := fmt.Sprintf(`Present this warmup plan for domain %s in a friendly, readable format.

Domain Score: %d/100 (%s)
Warmup Period: %d days
Plan Type: %s

Raw Plan Data (day and daily email limit):
%s

Instructions:
- Summarize the plan in phases (early days, middle, ramp-up)
- Highlight key milestones
- Add practical tips based on the domain score
- Keep it concise but informative
- Use emojis sparingly for visual appeal`,
		session.Domain, session.Score, session.ScoreLabel, session.WarmupDays, planKey, string(planJSON))

	response, err := client.ChatSimple(prompt, SystemPrompt)
	if err != nil {
		return formatWarmupPlanFallback(session, warmupData)
	}

	return response
}

// generateCombinedResponse generates domain analysis + warmup plan in one response (fallback)
func generateCombinedResponse(session *Session, vettingData map[string]any) string {
	client, err := GetGeminiClient()
	if err != nil {
		return generateCombinedFallback(session, vettingData)
	}

	vettingJSON, _ := json.MarshalIndent(vettingData, "", "  ")

	prompt := fmt.Sprintf(`User asked for warmup plan for domain %s for %d days. Provide a COMBINED response:

1. BRIEF domain analysis (2-3 lines max) with score %d/100 (%s)
2. Then directly show the warmup plan

Domain Vetting Data:
%s

Keep it concise - user wants quick results. Use emojis sparingly.`,
		session.Domain, session.WarmupDays, session.Score, session.ScoreLabel, string(vettingJSON))

	response, err := client.ChatSimple(prompt, SystemPrompt)
	if err != nil {
		return generateCombinedFallback(session, vettingData)
	}

	return response
}

// generateCombinedResponseWithData generates combined response with actual warmup data
func generateCombinedResponseWithData(session *Session, vettingData map[string]any, warmupData map[string]any) string {
	client, err := GetGeminiClient()
	if err != nil {
		return generateCombinedFallback(session, vettingData)
	}

	// Get the appropriate plan
	var planToUse []any
	planKey := "plan_30_day"
	if session.WarmupDays < 30 {
		planKey = "plan_less_than_30"
	} else if session.WarmupDays > 30 {
		planKey = "plan_greater_than_30"
	}

	if plan, ok := warmupData[planKey].([]any); ok {
		if len(plan) > session.WarmupDays {
			planToUse = plan[:session.WarmupDays]
		} else {
			planToUse = plan
		}
	}

	vettingJSON, _ := json.MarshalIndent(vettingData, "", "  ")
	planJSON, _ := json.MarshalIndent(planToUse, "", "  ")

	prompt := fmt.Sprintf(`User asked for warmup plan for domain %s for %d days. Provide a SINGLE COMBINED response:

1. BRIEF domain health check (2-3 lines) - Score: %d/100 (%s)
2. Directly present the %d-day warmup plan

Domain Data:
%s

Warmup Plan Data (day and daily limit):
%s

Instructions:
- Keep domain analysis SHORT (user wants quick results)
- Show plan in phases with key milestones
- Add 2-3 practical tips at end
- Use emojis sparingly`,
		session.Domain, session.WarmupDays, session.Score, session.ScoreLabel, session.WarmupDays,
		string(vettingJSON), string(planJSON))

	response, err := client.ChatSimple(prompt, SystemPrompt)
	if err != nil {
		return generateCombinedFallback(session, vettingData)
	}

	return response
}

// generateCombinedFallback is the non-AI fallback for combined response
func generateCombinedFallback(session *Session, vettingData map[string]any) string {
	var result string

	// Brief domain analysis
	switch {
	case session.Score >= 80:
		result = fmt.Sprintf("✅ **%s** - Excellent! Score: **%d/100**. Ready for warmup.\n\n", session.Domain, session.Score)
	case session.Score >= 60:
		result = fmt.Sprintf("👍 **%s** - Good. Score: **%d/100**. Minor issues, but can proceed.\n\n", session.Domain, session.Score)
	case session.Score >= 40:
		result = fmt.Sprintf("⚠️ **%s** - Medium. Score: **%d/100**. Proceed with caution.\n\n", session.Domain, session.Score)
	default:
		result = fmt.Sprintf("❌ **%s** - Poor. Score: **%d/100**. Contact deliverability team.\n\n", session.Domain, session.Score)
	}

	// Add warmup plan
	result += generateFallbackWarmupPlan(session)

	return result
}

// formatWarmupPlanFallback formats warmup plan without AI
func formatWarmupPlanFallback(session *Session, warmupData map[string]any) string {
	// Get the appropriate plan based on days
	planKey := "plan_30_day"
	planLabel := "30-Day Plan"
	if session.WarmupDays < 30 {
		planKey = "plan_less_than_30"
		planLabel = "Accelerated Plan (<30 days)"
	} else if session.WarmupDays > 30 {
		planKey = "plan_greater_than_30"
		planLabel = "Extended Plan (>30 days)"
	}

	plan := fmt.Sprintf("📧 **%d-Day Warmup Plan for %s**\n", session.WarmupDays, session.Domain)
	plan += fmt.Sprintf("Plan Type: %s\n", planLabel)
	plan += fmt.Sprintf("Domain Score: %d/100 (%s)\n\n", session.Score, session.ScoreLabel)

	if planData, ok := warmupData[planKey].([]any); ok {
		plan += "**Daily Email Limits:**\n"

		// Show first week in detail
		plan += "\n*Week 1 (Warmup Start):*\n"
		for i := 0; i < 7 && i < len(planData) && i < session.WarmupDays; i++ {
			if dayData, ok := planData[i].(map[string]any); ok {
				day := int(dayData["day"].(float64))
				limit := int(dayData["limit"].(float64))
				plan += fmt.Sprintf("• Day %d: %d emails\n", day, limit)
			}
		}

		// Show week 2 summary
		if session.WarmupDays > 7 {
			plan += "\n*Week 2:*\n"
			for i := 7; i < 14 && i < len(planData) && i < session.WarmupDays; i++ {
				if dayData, ok := planData[i].(map[string]any); ok {
					day := int(dayData["day"].(float64))
					limit := int(dayData["limit"].(float64))
					plan += fmt.Sprintf("• Day %d: %d emails\n", day, limit)
				}
			}
		}

		// Show remaining milestones
		if session.WarmupDays > 14 {
			plan += "\n*Key Milestones:*\n"
			milestones := []int{21, 30, 45, 60}
			for _, m := range milestones {
				if m <= session.WarmupDays && m <= len(planData) {
					if dayData, ok := planData[m-1].(map[string]any); ok {
						limit := int(dayData["limit"].(float64))
						plan += fmt.Sprintf("• Day %d: %d emails\n", m, limit)
					}
				}
			}
		}

		// Final day
		if session.WarmupDays > 1 {
			finalIdx := session.WarmupDays - 1
			if finalIdx < len(planData) {
				if dayData, ok := planData[finalIdx].(map[string]any); ok {
					limit := int(dayData["limit"].(float64))
					plan += fmt.Sprintf("\n🎯 **Final Day %d Target: %d emails**\n", session.WarmupDays, limit)
				}
			}
		}
	}

	plan += "\n**Tips:**\n"
	plan += "• Send to your most engaged contacts first\n"
	plan += "• Monitor bounce rates daily\n"
	plan += "• Pause if spam complaints exceed 0.1%\n"

	return plan
}

// ============================================================================
// FALLBACK RESPONSES (when AI is unavailable)
// ============================================================================

func generateFallbackAnalysis(session *Session) string {
	scoreLabel := ScoreInterpretation(session.Score)

	var analysis string
	switch {
	case session.Score >= 80:
		analysis = fmt.Sprintf("✅ Great news! Your domain **%s** has an excellent reputation score of **%d/100**. Your email infrastructure looks solid and you're ready for warmup.", session.Domain, session.Score)
	case session.Score >= 60:
		analysis = fmt.Sprintf("👍 Your domain **%s** has a good reputation score of **%d/100**. There are minor issues, but you can proceed with warmup.", session.Domain, session.Score)
	case session.Score >= 40:
		analysis = fmt.Sprintf("⚠️ Your domain **%s** has a medium reputation score of **%d/100**. There are some concerns, but warmup is still possible with caution.", session.Domain, session.Score)
	default:
		analysis = fmt.Sprintf("❌ Your domain **%s** has a poor reputation score of **%d/100**. I recommend contacting the deliverability team before attempting warmup.", session.Domain, session.Score)
	}

	if CanProceedWithWarmup(session.Score) {
		analysis += "\n\nWould you like me to create a warmup plan for you?"
	}

	_ = scoreLabel // used in AI version
	return analysis
}

func generateFallbackWarmupPlan(session *Session) string {
	days := session.WarmupDays
	startVolume := 50
	if session.Score < 60 {
		startVolume = 25
	}

	plan := fmt.Sprintf("📧 **%d-Day Warmup Plan for %s**\n\n", days, session.Domain)
	plan += fmt.Sprintf("Starting reputation: %d/100 (%s)\n\n", session.Score, session.ScoreLabel)

	// Generate phases
	phases := []struct {
		name    string
		days    int
		volumes []int
	}{
		{"Phase 1 - Foundation", days / 4, []int{startVolume, startVolume * 2, startVolume * 3}},
		{"Phase 2 - Building", days / 4, []int{startVolume * 5, startVolume * 8, startVolume * 12}},
		{"Phase 3 - Scaling", days / 4, []int{startVolume * 20, startVolume * 35, startVolume * 50}},
		{"Phase 4 - Full Volume", days / 4, []int{startVolume * 75, startVolume * 100, startVolume * 150}},
	}

	currentDay := 1
	for _, phase := range phases {
		plan += fmt.Sprintf("**%s (Days %d-%d)**\n", phase.name, currentDay, currentDay+phase.days-1)
		for i := 0; i < phase.days && currentDay <= days; i++ {
			volIdx := i % len(phase.volumes)
			plan += fmt.Sprintf("• Day %d: %d emails\n", currentDay, phase.volumes[volIdx])
			currentDay++
		}
		plan += "\n"
	}

	plan += "**Tips:**\n"
	plan += "• Send to your most engaged contacts first\n"
	plan += "• Monitor bounce rates closely\n"
	plan += "• Stop if you see spam complaints spike\n"

	return plan
}

// ============================================================================
// UTILITY FUNCTIONS
// ============================================================================

// isDomainValid checks if domain exists via DNS lookup or WHOIS
// Returns true for:
// 1. Domains with DNS records (A, MX, NS)
// 2. Registered domains even without DNS (valid WHOIS)
func isDomainValid(domain string) bool {
	// Set a short timeout for DNS lookup
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{
				Timeout: 3 * time.Second,
			}
			return d.DialContext(ctx, network, address)
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try to lookup IP addresses
	ips, err := resolver.LookupIP(ctx, "ip", domain)
	if err == nil && len(ips) > 0 {
		return true
	}

	// Also try MX records (some domains only have MX)
	mxs, err := resolver.LookupMX(ctx, domain)
	if err == nil && len(mxs) > 0 {
		return true
	}

	// Also try NS records
	nss, err := resolver.LookupNS(ctx, domain)
	if err == nil && len(nss) > 0 {
		return true
	}

	// DNS failed - but domain might still be registered
	// Accept domains that look valid (have proper TLD structure)
	// The vetting API will do detailed WHOIS check and show warnings
	// This allows domains like cathoderay.co.in (registered but no DNS) to proceed
	return isValidDomainFormat(domain)
}

// isValidDomainFormat checks if domain has valid format for common TLDs
// This is a fallback when DNS fails - allows registered domains without active DNS
func isValidDomainFormat(domain string) bool {
	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		return false
	}

	// Check common country-code TLDs with second-level domains
	// e.g., .co.in, .co.uk, .com.au, etc.
	knownSLDs := map[string]bool{
		"co.in": true, "co.uk": true, "co.nz": true, "co.za": true,
		"com.au": true, "com.br": true, "com.mx": true, "com.sg": true,
		"net.in": true, "org.in": true, "org.uk": true, "gov.in": true,
		"ac.in": true, "edu.in": true, "res.in": true, "gen.in": true,
	}

	// Check if last two parts form a known SLD
	if len(parts) >= 3 {
		sld := parts[len(parts)-2] + "." + parts[len(parts)-1]
		if knownSLDs[sld] {
			return true
		}
	}

	// Check common gTLDs
	knownTLDs := map[string]bool{
		"com": true, "net": true, "org": true, "io": true, "co": true,
		"dev": true, "app": true, "ai": true, "in": true, "uk": true,
		"us": true, "de": true, "fr": true, "jp": true, "cn": true,
		"ru": true, "br": true, "au": true, "ca": true, "edu": true,
		"gov": true, "mil": true, "int": true, "info": true, "biz": true,
	}

	lastPart := parts[len(parts)-1]
	return knownTLDs[lastPart]
}

func extractDomain(input string) string {
	input = strings.TrimSpace(input)
	input = strings.ToLower(input)

	// Remove common prefixes
	input = strings.TrimPrefix(input, "http://")
	input = strings.TrimPrefix(input, "https://")
	input = strings.TrimPrefix(input, "www.")

	// Remove paths
	if idx := strings.Index(input, "/"); idx != -1 {
		input = input[:idx]
	}

	// Domain validation - supports multi-level TLDs like .co.in, .co.uk, etc.
	// Pattern: alphanumeric start, can have hyphens, then at least one dot followed by more segments
	// Examples: example.com, example.co.in, sub.example.co.uk
	domainRegex := regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]*(\.[a-zA-Z0-9][a-zA-Z0-9-]*)+$`)
	if domainRegex.MatchString(input) {
		return input
	}

	// Try to extract domain from text
	words := strings.Fields(input)
	for _, word := range words {
		word = strings.Trim(word, ".,!?")
		if domainRegex.MatchString(word) {
			return word
		}
	}

	return ""
}

func extractDays(input string) int {
	// Look for numbers in the input
	re := regexp.MustCompile(`\d+`)
	matches := re.FindAllString(input, -1)

	for _, match := range matches {
		if days, err := strconv.Atoi(match); err == nil && days > 0 && days <= 90 {
			return days
		}
	}

	return 0
}

func callVettingAPI(domain string) (map[string]any, error) {
	// Call our own vetting endpoint
	reqBody, _ := json.Marshal(map[string]string{"domain": domain})

	// Use localhost since we're calling ourselves
	resp, err := http.Post(getBaseURL()+"/vet", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("vetting API error: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

func buildWarmupPlanData(session *Session) map[string]any {
	return map[string]any{
		"domain":      session.Domain,
		"days":        session.WarmupDays,
		"score":       session.Score,
		"score_label": session.ScoreLabel,
	}
}

func sendError(w http.ResponseWriter, message string, status int) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

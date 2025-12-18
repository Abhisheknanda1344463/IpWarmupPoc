package ai

import "time"

// Message represents a single chat message
type Message struct {
	Role      string    `json:"role"` // "user" or "assistant"
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// ConversationStage defines possible stages in the chat flow
type ConversationStage string

const (
	StageGreeting       ConversationStage = "greeting"
	StageDomainAnalyzed ConversationStage = "domain_analyzed"
	StageWarmupDays     ConversationStage = "warmup_days"
	StagePlanGenerated  ConversationStage = "plan_generated"
	StageFollowup       ConversationStage = "followup"
)

// ConversationState stores entire flow state
type ConversationState struct {
	SessionID    string            `json:"session_id"`
	Stage        ConversationStage `json:"stage"`
	Messages     []Message         `json:"messages"`
	Domain       string            `json:"domain,omitempty"`
	VettingData  map[string]any    `json:"vetting_data,omitempty"`
	Score        int               `json:"score,omitempty"`
	ScoreLabel   string            `json:"score_label,omitempty"`
	WarmupDays   int               `json:"warmup_days,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	LastActivity time.Time         `json:"last_activity"`
}

// NewConversation creates a new conversation state
func NewConversation(sessionID string) *ConversationState {
	return &ConversationState{
		SessionID:    sessionID,
		Stage:        StageGreeting,
		Messages:     []Message{},
		CreatedAt:    time.Now(),
		LastActivity: time.Now(),
	}
}

// AddMessage appends a message to conversation history
func (c *ConversationState) AddMessage(role, content string) {
	c.Messages = append(c.Messages, Message{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	})
	c.LastActivity = time.Now()
}

// GetLastUserMessage returns the most recent user message
func (c *ConversationState) GetLastUserMessage() string {
	for i := len(c.Messages) - 1; i >= 0; i-- {
		if c.Messages[i].Role == "user" {
			return c.Messages[i].Content
		}
	}
	return ""
}

// GetMessageHistory returns messages formatted for LLM context
func (c *ConversationState) GetMessageHistory() []map[string]string {
	history := make([]map[string]string, len(c.Messages))
	for i, msg := range c.Messages {
		history[i] = map[string]string{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}
	return history
}

// SetDomainData stores vetting results
func (c *ConversationState) SetDomainData(domain string, data map[string]any, score int, label string) {
	c.Domain = domain
	c.VettingData = data
	c.Score = score
	c.ScoreLabel = label
}

// TransitionTo moves to a new stage
func (c *ConversationState) TransitionTo(stage ConversationStage) {
	c.Stage = stage
	c.LastActivity = time.Now()
}

// CanProceedToWarmup checks if domain is healthy enough
func (c *ConversationState) CanProceedToWarmup() bool {
	return c.Score >= 40
}

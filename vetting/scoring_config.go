package vetting

// ScoringWeights defines configurable weights for scoring calculation
// These can be adjusted by AI agent based on real-world performance data
// type ScoringWeights struct {
// 	// Website checks
// 	HTTPSMissing     int `json:"https_missing"`      // Default: 20
// 	TLSExpiringSoon  int `json:"tls_expiring_soon"`  // Default: 10
// 	WebsiteNotExists int `json:"website_not_exists"` // Default: 15

// 	// Domain age
// 	DomainTooNew int `json:"domain_too_new"` // Default: 20

// 	// Reputation
// 	SenderScoreLow  int `json:"sender_score_low"`  // Default: 10
// 	BlacklistPerHit int `json:"blacklist_per_hit"` // Default: 10
// 	GoogleFlagged   int `json:"google_flagged"`    // Default: 30
// 	SpamhausHigh    int `json:"spamhaus_high"`     // Default: 20

// 	// Email security
// 	NoMXRecord int `json:"no_mx_record"` // Default: 10
// 	NoSPF      int `json:"no_spf"`       // Default: 10
// 	NoDMARC    int `json:"no_dmarc"`     // Default: 10

// 	// Website scores (traffic & trust)
// 	TrafficScoreLow    int `json:"traffic_score_low"`    // Default: 10
// 	TrafficScoreMedium int `json:"traffic_score_medium"` // Default: 5
// 	TrustScoreLow      int `json:"trust_score_low"`      // Default: 15
// 	TrustScoreMedium   int `json:"trust_score_medium"`   // Default: 8

// 	// Opt-in (mandatory)
// 	OptInNonCompliant int `json:"optin_non_compliant"` // Default: 25
// 	NoCaptcha         int `json:"no_captcha"`          // Default: 5
// }

// DefaultScoringWeights returns default weights (current hard-coded values)
// func DefaultScoringWeights() ScoringWeights {
// 	return ScoringWeights{
// 		HTTPSMissing:       20,
// 		TLSExpiringSoon:    10,
// 		WebsiteNotExists:   15,
// 		DomainTooNew:       20,
// 		SenderScoreLow:     10,
// 		BlacklistPerHit:    10,
// 		GoogleFlagged:      30,
// 		SpamhausHigh:       20,
// 		NoMXRecord:         10,
// 		NoSPF:              10,
// 		NoDMARC:            10,
// 		TrafficScoreLow:    10,
// 		TrafficScoreMedium: 5,
// 		TrustScoreLow:      15,
// 		TrustScoreMedium:   8,
// 		OptInNonCompliant:  25,
// 		NoCaptcha:          5,
// 	}
// }

// ScoringThresholds defines thresholds for risk levels
// AI can adjust these based on actual outcomes
type ScoringThresholds struct {
	HighRiskMax int `json:"high_risk_max"` // Default: 40
	MediumMax   int `json:"medium_max"`    // Default: 70
	// Good: 71-100
}

// DefaultScoringThresholds returns default thresholds
func DefaultScoringThresholds() ScoringThresholds {
	return ScoringThresholds{
		HighRiskMax: 40,
		MediumMax:   70,
	}
}

// ScoringFeatures extracts all features for ML/AI training
// This helps AI agent learn which factors matter most
// type ScoringFeatures struct {
// 	// Binary features
// 	HasHTTPS       bool `json:"has_https"`
// 	WebsiteExists  bool `json:"website_exists"`
// 	HasValidMX     bool `json:"has_valid_mx"`
// 	HasSPF         bool `json:"has_spf"`
// 	HasDMARC       bool `json:"has_dmarc"`
// 	GoogleFlagged  bool `json:"google_flagged"`
// 	OptInCompliant bool `json:"optin_compliant"`
// 	HasCaptcha     bool `json:"has_captcha"`

// 	// Numeric features
// 	TLSDaysLeft     int     `json:"tls_days_left"`
// 	WhoisAgeDays    int     `json:"whois_age_days"`
// 	BlacklistCount  int     `json:"blacklist_count"`
// 	SenderScore     int     `json:"sender_score"`
// 	SpamhausScore   float64 `json:"spamhaus_score"`
// 	TrafficScore    int     `json:"traffic_score"`
// 	TrustScore      int     `json:"trust_score"`
// 	SSLQualityScore int     `json:"ssl_quality_score"`

// 	// Calculated risk indicators
// 	IsNewDomain      bool `json:"is_new_domain"`       // < 60 days
// 	IsTLSExpiring    bool `json:"is_tls_expiring"`     // < 30 days
// 	IsLowSenderScore bool `json:"is_low_sender_score"` // < 60
// 	IsHighSpamhaus   bool `json:"is_high_spamhaus"`    // > 30
// 	IsLowTraffic     bool `json:"is_low_traffic"`      // < 5
// 	IsLowTrust       bool `json:"is_low_trust"`        // < 5
// }

// PenaltyBreakdown shows which penalties were applied and their values
type PenaltyBreakdown struct {
	HTTPSMissing       int `json:"https_missing,omitempty"`
	TLSExpiringSoon    int `json:"tls_expiring_soon,omitempty"`
	WebsiteNotExists   int `json:"website_not_exists,omitempty"`
	DomainTooNew       int `json:"domain_too_new,omitempty"`
	SenderScoreLow     int `json:"sender_score_low,omitempty"`
	BlacklistPenalty   int `json:"blacklist_penalty,omitempty"`
	BlacklistCount     int `json:"blacklist_count,omitempty"`
	GoogleFlagged      int `json:"google_flagged,omitempty"`
	SpamhausHigh       int `json:"spamhaus_high,omitempty"`
	NoMXRecord         int `json:"no_mx_record,omitempty"`
	NoSPF              int `json:"no_spf,omitempty"`
	NoDMARC            int `json:"no_dmarc,omitempty"`
	DMARCPolicyNone    int `json:"dmarc_policy_none,omitempty"` // p=none penalty
	TrafficScoreLow    int `json:"traffic_score_low,omitempty"`
	TrafficScoreMedium int `json:"traffic_score_medium,omitempty"`
	TrustScoreLow      int `json:"trust_score_low,omitempty"`
	TrustScoreMedium   int `json:"trust_score_medium,omitempty"`
	OptInNonCompliant  int `json:"optin_non_compliant,omitempty"`
	NoCaptcha          int `json:"no_captcha,omitempty"`
	TotalPenalties     int `json:"total_penalties"`
	FinalScore         int `json:"final_score"`
	StartingScore      int `json:"starting_score"`
}

// ExtractFeatures extracts all features from domain vetting results
// This is used for ML model training and AI learning
// func ExtractFeatures(
// 	httpsOK bool,
// 	tlsDays int,
// 	whoisDays int,
// 	blacklistCount int,
// 	mxRep int,
// 	googleFlagged bool,
// 	email EmailSecurity,
// 	ssl SSLQuality,
// 	spam SpamhausResponse,
// 	optIn OptInCheck,
// 	website WebsiteCheck,
// ) ScoringFeatures {
// 	return ScoringFeatures{
// 		HasHTTPS:       httpsOK,
// 		WebsiteExists:  website.Exists,
// 		HasValidMX:     email.HasValidMX,
// 		HasSPF:         email.HasSPF,
// 		HasDMARC:       email.HasDMARC,
// 		GoogleFlagged:  googleFlagged,
// 		OptInCompliant: optIn.Compliance,
// 		HasCaptcha:     optIn.HasCaptcha,

// 		TLSDaysLeft:     tlsDays,
// 		WhoisAgeDays:    whoisDays,
// 		BlacklistCount:  blacklistCount,
// 		SenderScore:     mxRep,
// 		SpamhausScore:   spam.Score,
// 		TrafficScore:    website.TrafficScore,
// 		TrustScore:      website.TrustScore,
// 		SSLQualityScore: ssl.Score,

// 		IsNewDomain:      whoisDays < 60,
// 		IsTLSExpiring:    tlsDays < 30,
// 		IsLowSenderScore: mxRep < 60,
// 		IsHighSpamhaus:   spam.Score > 30,
// 		IsLowTraffic:     website.TrafficScore < 5,
// 		IsLowTrust:       website.TrustScore < 5,
// 	}
// }

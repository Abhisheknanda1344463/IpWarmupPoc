package vetting

import (
	"fmt"
	"strings"
)

type RiskSummary struct {
	Score  int    `json:"score"`
	Level  string `json:"level"`
	Reason string `json:"reason"`
	// AI-ready fields
	// Features  ScoringFeatures  `json:"features,omitempty"`  // For ML training
	// Weights   ScoringWeights   `json:"weights,omitempty"`   // Weights used
	Breakdown PenaltyBreakdown `json:"breakdown,omitempty"` // Penalty breakdown
}

// CalculateScore calculates risk score with configurable weights
// If weights are nil, uses default weights
func CalculateScore(
	httpsOK bool,
	tlsDays int,
	whoisDays int,
	blacklistCount int,
	mxRep int,
	googleFlagged bool,
	email EmailSecurity,
	ssl SSLQuality,
	spam SpamhausResponse,
	optIn OptInCheck,
	website WebsiteCheck,
) RiskSummary {
	return CalculateScoreWithWeights(
		httpsOK, tlsDays, whoisDays, blacklistCount, mxRep,
		googleFlagged, email, ssl, spam, optIn, website,
	)
}

// CalculateScoreWithWeights calculates score with custom weights (AI-ready)
func CalculateScoreWithWeights(
	httpsOK bool,
	tlsDays int,
	whoisDays int,
	blacklistCount int,
	mxRep int,
	googleFlagged bool,
	email EmailSecurity,
	ssl SSLQuality,
	spam SpamhausResponse,

	optIn OptInCheck,
	website WebsiteCheck,
) RiskSummary {
	thresholds := DefaultScoringThresholds()

	// Initialize penalty breakdown
	breakdown := PenaltyBreakdown{
		StartingScore:  100,
		BlacklistCount: blacklistCount,
	}

	score := 100

	// Hard-coded weights (default values)
	const (
		weightHTTPSMissing      = 20
		weightTLSExpiringSoon   = 10
		weightWebsiteNotExists  = 15
		weightDomainTooNew      = 20
		weightSenderScoreLow    = 10
		weightBlacklistPerHit   = 10
		weightGoogleFlagged     = 30
		weightSpamhausHigh      = 20
		weightNoMXRecord        = 10
		weightNoSPF             = 10
		weightNoDMARC           = 10
		weightTrafficScoreLow   = 10
		weightTrafficScoreMed   = 5
		weightTrustScoreLow     = 15
		weightTrustScoreMed     = 8
		weightOptInNonCompliant = 25
		weightNoCaptcha         = 5
	)

	// HTTPS
	if !httpsOK {
		penalty := weightHTTPSMissing
		score -= penalty
		breakdown.HTTPSMissing = penalty
	}

	// TLS expiry
	if tlsDays < 30 {
		penalty := weightTLSExpiringSoon
		score -= penalty
		breakdown.TLSExpiringSoon = penalty
	}

	// Domain age
	if whoisDays < 60 {
		penalty := weightDomainTooNew
		score -= penalty
		breakdown.DomainTooNew = penalty
	}

	// MXRep (Sender Score)
	if mxRep < 60 {
		penalty := weightSenderScoreLow
		score -= penalty
		breakdown.SenderScoreLow = penalty
	}

	// Blacklists
	if blacklistCount > 0 {
		penalty := blacklistCount * weightBlacklistPerHit
		score -= penalty
		breakdown.BlacklistPenalty = penalty
	}

	// Safe Browsing
	if googleFlagged {
		penalty := weightGoogleFlagged
		score -= penalty
		breakdown.GoogleFlagged = penalty
	}

	// Email Security
	if !email.HasValidMX {
		penalty := weightNoMXRecord
		score -= penalty
		breakdown.NoMXRecord = penalty
	}
	if !email.HasSPF {
		penalty := weightNoSPF
		score -= penalty
		breakdown.NoSPF = penalty
	}
	if !email.HasDMARC {
		penalty := weightNoDMARC
		score -= penalty
		breakdown.NoDMARC = penalty
	}

	// Spamhaus
	if spam.Score > 30 {
		penalty := weightSpamhausHigh
		score -= penalty
		breakdown.SpamhausHigh = penalty
	}

	// Website checks
	if !website.Exists {
		penalty := weightWebsiteNotExists
		score -= penalty
		breakdown.WebsiteNotExists = penalty
	}
	// Traffic score (1-10) - lower traffic = lower score
	if website.TrafficScore < 5 {
		penalty := weightTrafficScoreLow
		score -= penalty
		breakdown.TrafficScoreLow = penalty
	} else if website.TrafficScore < 7 {
		penalty := weightTrafficScoreMed
		score -= penalty
		breakdown.TrafficScoreMedium = penalty
	}
	// Trust score (1-10) - lower trust = lower score
	if website.TrustScore < 5 {
		penalty := weightTrustScoreLow
		score -= penalty
		breakdown.TrustScoreLow = penalty
	} else if website.TrustScore < 7 {
		penalty := weightTrustScoreMed
		score -= penalty
		breakdown.TrustScoreMedium = penalty
	}

	// Opt-in compliance is MANDATORY
	if !optIn.Compliance {
		penalty := weightOptInNonCompliant
		score -= penalty
		breakdown.OptInNonCompliant = penalty
	}
	// CAPTCHA is a security enhancement
	if !optIn.HasCaptcha {
		penalty := weightNoCaptcha
		score -= penalty
		breakdown.NoCaptcha = penalty
	}

	// Calculate total penalties
	breakdown.TotalPenalties = 100 - score

	// Ensure score stays within 0-100 range (prevent negative scores)
	if score < 0 {
		score = 0
		breakdown.TotalPenalties = 100
	}
	if score > 100 {
		score = 100
		breakdown.TotalPenalties = 0
	}

	breakdown.FinalScore = score

	// LEVEL (using configurable thresholds)
	level := "good"
	if score <= thresholds.HighRiskMax {
		level = "high-risk"
	} else if score <= thresholds.MediumMax {
		level = "medium"
	}

	// If opt-in compliance fails, always mark as high-risk (mandatory)
	if !optIn.Compliance {
		level = "high-risk"
	}

	// Build reason with details
	reason := buildReason(score, level, breakdown)

	return RiskSummary{
		Score:  score,
		Level:  level,
		Reason: reason,
		// Features:  features,
		// Weights:   *weights,
		Breakdown: breakdown,
	}
}

// buildReason creates a detailed reason string
func buildReason(score int, level string, breakdown PenaltyBreakdown) string {
	reasons := []string{}

	if breakdown.HTTPSMissing > 0 {
		reasons = append(reasons, "missing HTTPS")
	}
	if breakdown.DomainTooNew > 0 {
		reasons = append(reasons, "new domain")
	}
	if breakdown.BlacklistCount > 0 {
		reasons = append(reasons, fmt.Sprintf("%d blacklist hits", breakdown.BlacklistCount))
	}
	if breakdown.OptInNonCompliant > 0 {
		reasons = append(reasons, "opt-in non-compliant")
	}
	if breakdown.GoogleFlagged > 0 {
		reasons = append(reasons, "flagged by Google Safe Browsing")
	}
	if breakdown.NoSPF > 0 {
		reasons = append(reasons, "no SPF record")
	}
	if breakdown.NoDMARC > 0 {
		reasons = append(reasons, "no DMARC record")
	}
	if breakdown.NoMXRecord > 0 {
		reasons = append(reasons, "no MX record")
	}

	if len(reasons) == 0 {
		return "All checks passed"
	}

	return fmt.Sprintf("Score: %d, Level: %s. Issues: %s", score, level, strings.Join(reasons, ", "))
}

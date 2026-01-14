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

// CalculateScoreV2 - New scoring with blacklist analysis and rejection support
func CalculateScoreV2(
	httpsOK bool,
	tlsDays int,
	whoisDays int,
	blacklistAnalysis BlacklistAnalysis,
	mxRepOk bool,
	googleFlagged bool,
	email EmailSecurity,
	ssl SSLQuality,
	optIn OptInCheck,
	website WebsiteCheck,
	isRejected bool,
) RiskSummary {
	thresholds := DefaultScoringThresholds()

	// If rejected, return score 0
	if isRejected {
		rejectReasons := []string{}
		if blacklistAnalysis.IsRejected {
			rejectReasons = append(rejectReasons, blacklistAnalysis.RejectReason)
		}
		if !mxRepOk {
			rejectReasons = append(rejectReasons, "MX reputation too low")
		}
		if !website.Exists {
			rejectReasons = append(rejectReasons, "Website does not exist (CRITICAL)")
		}
		if !httpsOK {
			rejectReasons = append(rejectReasons, "HTTPS not enabled (CRITICAL)")
		}
		if googleFlagged {
			rejectReasons = append(rejectReasons, "Google Safe Browsing flagged (CRITICAL)")
		}
		// Opt-in compliance is default true for now (will be discussed with client later)
		reason := "REJECTED: " + strings.Join(rejectReasons, "; ")
		return RiskSummary{
			Score:  0,
			Level:  "rejected",
			Reason: reason,
			Breakdown: PenaltyBreakdown{
				StartingScore:  100,
				TotalPenalties: 100,
				FinalScore:     0,
			},
		}
	}

	// Initialize penalty breakdown
	breakdown := PenaltyBreakdown{
		StartingScore:  100,
		BlacklistCount: len(blacklistAnalysis.PenaltyDetails),
	}

	score := 100

	// Hard-coded weights (default values)
	// NOTE: HTTPS and Website existence are now CRITICAL (auto-reject)
	// so they don't have penalties here - they cause rejection before scoring
	// NOTE: Spamhaus removed - we check it via DNS blacklist instead
	// NOTE: SPF removed from scoring per user request
	const (
		weightDomainTooNew = 20
		// Google Safe Browsing is now CRITICAL (reject), not penalty
		weightNoMXRecord      = 60 // Increased: MX is important for reply-to
		weightNoDMARC         = 20 // CRITICAL but warning-based (not reject)
		weightDMARCPolicyNone = 10 // p=none is weak policy
		// Traffic score removed - no real API integrated yet
		// Opt-in compliance is now CRITICAL (reject), not penalty
		weightNoCaptcha = 50 // IMPORTANT: no captcha = exposed to bots
	)

	// Domain age
	if whoisDays < 60 {
		penalty := weightDomainTooNew
		score -= penalty
		breakdown.DomainTooNew = penalty
	}

	// BLACKLIST PENALTIES (using analysis instead of simple count)
	if blacklistAnalysis.TotalPenalty > 0 {
		score -= blacklistAnalysis.TotalPenalty
		breakdown.BlacklistPenalty = blacklistAnalysis.TotalPenalty
	}

	// Google Safe Browsing is now CRITICAL (reject), handled in handlers.go

	// Email Security (SPF removed per user request)
	if !email.HasValidMX {
		penalty := weightNoMXRecord
		score -= penalty
		breakdown.NoMXRecord = penalty
	}
	if !email.HasDMARC {
		penalty := weightNoDMARC
		score -= penalty
		breakdown.NoDMARC = penalty
	} else {
		// Check DMARC policy - p=none is weak and gets -10 penalty
		// p=quarantine and p=reject are good, no penalty
		if strings.Contains(strings.ToLower(email.DMARCRecord), "p=none") {
			penalty := weightDMARCPolicyNone
			score -= penalty
			breakdown.DMARCPolicyNone = penalty
		}
	}

	// Spamhaus removed - we check it via DNS blacklist (zen.spamhaus.org) instead

	// Website checks - Exists and HTTPS are CRITICAL (rejection, not penalty)
	// Traffic score removed - no real API integrated yet

	// Opt-in compliance is now CRITICAL (rejection), handled in handlers.go

	// CAPTCHA is IMPORTANT - no captcha = exposed to bots (-50)
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
	reason := buildReasonV2(score, level, breakdown, blacklistAnalysis)

	return RiskSummary{
		Score:     score,
		Level:     level,
		Reason:    reason,
		Breakdown: breakdown,
	}
}

// CalculateScore - Legacy function (kept for backward compatibility)
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

// buildReasonV2 creates a detailed reason string with blacklist analysis
func buildReasonV2(score int, level string, breakdown PenaltyBreakdown, blAnalysis BlacklistAnalysis) string {
	reasons := []string{}

	if breakdown.HTTPSMissing > 0 {
		reasons = append(reasons, "missing HTTPS (-20)")
	}
	if breakdown.DomainTooNew > 0 {
		reasons = append(reasons, "new domain (-20)")
	}
	if breakdown.BlacklistPenalty > 0 {
		details := strings.Join(blAnalysis.PenaltyDetails, ", ")
		if details != "" {
			reasons = append(reasons, fmt.Sprintf("blacklist penalties: %s", details))
		} else {
			reasons = append(reasons, fmt.Sprintf("blacklist penalty (-%d)", breakdown.BlacklistPenalty))
		}
	}
	if breakdown.OptInNonCompliant > 0 {
		reasons = append(reasons, "opt-in non-compliant (-25)")
	}
	if breakdown.GoogleFlagged > 0 {
		reasons = append(reasons, "flagged by Google Safe Browsing (-30)")
	}
	if breakdown.NoSPF > 0 {
		reasons = append(reasons, "no SPF record (-10)")
	}
	if breakdown.NoDMARC > 0 {
		reasons = append(reasons, "no DMARC record (-10)")
	}
	if breakdown.NoMXRecord > 0 {
		reasons = append(reasons, "no MX record (-10)")
	}
	if breakdown.TLSExpiringSoon > 0 {
		reasons = append(reasons, "TLS expiring soon (-10)")
	}
	if breakdown.SpamhausHigh > 0 {
		reasons = append(reasons, "high Spamhaus score (-20)")
	}
	if breakdown.WebsiteNotExists > 0 {
		reasons = append(reasons, "website not accessible (-15)")
	}
	if breakdown.TrafficScoreLow > 0 {
		reasons = append(reasons, "low traffic score (-10)")
	}
	if breakdown.TrustScoreLow > 0 {
		reasons = append(reasons, "low trust score (-15)")
	}
	if breakdown.NoCaptcha > 0 {
		reasons = append(reasons, "no CAPTCHA (-5)")
	}

	if len(reasons) == 0 {
		return "All checks passed"
	}

	return fmt.Sprintf("Score: %d, Level: %s. Issues: %s", score, level, strings.Join(reasons, ", "))
}

// buildReason creates a detailed reason string (legacy)
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

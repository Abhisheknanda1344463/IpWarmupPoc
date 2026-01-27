package vetting

import (
	"fmt"
	"log"
	"strings"
)

// Critical blacklists - if found on ANY of these, domain is REJECTED
// No warmup plan will be generated
var CriticalBlacklists = []string{
	"spamhaus",    // zen.spamhaus.org
	"ivmurl",      // ivmuri.invaluement.com
	"invaluement", // ivmuri.invaluement.com (alternate name)
	"surbl",       // multi.surbl.org
	"abusix",      // Abusix
	"abuse.ch",    // combined.abuse.ch
	"abuseat",     // dnsbl.abuseat.org (Abusix CBL)
}

// Blacklist penalties - specific penalties for non-critical blacklists
var BlacklistPenalties = map[string]int{
	"spamcop":          10, // bl.spamcop.net
	"vadesecure":       30, // Vadesecure (if available via MXToolbox)
	"barracuda":        10, // b.barracudacentral.org
	"barracudacentral": 10, // b.barracudacentral.org (alternate match)
}

// UCEProtect level-specific penalties
var UCEProtectLevelPenalties = map[int]int{
	1: 5,
	2: 10,
	3: 20,
}

// BlacklistResult holds the analysis of blacklist entries
type BlacklistAnalysis struct {
	IsRejected     bool     `json:"is_rejected"`
	RejectReason   string   `json:"reject_reason,omitempty"`
	CriticalHits   []string `json:"critical_hits,omitempty"`
	TotalPenalty   int      `json:"total_penalty"`
	PenaltyDetails []string `json:"penalty_details,omitempty"`
}

// AnalyzeBlacklists checks all blacklist entries and returns analysis
func AnalyzeBlacklists(entries []BlacklistEntry) BlacklistAnalysis {
	result := BlacklistAnalysis{
		IsRejected:     false,
		TotalPenalty:   0,
		CriticalHits:   []string{},
		PenaltyDetails: []string{},
	}

	for _, entry := range entries {
		if !entry.Listed {
			continue
		}

		sourceLower := strings.ToLower(entry.Source)
		log.Printf("[BlacklistAnalysis] Processing blacklist entry: %s (lowercase: %s)", entry.Source, sourceLower)

		// Check for critical blacklists
		if isCriticalBlacklist(sourceLower) {
			log.Printf("[BlacklistAnalysis] ⚠️ CRITICAL blacklist detected: %s", entry.Source)
			result.IsRejected = true
			result.CriticalHits = append(result.CriticalHits, entry.Source)
			continue
		}

		// Check for UCEProtect levels
		if strings.Contains(sourceLower, "uceprotect") {
			level := getUCEProtectLevel(sourceLower)
			penalty := UCEProtectLevelPenalties[level]
			log.Printf("[BlacklistAnalysis] UCEProtect Level %d detected: %s (penalty: -%d)", level, entry.Source, penalty)
			result.TotalPenalty += penalty
			result.PenaltyDetails = append(result.PenaltyDetails,
				fmt.Sprintf("%s (Level %d: -%d)", entry.Source, level, penalty))
			continue
		}

		// Check for other known blacklists
		penalty := getBlacklistPenalty(sourceLower)
		if penalty == 10 && !strings.Contains(sourceLower, "uceprotect") {
			log.Printf("[BlacklistAnalysis] ⚠️ Unknown blacklist (default penalty -10): %s", entry.Source)
		} else {
			log.Printf("[BlacklistAnalysis] Known blacklist: %s (penalty: -%d)", entry.Source, penalty)
		}
		result.TotalPenalty += penalty
		result.PenaltyDetails = append(result.PenaltyDetails,
			fmt.Sprintf("%s (-%d)", entry.Source, penalty))
	}

	// Set reject reason if critical hits found
	if result.IsRejected && len(result.CriticalHits) > 0 {
		result.RejectReason = "Domain is blacklisted on critical list(s): " + strings.Join(result.CriticalHits, ", ")
	}

	return result
}

// isCriticalBlacklist checks if the source matches any critical blacklist
func isCriticalBlacklist(source string) bool {
	for _, critical := range CriticalBlacklists {
		if strings.Contains(source, critical) {
			return true
		}
	}
	return false
}

// getUCEProtectLevel extracts the level from UCEProtect blacklist name
func getUCEProtectLevel(source string) int {
	if strings.Contains(source, "dnsbl-3") || strings.Contains(source, "level3") || strings.Contains(source, "l3") {
		return 3
	}
	if strings.Contains(source, "dnsbl-2") || strings.Contains(source, "level2") || strings.Contains(source, "l2") {
		return 2
	}
	// Default to Level 1
	return 1
}

// getBlacklistPenalty returns penalty for a known blacklist
func getBlacklistPenalty(source string) int {
	for name, penalty := range BlacklistPenalties {
		if strings.Contains(source, name) {
			return penalty
		}
	}
	// Default penalty for unknown blacklists
	return 10
}

// CheckMXReputationAllowed returns true if MX reputation allows proceeding
// If MX reputation is too low, domain should be rejected
// Note: mxRep = 0 means API didn't return data, so we allow proceeding
func CheckMXReputationAllowed(mxRep int) bool {
	// If MX reputation is 0, API didn't return data - allow proceeding
	if mxRep == 0 {
		return true
	}
	// If MX reputation is below 40, reject the domain
	return mxRep >= 40
}

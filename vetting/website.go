package vetting

import (
	"net/http"
	"strings"
	"time"
)

// WebsiteCheck represents website-related checks
type WebsiteCheck struct {
	Exists      bool `json:"exists"`       // Binary: website exists and is accessible
	HTTPSOk     bool `json:"https_ok"`     // Binary: HTTPS available
	TrafficScore int `json:"traffic_score"` // 1-10 score
	TrustScore   int `json:"trust_score"`   // 1-10 score
}

// CheckWebsiteExistence verifies if the website is accessible
func CheckWebsiteExistence(domain string) bool {
	// Try HTTPS first
	client := &http.Client{Timeout: 5 * time.Second}
	
	urls := []string{
		"https://" + domain,
		"http://" + domain,
		"https://www." + domain,
		"http://www." + domain,
	}
	
	for _, url := range urls {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			// Consider it exists if we get any 2xx or 3xx response
			if resp.StatusCode >= 200 && resp.StatusCode < 400 {
				return true
			}
		}
	}
	
	return false
}

// CalculateTrafficScore estimates traffic score (1-10) based on available signals
// This is a simplified version - in production, you'd use APIs like SimilarWeb, Alexa, etc.
func CalculateTrafficScore(domain string, whoisDays int, hasHTTPS bool, ssl SSLQuality) int {
	score := 1 // Start with minimum
	
	// Domain age contributes to traffic likelihood
	if whoisDays > 365 {
		score += 2
	} else if whoisDays > 180 {
		score += 1
	}
	
	// HTTPS presence suggests active site
	if hasHTTPS {
		score += 2
	}
	
	// SSL quality indicates professional setup
	if ssl.Score >= 80 {
		score += 2
	} else if ssl.Score >= 60 {
		score += 1
	}
	
	// Website exists and is accessible
	if CheckWebsiteExistence(domain) {
		score += 3
	}
	
	// Cap at 10
	if score > 10 {
		score = 10
	}
	
	return score
}

// CalculateTrustScore calculates trust score (1-10) based on security and reputation signals
func CalculateTrustScore(
	hasHTTPS bool,
	whoisDays int,
	blacklistCount int,
	mxRep int,
	googleFlagged bool,
	emailSec EmailSecurity,
	ssl SSLQuality,
) int {
	score := 1 // Start with minimum
	
	// HTTPS is fundamental
	if hasHTTPS {
		score += 2
	}
	
	// Domain age builds trust
	if whoisDays > 365 {
		score += 2
	} else if whoisDays > 180 {
		score += 1
	} else if whoisDays < 60 {
		score -= 1 // New domains are less trusted
	}
	
	// Clean blacklist status
	if blacklistCount == 0 {
		score += 2
	} else if blacklistCount == 1 {
		score += 1
	} else {
		score -= blacklistCount // Penalize multiple listings
	}
	
	// Good sender reputation
	if mxRep >= 80 {
		score += 2
	} else if mxRep >= 60 {
		score += 1
	} else if mxRep < 40 {
		score -= 1
	}
	
	// Not flagged by Google
	if !googleFlagged {
		score += 1
	} else {
		score -= 2
	}
	
	// Email security setup
	if emailSec.HasSPF && emailSec.HasDMARC {
		score += 1
	}
	
	// SSL quality
	if ssl.Score >= 80 {
		score += 1
	}
	
	// Ensure score stays within 1-10 range
	if score < 1 {
		score = 1
	}
	if score > 10 {
		score = 10
	}
	
	return score
}

// CheckWebsite performs all website-related checks
func CheckWebsite(
	domain string,
	whoisDays int,
	hasHTTPS bool,
	ssl SSLQuality,
	blacklistCount int,
	mxRep int,
	googleFlagged bool,
	emailSec EmailSecurity,
) WebsiteCheck {
	exists := CheckWebsiteExistence(domain)
	
	// If website doesn't exist, use HTTPS check as fallback
	if !exists {
		exists = hasHTTPS
	}
	
	trafficScore := CalculateTrafficScore(domain, whoisDays, hasHTTPS, ssl)
	trustScore := CalculateTrustScore(hasHTTPS, whoisDays, blacklistCount, mxRep, googleFlagged, emailSec, ssl)
	
	return WebsiteCheck{
		Exists:      exists,
		HTTPSOk:     hasHTTPS,
		TrafficScore: trafficScore,
		TrustScore:   trustScore,
	}
}

// NormalizeDomain ensures domain is in correct format
func NormalizeDomain(domain string) string {
	domain = strings.TrimSpace(domain)
	domain = strings.ToLower(domain)
	
	// Remove protocol if present
	domain = strings.TrimPrefix(domain, "http://")
	domain = strings.TrimPrefix(domain, "https://")
	domain = strings.TrimPrefix(domain, "www.")
	
	// Remove trailing slash
	domain = strings.TrimSuffix(domain, "/")
	
	return domain
}


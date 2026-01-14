package vetting

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
)

// joinReasons combines multiple rejection reasons into a single string
func joinReasons(reasons []string) string {
	return strings.Join(reasons, "; ")
}

// isSubdomain checks if the domain is a subdomain
// Returns: isSubdomain, parentDomain
func isSubdomain(domain string) (bool, string) {
	parts := strings.Split(domain, ".")

	// Need at least 3 parts to be a subdomain (e.g., sub.example.com)
	if len(parts) < 3 {
		return false, domain
	}

	// Handle multi-level TLDs like .co.uk, .co.in, .com.au
	knownTwoLevelTLDs := []string{
		"co.uk", "co.in", "com.au", "com.br", "co.nz", "co.za",
		"org.uk", "net.au", "org.au", "ac.uk", "gov.uk",
	}

	lastTwo := strings.Join(parts[len(parts)-2:], ".")

	for _, tld := range knownTwoLevelTLDs {
		if lastTwo == tld {
			// For domains like sub.example.co.uk, need 4 parts to be subdomain
			if len(parts) < 4 {
				return false, domain
			}
			// Parent is last 3 parts: example.co.uk
			parent := strings.Join(parts[len(parts)-3:], ".")
			return true, parent
		}
	}

	// Normal TLD - parent is last 2 parts
	parent := strings.Join(parts[len(parts)-2:], ".")
	return true, parent
}

// getDMARCWarning returns warning message based on DMARC status and record
func getDMARCWarning(hasDMARC bool, dmarcRecord string) string {
	if !hasDMARC {
		return "CRITICAL: DMARC record is missing. Email authentication will fail. Please add a DMARC record."
	}

	// Check DMARC policy
	dmarcLower := strings.ToLower(dmarcRecord)

	// Check for p=none (weak policy)
	if strings.Contains(dmarcLower, "p=none") {
		return "WARNING: DMARC policy is set to 'none'. Consider upgrading to 'quarantine' or 'reject' for better protection."
	}

	// Check for missing rua (reporting)
	if !strings.Contains(dmarcLower, "rua=") {
		return "WARNING: DMARC record is missing 'rua' (aggregate reporting). Consider adding for monitoring."
	}

	return "" // No warning - DMARC is properly configured
}

type VetRequest struct {
	Domain       string             `json:"domain"`
	SelfAttested *SelfAttestedOptIn `json:"self_attested,omitempty"`
}

type VetResponse struct {
	Domain       string `json:"domain"`
	ParentDomain string `json:"parent_domain,omitempty"` // Only if subdomain
	IsSubdomain  bool   `json:"is_subdomain"`
	IPAddress    string `json:"ip_address"`
	CreatedOn    string `json:"created_on"` // Domain creation date (for reference)

	// Rejection status - if true, no warmup plan should be generated
	IsRejected   bool   `json:"is_rejected"`
	RejectReason string `json:"reject_reason,omitempty"`

	BlacklistHits     []BlacklistEntry  `json:"blacklist_hits"`
	BlacklistAnalysis BlacklistAnalysis `json:"blacklist_analysis"`
	MxReputationOk    bool              `json:"mx_reputation_ok"` // Boolean: true = allowed to proceed

	GoogleSafeBrowsing       bool   `json:"google_safe_browsing"`
	GoogleSafeBrowsingReason string `json:"google_safe_browsing_reason"`

	EmailSecurity EmailSecuritySimple `json:"email_security"`

	// Website checks - CRITICAL fields (checked on parent domain for subdomains)
	Website WebsiteCheckSimple `json:"website"`

	// Opt-in checks
	OptIn OptInCheck `json:"optin"`

	Summary   RiskSummary `json:"summary"`
	Timestamp string      `json:"timestamp"`
}

// WebsiteCheckSimple - simplified website check
type WebsiteCheckSimple struct {
	Exists  bool `json:"exists"`   // CRITICAL: must be true
	HTTPSOk bool `json:"https_ok"` // CRITICAL: must be true
}

// EmailSecuritySimple - simplified email security (kept essential fields)
type EmailSecuritySimple struct {
	HasValidMX   bool   `json:"has_valid_mx"`            // false = -60 penalty
	HasDMARC     bool   `json:"has_dmarc"`               // CRITICAL: must be true
	DMARCRecord  string `json:"dmarc_record,omitempty"`  // The actual DMARC record
	DMARCWarning string `json:"dmarc_warning,omitempty"` // Warning if DMARC has issues
}

func VetHandler(w http.ResponseWriter, r *http.Request) {

	// Parse request
	var req VetRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	domain := req.Domain
	if domain == "" {
		http.Error(w, "domain required", http.StatusBadRequest)
		return
	}

	// Normalize domain
	domain = NormalizeDomain(domain)

	// DETECT SUBDOMAIN
	isSubdom, parentDomain := isSubdomain(domain)

	// Determine which domain to use for website checks
	websiteCheckDomain := domain
	if isSubdom {
		websiteCheckDomain = parentDomain
		log.Printf("📍 Subdomain detected: %s → Parent: %s", domain, parentDomain)
	}

	// BASIC CHECKS
	ip := LookupIP(domain)

	// HTTPS/Website checks on PARENT domain for subdomains
	httpsOK, _ := ProbeHTTPS(websiteCheckDomain)
	ssl := CheckSSLQuality(websiteCheckDomain)

	// Email security on EXACT domain entered (subdomain needs its own MX/SPF/DMARC)
	emailSec := GetEmailSecurity(domain)

	whoisDays, createdOn, _ := WhoisAgeDays(websiteCheckDomain) // Use parent for WHOIS
	tlsDays, _ := GetExpirationDate(websiteCheckDomain)

	// Google Safe Browsing - check BOTH domains
	googleFlagged, googleReason := CheckGoogleReputation(domain)
	if !googleFlagged && isSubdom {
		// Also check parent domain
		parentFlagged, parentReason := CheckGoogleReputation(parentDomain)
		if parentFlagged {
			googleFlagged = true
			googleReason = "Parent domain flagged: " + parentReason
		}
	}

	// --- PARALLEL OPERATIONS ---
	var mxRes *MXBlacklistResult
	var abuse []BlacklistEntry
	var parentAbuse []BlacklistEntry

	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()

	g, _ := errgroup.WithContext(ctx)

	// MXToolbox - check exact domain
	g.Go(func() error {
		res, err := FetchMXToolboxBlacklist(domain)
		if err == nil && res != nil {
			mxRes = res
		}
		return nil
	})

	// RBL/Abuse - check exact domain
	g.Go(func() error {
		abuse = FetchAdditionalAbuseFeeds(domain)
		return nil
	})

	// If subdomain, also check parent domain for blacklists
	if isSubdom {
		g.Go(func() error {
			parentAbuse = FetchAdditionalAbuseFeeds(parentDomain)
			return nil
		})
	}

	// Wait
	_ = g.Wait()

	// MERGE BLACKLIST RESULTS (both subdomain and parent)
	var blacklistCombined []BlacklistEntry

	if mxRes != nil {
		blacklistCombined = append(blacklistCombined, convertMXToBlacklist(mxRes)...)
	}

	blacklistCombined = append(blacklistCombined, abuse...)

	// Add parent domain blacklist hits (if subdomain)
	if isSubdom && len(parentAbuse) > 0 {
		for i := range parentAbuse {
			parentAbuse[i].Info = "Parent domain: " + parentDomain
		}
		blacklistCombined = append(blacklistCombined, parentAbuse...)
	}

	// ANALYZE BLACKLISTS (Critical vs Penalty-based)
	blacklistAnalysis := AnalyzeBlacklists(blacklistCombined)

	// Get MX reputation and check if allowed
	mxRep := 0
	if mxRes != nil {
		mxRep = mxRes.MxRep
	}
	mxRepOk := CheckMXReputationAllowed(mxRep)

	// WEBSITE CHECKS on parent domain for subdomains
	website := CheckWebsite(
		websiteCheckDomain, // Use parent for website checks
		whoisDays,
		httpsOK,
		ssl,
		len(blacklistCombined),
		mxRep,
		googleFlagged,
		emailSec,
	)

	// DETERMINE REJECTION STATUS
	isRejected := false
	rejectReason := ""
	rejectReasons := []string{}

	// Check 1: Critical blacklist = REJECT
	if blacklistAnalysis.IsRejected {
		isRejected = true
		rejectReasons = append(rejectReasons, blacklistAnalysis.RejectReason)
	}

	// Check 2: MX Reputation too low = REJECT
	if !mxRepOk {
		isRejected = true
		rejectReasons = append(rejectReasons, "MX reputation too low (below 40)")
	}

	// Check 3: CRITICAL - Website must exist
	if !website.Exists {
		isRejected = true
		rejectReasons = append(rejectReasons, "Website does not exist or is not accessible")
	}

	// Check 4: CRITICAL - HTTPS must be enabled
	if !httpsOK {
		isRejected = true
		rejectReasons = append(rejectReasons, "HTTPS is not enabled on the domain")
	}

	// Check 5: CRITICAL - Google Safe Browsing must be clean
	if googleFlagged {
		isRejected = true
		rejectReasons = append(rejectReasons, "Google Safe Browsing flagged this domain as unsafe: "+googleReason)
	}

	// OPT-IN CHECKS - Real-time CAPTCHA detection (on parent domain for subdomains)
	optIn := EvaluateOptIn(req.SelfAttested, websiteCheckDomain)

	// Compliance is default true for now (will be discussed with client later)
	// CAPTCHA check is done via scoring penalty (-50 if not found)

	// Combine reject reasons
	if len(rejectReasons) > 0 {
		rejectReason = "REJECTED: " + joinReasons(rejectReasons)
	}

	// CALCULATE SCORE (using blacklist penalties instead of simple count)
	score := CalculateScoreV2(
		httpsOK,
		tlsDays,
		whoisDays,
		blacklistAnalysis, // Pass analysis instead of count
		mxRepOk,
		googleFlagged,
		emailSec,
		ssl,
		optIn,
		website,
		isRejected,
	)

	// Build response
	parentDomainResp := ""
	if isSubdom {
		parentDomainResp = parentDomain
	}

	resp := VetResponse{
		Domain:       domain,
		ParentDomain: parentDomainResp,
		IsSubdomain:  isSubdom,
		IPAddress:    ip,
		CreatedOn:    createdOn,

		IsRejected:   isRejected,
		RejectReason: rejectReason,

		BlacklistHits:     blacklistCombined,
		BlacklistAnalysis: blacklistAnalysis,
		MxReputationOk:    mxRepOk,

		GoogleSafeBrowsing:       googleFlagged,
		GoogleSafeBrowsingReason: googleReason,

		EmailSecurity: EmailSecuritySimple{
			HasValidMX:   emailSec.HasValidMX,
			HasDMARC:     emailSec.HasDMARC,
			DMARCRecord:  emailSec.DMARCRecord,
			DMARCWarning: getDMARCWarning(emailSec.HasDMARC, emailSec.DMARCRecord),
		},

		Website: WebsiteCheckSimple{
			Exists:  website.Exists,
			HTTPSOk: website.HTTPSOk,
		},
		OptIn: optIn,

		Summary:   score,
		Timestamp: time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)

	log.Println("✔ Vetting completed for:", domain)
}
func IndexHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "index.html")
}

package vetting

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"
)

type VetRequest struct {
	Domain       string             `json:"domain"`
	SelfAttested *SelfAttestedOptIn `json:"self_attested,omitempty"`
}

type VetResponse struct {
	Domain           string `json:"domain"`
	IPAddress        string `json:"ip_address"`
	HTTPSOk          bool   `json:"https_ok"`
	TLSDaysLeft      int    `json:"tls_days_left"`
	TLSExpiryDate    string `json:"tls_expiry_date"`
	DomainExpiryDate string `json:"domain_expiry_date"`
	WHOISAgeDays     int    `json:"whois_age_days"`
	CreatedOn        string `json:"created_on"`
	UpdatedOn        string `json:"updated_on"`

	BlacklistHits []BlacklistEntry `json:"blacklist_hits"`
	MxReputation  int              `json:"mx_reputation"` // Sender Score (1-100) from checklist

	GoogleSafeBrowsing       bool   `json:"google_safe_browsing"`
	GoogleSafeBrowsingReason string `json:"google_safe_browsing_reason"`

	EmailSecurity EmailSecurity    `json:"email_security"`
	Geo           GeoInfo          `json:"geo"`
	SSLQuality    SSLQuality       `json:"ssl_quality"`
	Spamhaus      SpamhausResponse `json:"spamhaus"`

	// Website checks (from checklist)
	Website WebsiteCheck `json:"website"`

	// Opt-in checks (from checklist)
	OptIn OptInCheck `json:"optin"`

	Summary   RiskSummary `json:"summary"`
	Timestamp string      `json:"timestamp"`
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

	// BASIC CHECKS
	ip := LookupIP(domain)
	httpsOK, _ := ProbeHTTPS(domain)
	geo := LookupGeo(ip)
	ssl := CheckSSLQuality(domain)
	emailSec := GetEmailSecurity(domain)

	whoisDays, createdOn, updatedOn := WhoisAgeDays(domain)
	tlsDays, tlsExpiry := GetExpirationDate(domain)
	domainExp := DomainExpiryDate(domain)

	googleFlagged, googleReason := CheckGoogleReputation(domain)

	// --- PARALLEL OPERATIONS ---
	var mxRes *MXBlacklistResult
	var abuse []BlacklistEntry
	var spam SpamhausResponse

	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()

	g, _ := errgroup.WithContext(ctx)

	// MXToolbox
	g.Go(func() error {
		res, err := FetchMXToolboxBlacklist(domain)
		if err == nil && res != nil {
			mxRes = res
		}
		return nil
	})

	// RBL/Abuse
	g.Go(func() error {
		abuse = FetchAdditionalAbuseFeeds(domain)
		return nil
	})

	// Spamhaus
	g.Go(func() error {
		rep, err := FetchSpamhausReputation(domain)
		if err == nil && rep != nil {
			spam = *rep
		}
		return nil
	})

	// Wait
	_ = g.Wait()

	// MERGE BLACKLIST RESULTS
	var blacklistCombined []BlacklistEntry

	if mxRes != nil {
		blacklistCombined = append(blacklistCombined, convertMXToBlacklist(mxRes)...)
	}

	blacklistCombined = append(blacklistCombined, abuse...)

	// Get MX reputation
	mxRep := 0
	if mxRes != nil {
		mxRep = mxRes.MxRep
	}

	// WEBSITE CHECKS (from checklist)
	website := CheckWebsite(
		domain,
		whoisDays,
		httpsOK,
		ssl,
		len(blacklistCombined),
		mxRep,
		googleFlagged,
		emailSec,
	)

	// OPT-IN CHECKS (from checklist)
	optIn := EvaluateOptIn(req.SelfAttested)

	// CALCULATE SCORE
	score := CalculateScore(
		httpsOK,
		tlsDays,
		whoisDays,
		len(blacklistCombined),
		mxRep,
		googleFlagged,
		emailSec,
		ssl,
		spam,
		optIn,
		website,
	)

	resp := VetResponse{
		Domain:           domain,
		IPAddress:        ip,
		HTTPSOk:          httpsOK,
		TLSDaysLeft:      tlsDays,
		TLSExpiryDate:    tlsExpiry,
		DomainExpiryDate: domainExp,
		WHOISAgeDays:     whoisDays,
		CreatedOn:        createdOn,
		UpdatedOn:        updatedOn,

		BlacklistHits: blacklistCombined,
		MxReputation:  mxRep, // Sender Score (1-100)

		GoogleSafeBrowsing:       googleFlagged,
		GoogleSafeBrowsingReason: googleReason,

		EmailSecurity: emailSec,
		Geo:           geo,
		SSLQuality:    ssl,
		Spamhaus:      spam,

		Website: website,
		OptIn:   optIn,

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

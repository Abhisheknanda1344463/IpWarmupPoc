package vetting

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	whois "github.com/likexian/whois"
	parser "github.com/likexian/whois-parser"
)

//
// BASIC CHECKS
//

func LookupIP(domain string) string {
	host := domain
	if strings.HasPrefix(host, "http://") || strings.HasPrefix(host, "https://") {
		host = strings.SplitN(host, "//", 2)[1]
	}
	host = strings.Split(host, "/")[0]

	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return ""
	}
	for _, ip := range ips {
		if ip.To4() != nil {
			return ip.String()
		}
	}
	return ips[0].String()
}

func ProbeHTTPS(domain string) (bool, int) {
	d := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := tls.DialWithDialer(d, "tcp", domain+":443", &tls.Config{ServerName: domain})
	if err != nil {
		return false, 0
	}
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return false, 0
	}

	days := int(time.Until(certs[0].NotAfter).Hours() / 24)
	return true, days
}

//
// WHOIS LOOKUP
//

func WhoisAgeDays(domain string) (int, string, string) {
	raw, err := whois.Whois(domain)
	if err != nil {
		return 0, "", ""
	}

	p, err := parser.Parse(raw)
	if err != nil || p.Domain == nil {
		// For subdomains, try parent domain (e.g., e.sellwithemail.online -> sellwithemail.online)
		parts := strings.Split(domain, ".")
		if len(parts) > 2 {
			parentDomain := strings.Join(parts[1:], ".")
			return WhoisAgeDays(parentDomain)
		}
		return 0, "", ""
	}

	createdStr := strings.TrimSpace(p.Domain.CreatedDate)
	updatedStr := strings.TrimSpace(p.Domain.UpdatedDate)

	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02",
		"02-Jan-2006",
		"2006.01.02",
	}

	var created, updated time.Time

	for _, l := range layouts {
		t, err := time.Parse(l, createdStr)
		if err == nil {
			created = t
			break
		}
	}

	for _, l := range layouts {
		t, err := time.Parse(l, updatedStr)
		if err == nil {
			updated = t
			break
		}
	}

	if created.IsZero() {
		return 0, "", ""
	}

	ageDays := int(time.Since(created).Hours() / 24)
	return ageDays, created.Format("02/01/2006"), updated.Format("02/01/2006")
}

//
// BLACKLIST FEEDS
//

type BlacklistEntry struct {
	Source string `json:"source"`
	Listed bool   `json:"listed"`
	Info   string `json:"info,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type MXBlacklistResult struct {
	MxRep int              `json:"mx_rep"`
	Lists []BlacklistEntry `json:"lists"`
}

var domainRBLs = []string{
	// CRITICAL - Auto Reject
	"multi.surbl.org",        // SURBL
	"ivmuri.invaluement.com", // ivmURL / Invaluement

	// Other domain-based RBLs
	"uribl.spameatingmonkey.net",
	"uribl.blacklist.woody.ch",
	"ubl.unsubscore.com",
}

func checkDomainRBL(domain string) []BlacklistEntry {
	var results []BlacklistEntry

	log.Printf("[RBL] Checking domain %s against %d domain RBLs", domain, len(domainRBLs))

	for _, rbl := range domainRBLs {
		query := domain + "." + rbl

		// Use custom resolver with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		resolver := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: 2 * time.Second}
				return d.DialContext(ctx, "udp", "8.8.8.8:53")
			},
		}

		addrs, err := resolver.LookupHost(ctx, query)
		cancel()

		if err == nil && len(addrs) > 0 {
			// Verify it's a valid RBL response (should be 127.0.0.x)
			isValidRBLResponse := false
			for _, addr := range addrs {
				if strings.HasPrefix(addr, "127.0.0.") {
					isValidRBLResponse = true
					break
				}
			}

			if isValidRBLResponse {
				log.Printf("[RBL] ⚠️ Domain LISTED on %s: %s (response: %v)", rbl, query, addrs)
				results = append(results, BlacklistEntry{
					Source: rbl,
					Listed: true,
				})
			} else {
				log.Printf("[RBL] Ignoring non-standard response from %s: %v", rbl, addrs)
			}
		}
	}

	return results
}

var ipRBLs = []string{
	// CRITICAL - Auto Reject
	"zen.spamhaus.org",  // Spamhaus (includes SBL, XBL, PBL)
	"combined.abuse.ch", // Abusix alternative (abuse.ch)
	"dnsbl.abuseat.org", // Abusix CBL

	// Penalty-based
	"bl.spamcop.net",         // Spamcop (-10)
	"b.barracudacentral.org", // Barracuda (-10)

	// UCEProtect Levels
	"dnsbl-1.uceprotect.net", // UCEProtect Level 1 (-5)
	"dnsbl-2.uceprotect.net", // UCEProtect Level 2 (-10)
	"dnsbl-3.uceprotect.net", // UCEProtect Level 3 (-20)

	// Other IP-based RBLs
	"bl.mailspike.net",
	"z.mailspike.net",
	// NOTE: hostkarma.junkemailfilter.com is a combined informational list, not a strict blacklist
	// It uses multiple return codes (127.0.0.1=whitelist, 127.0.0.2=blacklist, 127.0.0.3=yellowlist)
	// We only check strict blacklists, so this is excluded to avoid false positives
	// "hostkarma.junkemailfilter.com",
	"psbl.surriel.com",
	"dnsbl.sorbs.net",
}

func reverseIP(ip string) string {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return ""
	}
	return parts[3] + "." + parts[2] + "." + parts[1] + "." + parts[0]
}

func checkIPRBL(domain string) []BlacklistEntry {
	ip := LookupIP(domain)
	if ip == "" {
		log.Printf("[RBL] Could not resolve IP for domain: %s", domain)
		return nil
	}

	rev := reverseIP(ip)
	if rev == "" {
		log.Printf("[RBL] Could not reverse IP: %s for domain: %s", ip, domain)
		return nil
	}

	log.Printf("[RBL] Checking IP %s (reversed: %s) for domain: %s", ip, rev, domain)

	var results []BlacklistEntry

	for _, rbl := range ipRBLs {
		query := rev + "." + rbl

		// Use custom resolver with timeout to avoid cloud DNS issues
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		resolver := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: 2 * time.Second}
				// Use Google's DNS for more reliable results
				return d.DialContext(ctx, "udp", "8.8.8.8:53")
			},
		}

		addrs, err := resolver.LookupHost(ctx, query)
		cancel()

		if err == nil && len(addrs) > 0 {
			// Verify it's a valid RBL response (should be 127.0.0.x)
			// False positives can occur if DNS returns unexpected results
			isValidRBLResponse := false
			for _, addr := range addrs {
				if strings.HasPrefix(addr, "127.0.0.") {
					isValidRBLResponse = true
					break
				}
			}

			if isValidRBLResponse {
				log.Printf("[RBL] ⚠️ LISTED on %s: %s (response: %v)", rbl, query, addrs)
				results = append(results, BlacklistEntry{
					Source: rbl,
					Listed: true,
				})
			} else {
				log.Printf("[RBL] Ignoring non-standard RBL response from %s: %v", rbl, addrs)
			}
		} else if err != nil {
			// Not listed (DNS lookup failed = not on blacklist)
			// This is normal and expected for clean IPs
		}
	}

	return results
}

func FetchAdditionalAbuseFeeds(domain string) []BlacklistEntry {
	var combined []BlacklistEntry
	combined = append(combined, checkDomainRBL(domain)...)
	combined = append(combined, checkIPRBL(domain)...)
	return combined
}

//
// MXTOOLBOX BLACKLIST LOOKUP
//

func FetchMXToolboxBlacklist(domain string) (*MXBlacklistResult, error) {
	apiKey := os.Getenv("MXTOOLBOX_API_KEY")
	url := fmt.Sprintf("https://mxtoolbox.com/api/v1/Lookup?command=blacklist&argument=%s", domain)

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", apiKey)
	req.Header.Set("accept", "application/json")

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var raw struct {
		MxRep  int `json:"MxRep"`
		Failed []struct {
			Name        string `json:"Name"`
			Info        string `json:"Info"`
			Description string `json:"BlacklistReasonDescription"`
		} `json:"Failed"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	var entries []BlacklistEntry
	for _, f := range raw.Failed {
		log.Printf("[MXToolbox] Blacklist found for %s: %s (Info: %s, Reason: %s)", domain, f.Name, f.Info, f.Description)
		entries = append(entries, BlacklistEntry{
			Source: f.Name,
			Listed: true,
			Info:   f.Info,
			Reason: f.Description,
		})
	}

	if len(entries) == 0 {
		log.Printf("[MXToolbox] No blacklists found for %s (MxRep: %d)", domain, raw.MxRep)
	}

	return &MXBlacklistResult{
		MxRep: raw.MxRep,
		Lists: entries,
	}, nil
}

func convertMXToBlacklist(mx *MXBlacklistResult) []BlacklistEntry {
	var list []BlacklistEntry
	for _, f := range mx.Lists {
		list = append(list, BlacklistEntry{
			Source: f.Source,
			Listed: true,
			Info:   f.Info,
			Reason: f.Reason,
		})
	}
	return list
}

//
// TLS + EXPIRY
//

func GetExpirationDate(domain string) (int, string) {
	conn, err := tls.Dial("tcp", domain+":443", &tls.Config{ServerName: domain})
	if err != nil {
		return 0, ""
	}
	defer conn.Close()

	expiry := conn.ConnectionState().PeerCertificates[0].NotAfter
	return int(time.Until(expiry).Hours() / 24), expiry.Format("02/01/2006")
}

func DomainExpiryDate(domain string) string {
	raw, err := whois.Whois(domain)
	if err != nil {
		return ""
	}

	p, err := parser.Parse(raw)
	if err != nil || p.Domain == nil {
		// For subdomains, try parent domain
		parts := strings.Split(domain, ".")
		if len(parts) > 2 {
			parentDomain := strings.Join(parts[1:], ".")
			return DomainExpiryDate(parentDomain)
		}
		return ""
	}
	dateStr := strings.TrimSpace(p.Domain.ExpirationDate)

	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02",
		"02-Jan-2006",
	}

	var t time.Time

	for _, l := range layouts {
		parsed, err := time.Parse(l, dateStr)
		if err == nil {
			t = parsed
			break
		}
	}

	if t.IsZero() {
		return ""
	}

	return t.Format("02/01/2006")
}

//
// GOOGLE SAFE BROWSING
//

func CheckGoogleReputation(domain string) (bool, string) {
	apiKey := os.Getenv("GOOGLE_SAFE_BROWSING_KEY")
	if apiKey == "" {
		return false, "API key missing"
	}

	url := "https://safebrowsing.googleapis.com/v4/threatMatches:find?key=" + apiKey

	body := fmt.Sprintf(`
    {
      "client": {
        "clientId": "vetting-service",
        "clientVersion": "1.0"
      },
      "threatInfo": {
        "threatTypes": ["MALWARE", "SOCIAL_ENGINEERING", "UNWANTED_SOFTWARE"],
        "platformTypes": ["ANY_PLATFORM"],
        "threatEntryTypes": ["URL"],
        "threatEntries": [{"url": "http://%s"}]
      }
    }`, domain)

	req, _ := http.NewRequest("POST", url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 6 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, "API error"
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if result["matches"] != nil {
		return true, "Google flagged this domain"
	}
	return false, "No threats"
}

package vetting

import (
	"context"
	"log"
	"net"
	"strings"
	"time"
)

type EmailSecurity struct {
	HasValidMX  bool   `json:"has_valid_mx"`
	HasSPF      bool   `json:"has_spf"`
	HasDMARC    bool   `json:"has_dmarc"`
	SPFRecord   string `json:"spf_record,omitempty"`
	DMARCRecord string `json:"dmarc_record,omitempty"`
}

// DNS servers to try (in order)
var dnsServers = []string{
	"8.8.8.8:53",        // Google Primary
	"8.8.4.4:53",        // Google Secondary
	"1.1.1.1:53",        // Cloudflare Primary
	"1.0.0.1:53",        // Cloudflare Secondary
	"9.9.9.9:53",        // Quad9
}

// getResolverWithDNS creates a resolver with a specific DNS server
func getResolverWithDNS(dnsServer string) *net.Resolver {
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{
				Timeout: 5 * time.Second,
			}
			return d.DialContext(ctx, "udp", dnsServer)
		},
	}
}

// lookupTXTWithRetry tries multiple DNS servers
func lookupTXTWithRetry(domain string) ([]string, error) {
	var lastErr error

	for _, dns := range dnsServers {
		resolver := getResolverWithDNS(dns)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		txts, err := resolver.LookupTXT(ctx, domain)
		cancel()

		if err == nil && len(txts) > 0 {
			log.Printf("[DNS] TXT lookup for %s succeeded via %s", domain, dns)
			return txts, nil
		}
		lastErr = err
		log.Printf("[DNS] TXT lookup for %s failed via %s: %v", domain, dns, err)
	}

	// Also try system resolver as fallback
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	txts, err := net.DefaultResolver.LookupTXT(ctx, domain)
	if err == nil && len(txts) > 0 {
		log.Printf("[DNS] TXT lookup for %s succeeded via system resolver", domain)
		return txts, nil
	}

	return nil, lastErr
}

// lookupMXWithRetry tries multiple DNS servers for MX records
func lookupMXWithRetry(domain string) ([]*net.MX, error) {
	var lastErr error

	for _, dns := range dnsServers {
		resolver := getResolverWithDNS(dns)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		mxs, err := resolver.LookupMX(ctx, domain)
		cancel()

		if err == nil && len(mxs) > 0 {
			log.Printf("[DNS] MX lookup for %s succeeded via %s", domain, dns)
			return mxs, nil
		}
		lastErr = err
		log.Printf("[DNS] MX lookup for %s failed via %s: %v", domain, dns, err)
	}

	// Also try system resolver as fallback
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mxs, err := net.DefaultResolver.LookupMX(ctx, domain)
	if err == nil && len(mxs) > 0 {
		log.Printf("[DNS] MX lookup for %s succeeded via system resolver", domain)
		return mxs, nil
	}

	return nil, lastErr
}

func GetEmailSecurity(domain string) EmailSecurity {
	sec := EmailSecurity{}

	log.Printf("[EmailSecurity] Starting checks for %s", domain)

	// -------------------------
	// MX CHECK (with retry)
	// -------------------------
	mxRecords, err := lookupMXWithRetry(domain)
	if err != nil {
		log.Printf("[EmailSecurity] MX lookup failed for %s after all retries: %v", domain, err)
	}
	if len(mxRecords) > 0 {
		sec.HasValidMX = true
		log.Printf("[EmailSecurity] ✓ MX found for %s: %d records", domain, len(mxRecords))
	}

	// -------------------------
	// SPF CHECK (TXT record with retry)
	// -------------------------
	txts, err := lookupTXTWithRetry(domain)
	if err != nil {
		log.Printf("[EmailSecurity] TXT lookup failed for %s after all retries: %v", domain, err)
	}
	for _, t := range txts {
		lower := strings.ToLower(t)
		if strings.HasPrefix(lower, "v=spf1") || strings.Contains(lower, "v=spf1") {
			sec.HasSPF = true
			sec.SPFRecord = t
			log.Printf("[EmailSecurity] ✓ SPF found for %s: %s", domain, truncate(t, 50))
			break
		}
	}

	// -------------------------
	// DMARC CHECK (_dmarc.domain with retry)
	// -------------------------
	dmarcDomain := "_dmarc." + domain
	dmarcTXT, err := lookupTXTWithRetry(dmarcDomain)
	if err != nil {
		log.Printf("[EmailSecurity] DMARC lookup failed for %s after all retries: %v", dmarcDomain, err)
	}

	for _, t := range dmarcTXT {
		lower := strings.ToLower(t)
		if strings.HasPrefix(lower, "v=dmarc1") || strings.Contains(lower, "v=dmarc1") {
			sec.HasDMARC = true
			sec.DMARCRecord = t
			log.Printf("[EmailSecurity] ✓ DMARC found for %s: %s", domain, truncate(t, 50))
			break
		}
	}

	log.Printf("[EmailSecurity] Final result for %s: MX=%v, SPF=%v, DMARC=%v",
		domain, sec.HasValidMX, sec.HasSPF, sec.HasDMARC)

	return sec
}

// truncate helper for logging
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

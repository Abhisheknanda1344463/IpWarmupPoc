package vetting

import (
	"net"
	"strings"
)

type EmailSecurity struct {
	HasValidMX  bool   `json:"has_valid_mx"`
	HasSPF      bool   `json:"has_spf"`
	HasDMARC    bool   `json:"has_dmarc"`
	SPFRecord   string `json:"spf_record,omitempty"`
	DMARCRecord string `json:"dmarc_record,omitempty"`
}

func GetEmailSecurity(domain string) EmailSecurity {
	sec := EmailSecurity{}

	// -------------------------
	// MX CHECK
	// -------------------------
	mxRecords, err := net.LookupMX(domain)
	if err == nil && len(mxRecords) > 0 {
		sec.HasValidMX = true
	}

	// -------------------------
	// SPF CHECK (TXT record)
	// -------------------------
	txts, _ := net.LookupTXT(domain)
	for _, t := range txts {
		if strings.HasPrefix(strings.ToLower(t), "v=spf1") {
			sec.HasSPF = true
			sec.SPFRecord = t
		}
	}

	// -------------------------
	// DMARC CHECK (_dmarc.domain)
	// -------------------------
	dmarcDomain := "_dmarc." + domain
	dmarcTXT, _ := net.LookupTXT(dmarcDomain)

	for _, t := range dmarcTXT {
		if strings.HasPrefix(strings.ToLower(t), "v=dmarc1") {
			sec.HasDMARC = true
			sec.DMARCRecord = t
		}
	}

	return sec
}

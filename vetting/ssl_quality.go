package vetting

import (
	"crypto/tls"
	"strings"
	"time"
)

type SSLQuality struct {
	ValidUntil string `json:"valid_until"`
	SelfSigned bool   `json:"self_signed"`
	Protocol   string `json:"protocol"`
	Cipher     string `json:"cipher"`
	Score      int    `json:"score"` // 0-100
}

func CheckSSLQuality(domain string) SSLQuality {
	q := SSLQuality{}

	conn, err := tls.Dial("tcp", domain+":443", &tls.Config{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return q
	}
	defer conn.Close()

	state := conn.ConnectionState()

	// Expiry
	if len(state.PeerCertificates) > 0 {
		cert := state.PeerCertificates[0]
		q.ValidUntil = cert.NotAfter.Format(time.RFC3339)
		q.SelfSigned = cert.IsCA
	}

	// Protocol
	q.Protocol = tlsVersionName(state.Version)

	// Cipher
	q.Cipher = tls.CipherSuiteName(state.CipherSuite)

	// Score
	score := 100
	if q.SelfSigned {
		score -= 40
	}
	if !strings.Contains(q.Protocol, "TLS1.3") {
		score -= 20
	}
	q.Score = score

	return q
}

func tlsVersionName(v uint16) string {
	switch v {
	case tls.VersionTLS13:
		return "TLS1.3"
	case tls.VersionTLS12:
		return "TLS1.2"
	default:
		return "weak"
	}
}

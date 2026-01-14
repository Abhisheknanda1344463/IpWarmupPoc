package vetting

import (
	"io"
	"net/http"
	"strings"
	"time"
)

// OptInCheck represents opt-in compliance and security checks
type OptInCheck struct {
	Compliance     bool   `json:"compliance"`                // Default true for now (will discuss with client)
	HasCaptcha     bool   `json:"has_captcha"`               // Real-time detection
	CaptchaWarning string `json:"captcha_warning,omitempty"` // Warning if no captcha
}

// SelfAttestedOptIn - kept for backward compatibility but not used for now
type SelfAttestedOptIn struct {
	OptInLink  string `json:"optin_link,omitempty"`
	HasCaptcha bool   `json:"has_captcha,omitempty"`
}

// CAPTCHA detection patterns
var captchaPatterns = map[string]string{
	"google.com/recaptcha":      "reCAPTCHA",
	"www.google.com/recaptcha":  "reCAPTCHA",
	"recaptcha.net":             "reCAPTCHA",
	"gstatic.com/recaptcha":     "reCAPTCHA",
	"hcaptcha.com":              "hCaptcha",
	"challenges.cloudflare.com": "Cloudflare Turnstile",
	"turnstile.cloudflare.com":  "Cloudflare Turnstile",
	"funcaptcha.com":            "FunCaptcha",
	"arkoselabs.com":            "Arkose Labs",
	"mtcaptcha.com":             "MTCaptcha",
	"captcha.com":               "Generic CAPTCHA",
	"data-sitekey":              "CAPTCHA (sitekey detected)",
	"g-recaptcha":               "reCAPTCHA",
	"h-captcha":                 "hCaptcha",
	"cf-turnstile":              "Cloudflare Turnstile",
}

// DetectCaptcha crawls the website and detects if CAPTCHA is present
func DetectCaptcha(domain string) (bool, string) {
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	// Try multiple URLs
	urls := []string{
		"https://" + domain,
		"https://www." + domain,
		"https://" + domain + "/signup",
		"https://" + domain + "/register",
		"https://" + domain + "/subscribe",
		"https://" + domain + "/contact",
		"http://" + domain,
	}

	for _, url := range urls {
		hasCaptcha, captchaType := checkURLForCaptcha(client, url)
		if hasCaptcha {
			return true, captchaType
		}
	}

	return false, ""
}

// checkURLForCaptcha checks a single URL for CAPTCHA presence
func checkURLForCaptcha(client *http.Client, url string) (bool, string) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, ""
	}

	// Set user agent to avoid being blocked
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return false, ""
	}
	defer resp.Body.Close()

	// Only check successful responses
	if resp.StatusCode >= 400 {
		return false, ""
	}

	// Read body (limit to 1MB to avoid large files)
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return false, ""
	}

	bodyLower := strings.ToLower(string(body))

	// Check for CAPTCHA patterns
	for pattern, captchaType := range captchaPatterns {
		if strings.Contains(bodyLower, strings.ToLower(pattern)) {
			return true, captchaType
		}
	}

	return false, ""
}

// EvaluateOptIn performs all opt-in related checks
func EvaluateOptIn(selfAttested *SelfAttestedOptIn, domain string) OptInCheck {
	// Compliance is default true for now (will be discussed with client later)
	compliance := true

	// Real-time CAPTCHA detection
	hasCaptcha, _ := DetectCaptcha(domain)

	warning := ""
	if !hasCaptcha {
		warning = "WARNING: No CAPTCHA detected on the website. Domain may be exposed to bots, spam, and automated attacks. Consider implementing reCAPTCHA, hCaptcha, or Cloudflare Turnstile."
	}

	return OptInCheck{
		Compliance:     compliance,
		HasCaptcha:     hasCaptcha,
		CaptchaWarning: warning,
	}
}

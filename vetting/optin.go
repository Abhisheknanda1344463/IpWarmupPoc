package vetting

import (
	"context"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
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

// CAPTCHA detection patterns - comprehensive list
var captchaPatterns = map[string]string{
	// Google reCAPTCHA
	"google.com/recaptcha":     "reCAPTCHA",
	"www.google.com/recaptcha": "reCAPTCHA",
	"recaptcha.net":            "reCAPTCHA",
	"gstatic.com/recaptcha":    "reCAPTCHA",
	"grecaptcha":               "reCAPTCHA",
	"g-recaptcha":              "reCAPTCHA",
	"recaptcha-badge":          "reCAPTCHA v3 (invisible)",
	"recaptcha_invisible":      "reCAPTCHA v3 (invisible)",

	// hCaptcha
	"hcaptcha.com": "hCaptcha",
	"h-captcha":    "hCaptcha",

	// Cloudflare Turnstile
	"challenges.cloudflare.com": "Cloudflare Turnstile",
	"turnstile.cloudflare.com":  "Cloudflare Turnstile",
	"cf-turnstile":              "Cloudflare Turnstile",
	"cf-chl-widget":             "Cloudflare Challenge",

	// Other CAPTCHAs
	"funcaptcha.com":  "FunCaptcha",
	"arkoselabs.com":  "Arkose Labs",
	"mtcaptcha.com":   "MTCaptcha",
	"captcha.com":     "Generic CAPTCHA",
	"data-sitekey":    "CAPTCHA (sitekey detected)",
	"data-captcha":    "CAPTCHA (data attribute)",
	"captcha-widget":  "CAPTCHA Widget",
	"captcha-element": "CAPTCHA Element",
}

// DetectCaptcha detects CAPTCHA using headless Chrome (chromedp)
// Falls back to HTTP method if chromedp fails
func DetectCaptcha(domain string) (bool, string) {
	// Try chromedp first (more accurate, can detect JS-loaded CAPTCHAs)
	hasCaptcha, captchaType := detectCaptchaWithChromedp(domain)
	if hasCaptcha {
		return true, captchaType
	}

	// Fallback to HTTP method
	return detectCaptchaWithHTTP(domain)
}

// detectCaptchaWithChromedp uses headless Chrome to render JS and detect CAPTCHAs
func detectCaptchaWithChromedp(domain string) (bool, string) {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create headless Chrome options
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("disable-background-networking", true),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	defer allocCancel()

	browserCtx, browserCancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
	defer browserCancel()

	// URLs to check
	urls := []string{
		"https://" + domain,
		"https://www." + domain,
		"https://" + domain + "/signup",
		"https://" + domain + "/register",
		"https://" + domain + "/contact",
	}

	for _, url := range urls {
		hasCaptcha, captchaType := checkURLWithChromedp(browserCtx, url)
		if hasCaptcha {
			return true, captchaType
		}
	}

	return false, ""
}

// checkURLWithChromedp loads a URL and checks for CAPTCHA in rendered HTML
func checkURLWithChromedp(ctx context.Context, url string) (bool, string) {
	var htmlContent string

	// Create a timeout context for this specific URL
	urlCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Navigate and get HTML
	err := chromedp.Run(urlCtx,
		chromedp.Navigate(url),
		chromedp.WaitReady("body"),
		chromedp.Sleep(2*time.Second), // Wait for JS to execute
		chromedp.OuterHTML("html", &htmlContent),
	)

	if err != nil {
		// URL might not be accessible, continue to next
		return false, ""
	}

	// Check for CAPTCHA patterns in rendered HTML
	htmlLower := strings.ToLower(htmlContent)
	for pattern, captchaType := range captchaPatterns {
		if strings.Contains(htmlLower, strings.ToLower(pattern)) {
			return true, captchaType
		}
	}

	return false, ""
}

// detectCaptchaWithHTTP is the fallback HTTP-based detection
func detectCaptchaWithHTTP(domain string) (bool, string) {
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

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
		hasCaptcha, captchaType := checkURLForCaptchaHTTP(client, url)
		if hasCaptcha {
			return true, captchaType
		}
	}

	return false, ""
}

// checkURLForCaptchaHTTP checks a single URL for CAPTCHA presence using HTTP
func checkURLForCaptchaHTTP(client *http.Client, url string) (bool, string) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, ""
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return false, ""
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return false, ""
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return false, ""
	}

	bodyLower := strings.ToLower(string(body))

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

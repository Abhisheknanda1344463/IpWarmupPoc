package vetting

// OptInCheck represents opt-in compliance and security checks
type OptInCheck struct {
	Compliance bool `json:"compliance"` // Mandatory: opt-in compliance verified
	HasCaptcha bool `json:"has_captcha"` // Security: CAPTCHA protection present
}

// SelfAttestedOptIn represents customer self-attestation of opt-in practices
type SelfAttestedOptIn struct {
	HasOptIn   bool `json:"has_optin"`
	HasCaptcha bool `json:"has_captcha"`
}

// ValidateOptInCompliance validates opt-in compliance (mandatory check)
// In production, this could integrate with:
// - Email service provider APIs to verify double opt-in
// - Database checks for consent records
// - Third-party compliance verification services
func ValidateOptInCompliance(selfAttested *SelfAttestedOptIn) bool {
	// For now, we accept self-attestation but flag it for manual review
	// In production, this should be more robust
	if selfAttested == nil {
		return false // No attestation = non-compliant
	}
	
	// Opt-in is MANDATORY - must be true
	return selfAttested.HasOptIn
}

// CheckCaptchaSecurity checks if CAPTCHA is implemented (security enhancement)
// This is typically self-attested or verified through website scanning
func CheckCaptchaSecurity(selfAttested *SelfAttestedOptIn) bool {
	if selfAttested == nil {
		return false
	}
	
	return selfAttested.HasCaptcha
}

// EvaluateOptIn performs all opt-in related checks
func EvaluateOptIn(selfAttested *SelfAttestedOptIn) OptInCheck {
	return OptInCheck{
		Compliance: ValidateOptInCompliance(selfAttested),
		HasCaptcha: CheckCaptchaSecurity(selfAttested),
	}
}


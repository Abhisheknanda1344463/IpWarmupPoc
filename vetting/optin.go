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
	// For POC/demo: assume compliant if no attestation provided
	// This allows scoring to work based on technical checks
	// In production, this should require explicit attestation
	if selfAttested == nil {
		return true // POC: assume compliant for demo purposes
	}
	
	// Opt-in is MANDATORY - must be true
	return selfAttested.HasOptIn
}

// CheckCaptchaSecurity checks if CAPTCHA is implemented (security enhancement)
// This is typically self-attested or verified through website scanning
func CheckCaptchaSecurity(selfAttested *SelfAttestedOptIn) bool {
	// For POC/demo: assume has captcha if no attestation provided
	if selfAttested == nil {
		return true // POC: assume has captcha for demo purposes
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


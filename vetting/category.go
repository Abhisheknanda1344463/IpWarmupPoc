package vetting

type CategoryInfo struct {
	Label string `json:"label"`
	Risk  string `json:"risk"`
}

// Stub (future)
func LookupCategory(domain string) CategoryInfo {
	// Free alternatives: Webshrinker trial, VirusTotal free
	// For now, return "unknown"
	return CategoryInfo{
		Label: "unknown",
		Risk:  "unknown",
	}
}

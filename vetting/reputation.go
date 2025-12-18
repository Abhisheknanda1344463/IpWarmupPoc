package vetting

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
)

type SpamhausResponse struct {
	Domain     string   `json:"domain"`
	Score      float64  `json:"score"`
	Abused     bool     `json:"abused"`
	Tags       []string `json:"tags"`
	Dimensions struct {
		Human    float64 `json:"human"`
		Identity float64 `json:"identity"`
		Infra    float64 `json:"infra"`
		Malware  float64 `json:"malware"`
		SMTP     float64 `json:"smtp"`
	} `json:"dimensions"`
}

func FetchSpamhausReputation(domain string) (*SpamhausResponse, error) {
	_ = godotenv.Load()

	apiKey := os.Getenv("SPAMHAUS_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("missing SPAMHAUS_API_KEY")
	}

	url := fmt.Sprintf("https://www.spamhaus.org/api/v1/sia-proxy/api/intel/v2/byobject/domain/%s/overview", domain)

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("spamhaus error: %v", resp.Status)
	}

	var data SpamhausResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	return &data, nil
}

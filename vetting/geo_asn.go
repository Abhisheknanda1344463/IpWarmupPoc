package vetting

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type GeoInfo struct {
	Country string `json:"country"`
	Region  string `json:"region"`
	City    string `json:"city"`
	ISP     string `json:"isp"`
	ASN     int    `json:"asn"`
	ASName  string `json:"as_name"`
}

func LookupGeo(ip string) GeoInfo {
	var info GeoInfo
	if ip == "" {
		return info
	}

	url := fmt.Sprintf("http://ip-api.com/json/%s?fields=status,country,regionName,city,isp,as,asname,query", ip)
	client := http.Client{Timeout: 6 * time.Second}

	resp, err := client.Get(url)
	if err != nil {
		return info
	}
	defer resp.Body.Close()

	var data map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&data)

	info.Country = toStr(data["country"])
	info.Region = toStr(data["regionName"])
	info.City = toStr(data["city"])
	info.ISP = toStr(data["isp"])
	info.ASName = toStr(data["asname"])

	return info
}

func toStr(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

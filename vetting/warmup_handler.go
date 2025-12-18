package vetting

import (
	"encoding/json"
	"net/http"
)

type WarmupRequest struct {
	TargetVolume int `json:"target_volume"`
	// isko tumhari HTML me "days" bhej rahe ho, to alias rakh sakte ho:
	Days int `json:"days"`
}

type WarmupPlansResponse struct {
	Plan30Day         []WarmupDay `json:"plan_30_day"`
	PlanLessThan30    []WarmupDay `json:"plan_less_than_30"`
	PlanGreaterThan30 []WarmupDay `json:"plan_greater_than_30"`
}

func WarmupHandler(w http.ResponseWriter, r *http.Request) {
	var req WarmupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid input", http.StatusBadRequest)
		return
	}

	if req.TargetVolume <= 0 {
		http.Error(w, "target_volume must be > 0", http.StatusBadRequest)
		return
	}
	if req.Days <= 0 {
		req.Days = 30
	}

	plan30, planLt30, planGt30 := GenerateWarmupPlans(req.TargetVolume, req.Days)

	resp := WarmupPlansResponse{
		Plan30Day:         plan30,
		PlanLessThan30:    planLt30,
		PlanGreaterThan30: planGt30,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

package vetting

import "math"

type WarmupDay struct {
	Day   int `json:"day"`
	Limit int `json:"limit"`
}

// Excel-style rounding: 0.5 -> up
func excelRound(v float64) int {
	if v <= 0 {
		return 0
	}
	return int(math.Floor(v + 0.5))
}

// Ye function exactly Excel ki tarah 3 columns generate karega:
// - 30 DAY column
// - <30 column
// - >30 column
//
// targetVolume  => G1 (TARGET VOLUME)
// customPeriod  => G8 (CUSTOM PERIOD)
func GenerateWarmupPlans(targetVolume int, customPeriod int) (plan30, planLt30, planGt30 []WarmupDay) {
	// Hard cap like sheet: humesha 60 din ka grid
	const maxDays = 60
	if customPeriod <= 0 {
		customPeriod = 30
	}
	if customPeriod > maxDays {
		customPeriod = maxDays
	}

	tv := float64(targetVolume)
	cp := float64(customPeriod)
	tp := 30.0 // TARGET PERIOD fixed 30

	// Excel cells:
	medianCustom := tv / cp // G1/G8
	median30 := tv / tp     // G1/G10
	factor := tp / cp       // G10/G8

	// 30 DAY multipliers (G1/G10 * multiplier) – yahi pattern sheet me hai
	multipliers30 := []float64{
		1.0 / 7.0, 1.0 / 6.0, 1.0 / 5.0, 1.0 / 4.0, 1.0 / 3.0, 1.0 / 2.0,
		0.8, 5.0 / 6.0, 1.0,
		1.2, 1.4, 1.8, 2.2, 2.4, 2.8, 3.2, 3.5, 4, 4.5, 5,
		6, 7, 8, 9, 11, 13, 15, 20, 25, 30,
		40, 50, 60, 70, 80, 90, 100, 110, 130, 150,
	}

	plan30 = make([]WarmupDay, 0, maxDays)
	planLt30 = make([]WarmupDay, 0, maxDays)
	planGt30 = make([]WarmupDay, 0, maxDays)

	// Helper: <30 column (second column in sheet)
	lessThan30 := func(day int) float64 {
		d := day
		mc := medianCustom
		f := factor

		switch d {
		case 1:
			return (mc / 7.0) * f
		case 2:
			return (mc / 6.0) * f
		case 3:
			return (mc / 5.0) * f
		case 4:
			return (mc / 4.0) * f
		case 5:
			return (mc / 3.0) * f
		case 6:
			return (mc / 2.0) * f
		case 7:
			return (mc / 1.5) * f
		case 8:
			return (mc / 1.2) * f
		case 9:
			return mc * f
		}

		// Rows 10–60 : =(((G1/G8)*M)*G7)*M
		mults := []float64{
			1.2, 1.4, 1.8, 2.2, 2.4, 2.8, 3.2, 3.5, 4, 4.5, 5,
			6, 7, 8, 9, 11, 13, 15, 20, 25, 30,
			40, 50, 60, 70, 80, 90, 100, 110, 130, 150,
			30, 33, 35, 40, 45, 50, 55, 60, 65, 70,
			75, 80, 85, 90, 95, 100, 105, 110, 115, 120,
		}

		idx := d - 10
		if idx < 0 || idx >= len(mults) {
			return 0
		}
		m := mults[idx]
		return ((mc * m) * f) * m
	}

	// Helper: >30 column (third column in sheet)
	greaterThan30 := func(day int) float64 {
		d := day
		mc := medianCustom
		f := factor

		// Day 1–9: same as <30 column
		if d <= 9 {
			return lessThan30(d)
		}

		// Rows 10–60 : =(((G1/G8)*M)*G7)
		mults := []float64{
			1.2, 1.4, 1.6, 1.8, 2, 2.2, 2.4, 2.6, 2.8, 3,
			3.5, 4, 4.5, 5, 6, 7, 8, 9, 10, 11, 12,
			13, 14, 15, 16, 17, 18, 20, 22, 25, 28,
			30, 33, 35, 40, 45, 50, 55, 60, 65, 70,
			75, 80, 85, 90, 95, 100, 105, 110, 115, 120,
		}

		idx := d - 10
		if idx < 0 || idx >= len(mults) {
			return 0
		}
		m := mults[idx]
		return (mc * m) * f
	}

	for day := 1; day <= maxDays; day++ {
		// 30 DAY plan
		var v30 int
		if day <= len(multipliers30) {
			v30 = excelRound(median30 * multipliers30[day-1])
		} else {
			v30 = 0 // Excel me yahan NA hota hai – hum 0 rakh rhe
		}

		// <30 & >30 plans
		vLt := excelRound(lessThan30(day))
		vGt := excelRound(greaterThan30(day))

		plan30 = append(plan30, WarmupDay{Day: day, Limit: v30})
		planLt30 = append(planLt30, WarmupDay{Day: day, Limit: vLt})
		planGt30 = append(planGt30, WarmupDay{Day: day, Limit: vGt})
	}

	return
}

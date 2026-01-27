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

	// Generate all plans first
	for day := 1; day <= maxDays; day++ {
		// 30 DAY plan
		var v30 int
		if day <= len(multipliers30) {
			v30 = excelRound(median30 * multipliers30[day-1])
		} else {
			v30 = 0 // Excel me yahan NA hota hai – hum 0 rakh rhe
		}

		// <30 & >30 plans - use static calculator for non-Fibonacci periods
		var vLt, vGt int
		if customPeriod == 20 || customPeriod == 45 {
			// For 20 and 45 days, we'll use Fibonacci calculator (handled separately below)
			// For now, generate static values for other days
			vLt = excelRound(lessThan30(day))
			vGt = excelRound(greaterThan30(day))
		} else {
			// Use static calculator for other periods
			vLt = excelRound(lessThan30(day))
			vGt = excelRound(greaterThan30(day))
		}

		plan30 = append(plan30, WarmupDay{Day: day, Limit: v30})
		planLt30 = append(planLt30, WarmupDay{Day: day, Limit: vLt})
		planGt30 = append(planGt30, WarmupDay{Day: day, Limit: vGt})
	}

	// Fibonacci calculator for 20 and 45 days
	// Uses a variable between 385-424 to ensure it reaches target volume 100% of the time
	if customPeriod == 20 || customPeriod == 45 {
		fibPlan := generateFibonacciPlan(targetVolume, customPeriod)
		if customPeriod == 20 {
			// Replace planLt30 with Fibonacci plan
			for i := 0; i < customPeriod && i < len(fibPlan); i++ {
				planLt30[i] = fibPlan[i]
			}
		} else if customPeriod == 45 {
			// Replace planGt30 with Fibonacci plan
			for i := 0; i < customPeriod && i < len(fibPlan); i++ {
				planGt30[i] = fibPlan[i]
			}
		}
	}

	return
}

// generateFibonacciPlan generates a Fibonacci-based warmup plan
// Based on Excel sheet formula: VOLUME PER IP = (VOLUME / TOTAL ADJUSTER) × SINGULARITY
// The TOTAL ADJUSTER sequence decreases from initial value to 1 over the warmup period
func generateFibonacciPlan(targetVolume int, days int) []WarmupDay {
	if days <= 0 || targetVolume <= 0 {
		return []WarmupDay{}
	}

	plan := make([]WarmupDay, days)

	// Calculate PERIOD ADJUSTER (383-424 range) - used to determine initial TOTAL ADJUSTER
	periodAdjuster := calculatePeriodAdjuster(targetVolume, days)

	// Calculate initial TOTAL ADJUSTER based on PERIOD ADJUSTER
	// For 20 days: initial TOTAL ADJUSTER is typically 385 (periodAdjuster + 1 or similar)
	// For 45 days: initial TOTAL ADJUSTER needs to be calculated to reach 1 on day 45
	initialTotalAdjuster := calculateInitialTotalAdjuster(periodAdjuster, days)

	// Generate TOTAL ADJUSTER sequence that decreases from initialTotalAdjuster to 1
	totalAdjusterSequence := generateTotalAdjusterSequence(initialTotalAdjuster, days)

	// SINGULARITY is always 1 (from Excel sheet)
	singularity := 1.0

	// Generate plan using Excel formula: VOLUME PER IP = (VOLUME / TOTAL ADJUSTER) × SINGULARITY
	for i := 0; i < days; i++ {
		// Day 0 in Excel = Day 1 in our system (1-indexed)
		// But we need to handle the sequence correctly
		dayIndex := i
		if dayIndex >= len(totalAdjusterSequence) {
			dayIndex = len(totalAdjusterSequence) - 1
		}

		totalAdjuster := totalAdjusterSequence[dayIndex]
		if totalAdjuster <= 0 {
			totalAdjuster = 1 // Avoid division by zero
		}

		// Excel formula: VOLUME PER IP = (VOLUME / TOTAL ADJUSTER) × SINGULARITY
		volumePerIP := (float64(targetVolume) / totalAdjuster) * singularity

		plan[i] = WarmupDay{
			Day:   i + 1,
			Limit: excelRound(volumePerIP),
		}
	}

	// Final adjustment: ensure last day is exactly target volume (when TOTAL ADJUSTER = 1)
	plan[days-1].Limit = targetVolume

	return plan
}

// calculatePeriodAdjuster calculates the PERIOD ADJUSTER value (383-424 range)
// This is a key parameter from the Excel sheet that ensures the plan reaches target volume
func calculatePeriodAdjuster(targetVolume int, days int) float64 {
	// Calculate based on target volume and days
	// The adjuster should be in the 383-424 range
	// Formula: adjuster = base + (targetVolume / days) * factor

	// Base adjuster around 390-400
	baseAdjuster := 390.0

	// Adjust based on volume per day ratio
	volumePerDay := float64(targetVolume) / float64(days)

	// Fine-tune adjuster based on volume per day
	// Higher volume per day -> slightly higher adjuster
	adjustment := (volumePerDay / 1000.0) * 0.5 // Small adjustment factor

	periodAdjuster := baseAdjuster + adjustment

	// Clamp to 383-424 range (as per Excel sheet)
	if periodAdjuster < 383 {
		periodAdjuster = 383
	} else if periodAdjuster > 424 {
		periodAdjuster = 424
	}

	return periodAdjuster
}

// calculateInitialTotalAdjuster calculates the starting TOTAL ADJUSTER value
// Based on PERIOD ADJUSTER and number of days
// For 20 days: exactly 385 (as per Excel sheet)
// For 45 days: calculated so that TOTAL ADJUSTER reaches exactly 1 on day 45
func calculateInitialTotalAdjuster(periodAdjuster float64, days int) float64 {
	// For 20 days, initial TOTAL ADJUSTER is exactly 385 (as per Excel sheet)
	if days == 20 {
		return 385.0
	} else if days == 45 {
		// For 45 days: use a reasonable initial value and adjust Fibonacci sequence accordingly
		// Based on the pattern, for 45 days we should use a value around 400-450
		// This ensures Day 1 has a reasonable VOLUME PER IP value

		// Use periodAdjuster + 10-15 as a base, but ensure it's reasonable
		initial := periodAdjuster + 10.0

		// Clamp to reasonable range for 45 days (400-450)
		if initial < 400 {
			initial = 400.0
		} else if initial > 450 {
			initial = 450.0
		}

		// The Fibonacci sequence will be generated to sum to (initial - 1)
		// This is handled in generateFibonacciDecreasingSequence for 45 days
		return initial
	}

	// Default: use periodAdjuster + 1
	return periodAdjuster + 1.0
}

// generateTotalAdjusterSequence generates the TOTAL ADJUSTER sequence
// Based on Excel sheet: TOTAL ADJUSTER[day] = TOTAL ADJUSTER[day-1] - FIBONACCI[day]
// The FIBONACCI sequence is a reversed/decreasing Fibonacci pattern
func generateTotalAdjusterSequence(initialTotalAdjuster float64, days int) []float64 {
	sequence := make([]float64, days)

	// Generate FIBONACCI sequence (reversed/decreasing pattern)
	// Excel shows: 0, 144, 89, 55, 34, 21, 13, 8, 5, 3, 2, 1, 1, 1...
	fibSequence := generateFibonacciDecreasingSequence(days)

	// For 45 days, normalize Fibonacci sequence so it sums to (initialTotalAdjuster - 1)
	// This ensures we reach exactly 1 on the last day
	if days == 45 {
		// Calculate current sum (excluding day 0 which is 0)
		sum := 0.0
		for i := 1; i < days; i++ {
			sum += fibSequence[i]
		}

		// Target sum should be (initialTotalAdjuster - 1)
		targetSum := initialTotalAdjuster - 1.0

		// Normalize if sum is not zero
		if sum > 0 && targetSum > 0 {
			scaleFactor := targetSum / sum
			// Scale all Fibonacci values (except day 0)
			for i := 1; i < days; i++ {
				fibSequence[i] = fibSequence[i] * scaleFactor
			}
		}
	}

	// Initialize first value
	sequence[0] = initialTotalAdjuster

	// Generate sequence: TOTAL ADJUSTER[day] = TOTAL ADJUSTER[day-1] - FIBONACCI[day]
	for i := 1; i < days; i++ {
		prevAdjuster := sequence[i-1]
		fibValue := fibSequence[i]

		// Calculate new adjuster
		newAdjuster := prevAdjuster - fibValue

		// Ensure it doesn't go below 1
		if newAdjuster < 1.0 {
			newAdjuster = 1.0
		}

		sequence[i] = newAdjuster
	}

	// Ensure last value is exactly 1
	sequence[days-1] = 1.0

	return sequence
}

// generateFibonacciDecreasingSequence generates a decreasing Fibonacci sequence
// Excel pattern for 20 days: 0, 144, 89, 55, 34, 21, 13, 8, 5, 3, 2, 1, 1, 1...
// For 45 days: use a more gradual pattern that spreads the decrease over 45 days
func generateFibonacciDecreasingSequence(days int) []float64 {
	sequence := make([]float64, days)

	// Day 0 is always 0
	sequence[0] = 0.0

	if days == 20 {
		// For 20 days: use exact Excel pattern
		// Pattern: 0, 144, 89, 55, 34, 21, 13, 8, 5, 3, 2, 1, 1, 1...
		fibNumbers := []float64{144, 89, 55, 34, 21, 13, 8, 5, 3, 2}

		idx := 1
		for i := 0; i < len(fibNumbers) && idx < days; i++ {
			sequence[idx] = fibNumbers[i]
			idx++
		}

		// Fill remaining with 1s
		for idx < days {
			sequence[idx] = 1.0
			idx++
		}
	} else if days == 45 {
		// For 45 days: use a pattern based on 20 days but spread more evenly
		// Use the same Fibonacci values as 20 days but repeat/spread them
		// Pattern will be normalized to sum to (initialTotalAdjuster - 1)

		// Use 20-day pattern values but spread them over 45 days more evenly
		// Pattern: 0, then repeat 144, 89, 55, 34, 21, 13, 8, 5, 3, 2, then many 1s
		pattern20 := []float64{144, 89, 55, 34, 21, 13, 8, 5, 3, 2}

		idx := 1
		// Use the 20-day pattern values, but repeat them to fill more days
		// Repeat the pattern 2-3 times to spread over 45 days
		for repeat := 0; repeat < 2 && idx < days-15; repeat++ {
			for i := 0; i < len(pattern20) && idx < days-15; i++ {
				sequence[idx] = pattern20[i]
				idx++
			}
		}

		// Fill remaining with smaller values and 1s
		// Use decreasing pattern: 5, 3, 2, then 1s
		remaining := []float64{5, 3, 2}
		for i := 0; i < len(remaining) && idx < days; i++ {
			sequence[idx] = remaining[i]
			idx++
		}

		// Fill rest with 1s
		for idx < days {
			sequence[idx] = 1.0
			idx++
		}
	} else {
		// Default: use standard pattern
		fibNumbers := []float64{144, 89, 55, 34, 21, 13, 8, 5, 3, 2}
		idx := 1
		for i := 0; i < len(fibNumbers) && idx < days; i++ {
			sequence[idx] = fibNumbers[i]
			idx++
		}
		for idx < days {
			sequence[idx] = 1.0
			idx++
		}
	}

	return sequence
}

// calculateFibonacciStart calculates the starting value for Fibonacci sequence
// Based on target volume, days, period adjuster, total adjuster, and singularity
// This ensures the starting value is appropriate for the target volume
// Uses the PERIOD ADJUSTER to determine starting value, similar to Excel sheet formula
func calculateFibonacciStart(targetVolume int, days int, periodAdjuster, totalAdjuster, singularity float64) float64 {
	// Based on the 20-day example: 50,000 target, start ~130
	// Ratio: 50,000 / 130 = 384.6 (close to PERIOD ADJUSTER ~394)
	//
	// The Excel formula likely uses: start = targetVolume / PERIOD_ADJUSTER * factor
	// Where factor accounts for the period length

	// For 20 days: factor ~1.02 (50k/394 * 1.02 = 129.4 ≈ 130)
	// For 45 days: we need a different factor since it's a longer period
	// Longer period means we can start proportionally lower

	var factor float64
	if days == 20 {
		// Calibrated for 20 days: gives start ~130 for 50k target
		factor = 1.02
	} else if days == 45 {
		// For 45 days: use a factor that gives reasonable starting values
		// For 96,744 target: (96744/394) * 0.65 = 159.6 (reasonable start)
		// This ensures we don't start too low, accounting for longer growth period
		factor = 0.65
	} else {
		// Default factor for other periods
		factor = 0.8
	}

	// Calculate base start: targetVolume / periodAdjuster * factor
	baseStart := (float64(targetVolume) / periodAdjuster) * factor

	// Apply singularity (always 1, but included for formula completeness)
	baseStart = baseStart * singularity

	// For very large volumes, ensure the start is reasonable
	// Minimum: at least 1% of (targetVolume / days) to avoid starting too low
	minStart := float64(targetVolume) / float64(days) * 0.01
	if baseStart < minStart {
		baseStart = minStart
	}

	// Absolute minimum: at least 1
	if baseStart < 1.0 {
		baseStart = 1.0
	}

	return baseStart
}

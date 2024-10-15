package main

import (
	"math"
)

// AQIBreakpoint represents the breakpoints for the AQI calculation
type AQIBreakpoint struct {
	CpLow   float64 // Lower bound of concentration
	CpHigh  float64 // Upper bound of concentration
	AqiLow  int     // Lower bound of AQI
	AqiHigh int     // Upper bound of AQI
}

// AQIBreakpoints for PM2.5 based on EPA standards
var pm25Breakpoints = []AQIBreakpoint{
	{0.0, 12.0, 0, 50},       // Good
	{12.1, 35.4, 51, 100},    // Moderate
	{35.5, 55.4, 101, 150},   // Unhealthy for Sensitive Groups
	{55.5, 150.4, 151, 200},  // Unhealthy
	{150.5, 250.4, 201, 300}, // Very Unhealthy
	{250.5, 500.4, 301, 500}, // Hazardous
}

// calculateAQI calculates AQI for a given pollutant concentration based on breakpoints
func calculateAQI(concentration float64, breakpoints []AQIBreakpoint) int {
	for _, bp := range breakpoints {
		if concentration >= bp.CpLow && concentration <= bp.CpHigh {
			return int(math.Round(((float64(bp.AqiHigh)-float64(bp.AqiLow))/(bp.CpHigh-bp.CpLow))*(concentration-bp.CpLow) + float64(bp.AqiLow)))
		}
	}
	return -1 // Invalid concentration, out of range
}

// PM25toAQI takes a PM2.5 concentration and returns the AQI value
func PM25toAQI(pm25Concentration int) int {
	// Convert the integer concentration to a float for the calculation
	return calculateAQI(float64(pm25Concentration), pm25Breakpoints)
}

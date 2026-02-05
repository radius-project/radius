// Package discovery provides application discovery and analysis functionality.
package discovery

import (
	"fmt"
	"sort"
)

// ConfidenceLevel represents the confidence level of a detection.
type ConfidenceLevel string

const (
	// ConfidenceHigh indicates high confidence (>= 0.8).
	ConfidenceHigh ConfidenceLevel = "high"
	// ConfidenceMedium indicates medium confidence (>= 0.5, < 0.8).
	ConfidenceMedium ConfidenceLevel = "medium"
	// ConfidenceLow indicates low confidence (< 0.5).
	ConfidenceLow ConfidenceLevel = "low"
)

// ConfidenceThreshold defines filtering thresholds.
type ConfidenceThreshold struct {
	// MinConfidence is the minimum confidence to include.
	MinConfidence float64

	// HighThreshold is the threshold for high confidence.
	HighThreshold float64

	// MediumThreshold is the threshold for medium confidence.
	MediumThreshold float64
}

// DefaultConfidenceThreshold returns the default confidence thresholds.
func DefaultConfidenceThreshold() ConfidenceThreshold {
	return ConfidenceThreshold{
		MinConfidence:   0.5,
		HighThreshold:   0.8,
		MediumThreshold: 0.5,
	}
}

// GetLevel returns the confidence level for a given confidence value.
func (t ConfidenceThreshold) GetLevel(confidence float64) ConfidenceLevel {
	if confidence >= t.HighThreshold {
		return ConfidenceHigh
	}
	if confidence >= t.MediumThreshold {
		return ConfidenceMedium
	}
	return ConfidenceLow
}

// FormatConfidence formats a confidence value with level indicator.
func (t ConfidenceThreshold) FormatConfidence(confidence float64) string {
	level := t.GetLevel(confidence)
	percentage := confidence * 100

	var icon string
	switch level {
	case ConfidenceHigh:
		icon = "●"
	case ConfidenceMedium:
		icon = "◐"
	case ConfidenceLow:
		icon = "○"
	}

	return fmt.Sprintf("%s %.0f%%", icon, percentage)
}

// FilterByConfidence filters items by minimum confidence.
func FilterByConfidence[T any](items []T, getConfidence func(T) float64, minConfidence float64) []T {
	result := make([]T, 0, len(items))
	for _, item := range items {
		if getConfidence(item) >= minConfidence {
			result = append(result, item)
		}
	}
	return result
}

// SortByConfidence sorts items by confidence in descending order.
func SortByConfidence[T any](items []T, getConfidence func(T) float64) {
	sort.Slice(items, func(i, j int) bool {
		return getConfidence(items[i]) > getConfidence(items[j])
	})
}

// GroupByConfidenceLevel groups items by their confidence level.
func GroupByConfidenceLevel[T any](items []T, getConfidence func(T) float64, threshold ConfidenceThreshold) map[ConfidenceLevel][]T {
	groups := map[ConfidenceLevel][]T{
		ConfidenceHigh:   {},
		ConfidenceMedium: {},
		ConfidenceLow:    {},
	}

	for _, item := range items {
		level := threshold.GetLevel(getConfidence(item))
		groups[level] = append(groups[level], item)
	}

	return groups
}

// ConfidenceStats provides statistics about confidence levels in a result set.
type ConfidenceStats struct {
	// Total is the total number of items.
	Total int

	// HighCount is the number of high confidence items.
	HighCount int

	// MediumCount is the number of medium confidence items.
	MediumCount int

	// LowCount is the number of low confidence items.
	LowCount int

	// Average is the average confidence.
	Average float64

	// Min is the minimum confidence.
	Min float64

	// Max is the maximum confidence.
	Max float64
}

// CalculateConfidenceStats calculates statistics for a set of items.
func CalculateConfidenceStats[T any](items []T, getConfidence func(T) float64, threshold ConfidenceThreshold) ConfidenceStats {
	stats := ConfidenceStats{
		Total: len(items),
		Min:   1.0,
		Max:   0.0,
	}

	if len(items) == 0 {
		stats.Min = 0.0
		return stats
	}

	var sum float64
	for _, item := range items {
		conf := getConfidence(item)
		sum += conf

		if conf < stats.Min {
			stats.Min = conf
		}
		if conf > stats.Max {
			stats.Max = conf
		}

		level := threshold.GetLevel(conf)
		switch level {
		case ConfidenceHigh:
			stats.HighCount++
		case ConfidenceMedium:
			stats.MediumCount++
		case ConfidenceLow:
			stats.LowCount++
		}
	}

	stats.Average = sum / float64(len(items))
	return stats
}

// FormatConfidenceStats formats confidence statistics for display.
func FormatConfidenceStats(stats ConfidenceStats) string {
	return fmt.Sprintf(
		"Total: %d | High: %d (●) | Medium: %d (◐) | Low: %d (○) | Avg: %.0f%%",
		stats.Total,
		stats.HighCount,
		stats.MediumCount,
		stats.LowCount,
		stats.Average*100,
	)
}

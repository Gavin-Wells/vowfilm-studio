package studio

import "strings"

// videoModelMaxSegmentSeconds is the longest single /v1/videos/generate task the configured model supports.
func videoModelMaxSegmentSeconds(model string) int {
	m := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.Contains(m, "seedance"), strings.Contains(m, "doubao-seedance"):
		return 30
	case strings.Contains(m, "minimax"), strings.Contains(m, "/h3"):
		return 15
	default:
		return 15
	}
}

func commerceSubmitDuration(shot Shot) int {
	if shot.GenerateSeconds > 0 {
		return shot.GenerateSeconds
	}
	return shot.Duration
}

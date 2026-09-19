package studio

import (
	"fmt"
	"strconv"
	"strings"
)

const commerceDirectLegacyMode = "commerce-direct-15s-v3"

const commerceDirectMode = commerceDirectLegacyMode

func commerceDirectModeFor(duration int) string {
	return fmt.Sprintf("commerce-direct-%ds-v3", duration)
}

func isCommerceDuration(d int) bool {
	return d >= 10 && d <= 60
}

func isDirectCommerce(p *Project) bool {
	if p == nil || sceneID(p) != "commerce" {
		return false
	}
	if p.GenerationMode == commerceDirectLegacyMode {
		return true
	}
	if strings.HasPrefix(p.GenerationMode, "commerce-direct-") && strings.HasSuffix(p.GenerationMode, "-v3") {
		return isCommerceDuration(parseCommerceDirectDuration(p.GenerationMode))
	}
	return false
}

func parseCommerceDirectDuration(mode string) int {
	if mode == commerceDirectLegacyMode {
		return 15
	}
	mode = strings.TrimPrefix(mode, "commerce-direct-")
	mode = strings.TrimSuffix(mode, "s-v3")
	d, _ := strconv.Atoi(mode)
	return d
}

func directCommerceDurationMatches(p *Project) bool {
	if !isDirectCommerce(p) {
		return false
	}
	return parseCommerceDirectDuration(p.GenerationMode) == p.Duration
}

const maxCommerceSegmentSeconds = 15

// commerceSegmentLengths splits total seconds into MiniMax-sized chunks (≤15s each).
func commerceSegmentLengths(total int) []int {
	if total <= 0 {
		return nil
	}
	out := []int{}
	for remaining := total; remaining > 0; {
		if remaining <= maxCommerceSegmentSeconds {
			out = append(out, remaining)
			break
		}
		out = append(out, maxCommerceSegmentSeconds)
		remaining -= maxCommerceSegmentSeconds
	}
	return out
}

func commerceSegmentCount(total int) int {
	return len(commerceSegmentLengths(total))
}

func commerceUsesSegments(total int) bool {
	return total > maxCommerceSegmentSeconds
}

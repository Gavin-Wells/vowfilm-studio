package studio

import (
	"errors"
	"fmt"
	"strings"
)

func commerceStoryboardShotCount(duration int) int {
	if duration <= 15 {
		return max(3, duration/3)
	}
	return max(4, (duration+3)/4)
}

func packCommerceGenerateGroups(shots []Shot, maxSegment int) ([]Shot, error) {
	if maxSegment < 1 {
		return nil, errors.New("视频模型单段时长无效")
	}
	if len(shots) == 0 {
		return nil, errors.New("没有可分镜镜头")
	}
	out := make([]Shot, len(shots))
	copy(out, shots)
	group := 0
	i := 0
	for i < len(out) {
		group++
		gid := fmt.Sprintf("G%02d", group)
		sum := 0
		start := i
		for i < len(out) {
			next := out[i].Duration
			if next <= 0 {
				return nil, fmt.Errorf("镜头 %s 缺少时长", out[i].ID)
			}
			if sum > 0 && sum+next > maxSegment {
				break
			}
			if next > maxSegment {
				return nil, fmt.Errorf("单个分镜 %d 秒超过当前模型 %d 秒上限，请重新编排", next, maxSegment)
			}
			sum += next
			i++
		}
		if sum <= 0 {
			return nil, errors.New("生成包为空")
		}
		merged := mergeCommerceGroupPrompt(out[start:i], sum)
		for j := start; j < i; j++ {
			out[j].GenerateGroup = gid
			out[j].GenerateUnit = j == start
			if j == start {
				out[j].GenerateSeconds = sum
				out[j].Prompt = merged
			} else {
				out[j].GenerateSeconds = 0
			}
		}
	}
	return out, nil
}

func mergeCommerceGroupPrompt(shots []Shot, totalSeconds int) string {
	if len(shots) == 1 {
		return strings.TrimSpace(shots[0].Prompt)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "【本段提示词 · %d秒 · 连续%d镜】\n", totalSeconds, len(shots))
	for idx, s := range shots {
		if idx > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(strings.TrimSpace(s.Prompt))
	}
	return b.String()
}

func commerceGenerateUnitCount(shots []Shot) int {
	n := 0
	for _, s := range shots {
		if s.GenerateUnit {
			n++
		}
	}
	if n == 0 {
		return len(shots)
	}
	return n
}

func commerceGenerateLeaders(shots []Shot) []Shot {
	out := []Shot{}
	for _, s := range shots {
		if s.GenerateUnit {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return shots
	}
	return out
}

func commerceGroupPeerIDs(shots []Shot, leaderID string) []string {
	var group string
	for _, s := range shots {
		if s.ID == leaderID {
			group = s.GenerateGroup
			break
		}
	}
	if group == "" {
		return []string{leaderID}
	}
	ids := []string{}
	for _, s := range shots {
		if s.GenerateGroup == group {
			ids = append(ids, s.ID)
		}
	}
	return ids
}

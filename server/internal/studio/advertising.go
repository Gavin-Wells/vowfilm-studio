package studio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"vowfilm/server/internal/advertising"
)

// creativeAssets describes the exact visual ordering used by references().
// Music is not bound to video inputs; direct commerce retains model-generated audio.
func creativeAssets(p *Project) []map[string]any {
	out := []map[string]any{}
	imageIndex := 0
	for _, asset := range p.Assets {
		item := map[string]any{"role": asset.Role, "name": asset.Name, "visual_inspection": false}
		if asset.Role == "music" {
			item["usage"] = "非电商场景可用于后期混音；电商v3保留模型原生声音，不使用此素材，也未绑定音频参考"
		} else {
			imageIndex++
			item["input_index"] = imageIndex
			item["placeholder"] = fmt.Sprintf("<图片%d>", imageIndex)
		}
		out = append(out, item)
	}
	return out
}

func validateCommerceAction(p *Project, action string) error {
	if sceneID(p) != "commerce" || action == "render" || action == "review" {
		return nil
	}
	if !isCommerceDuration(p.Duration) {
		return errors.New("电商 v3 支持 10–60 秒整条直出，请在导演手记保存时长后重新编排")
	}
	if (action == "generate" || action == "shot") && len(p.Shots) > 0 {
		if !isDirectCommerce(p) || p.PromptPolicy != advertising.Version() {
			return errors.New("这是旧版电商分镜，请先重新编排为 v3 整条广告")
		}
		if !directCommerceDurationMatches(p) {
			return errors.New("时长与已编排版本不一致，请重新编排")
		}
		if err := validateCommerceStoryboard(p); err != nil {
			return err
		}
	}
	return nil
}

func validateCommerceStoryboard(p *Project) error {
	if len(p.Shots) < 1 {
		return errors.New("电商广告缺少分镜")
	}
	total := 0
	maxSeg := videoModelMaxSegmentSeconds(p.VideoModel)
	for _, s := range p.Shots {
		if s.Duration <= 0 {
			return fmt.Errorf("镜头 %s 缺少时长", s.ID)
		}
		total += s.Duration
		if s.GenerateUnit && s.GenerateSeconds > maxSeg {
			return fmt.Errorf("生成包 %s 为 %d 秒，超过当前模型单次 %d 秒上限", s.GenerateGroup, s.GenerateSeconds, maxSeg)
		}
	}
	if total != p.Duration {
		return fmt.Errorf("分镜总时长 %d 秒与工程 %d 秒不一致，请重新编排", total, p.Duration)
	}
	return nil
}

func (p *Provider) planCommerceDirect(ctx context.Context, project *Project) (string, []Shot, error) {
	if err := validateCommerceAction(project, "plan"); err != nil {
		return "", nil, err
	}
	return p.planCommerceStoryboard(ctx, project)
}

func (p *Provider) planCommerceStoryboard(ctx context.Context, project *Project) (string, []Shot, error) {
	shotCount := commerceStoryboardShotCount(project.Duration)
	maxSeg := videoModelMaxSegmentSeconds(p.config.VideoModel)
	brief := map[string]any{
		"title": project.Title, "facts": project.Brief, "custom_prompt": project.CustomPrompt,
		"voiceover_script": project.VoiceoverScript, "voice_direction": project.VoiceDirection,
		"duration_seconds": project.Duration, "shot_count": shotCount,
		"shot_duration_hint_seconds": "每镜约2–6秒，总和必须等于 duration_seconds",
		"video_model":                p.config.VideoModel, "model_max_segment_seconds": maxSeg,
		"ratio": project.Ratio, "occasion": occasion(project), "style": project.Style,
		"assets": creativeAssets(project), "ending_text": project.EndingText,
		"prompt_policy": advertising.Version(), "native_audio": true,
		"generation_strategy": "storyboard_then_pack_by_model_limit",
	}
	raw, _ := json.Marshal(brief)
	out, err := p.request(ctx, "POST", "/v1/chat/completions", map[string]any{
		"model": p.config.LLMModel,
		"messages": []map[string]any{
			{"role": "system", "content": advertising.SystemPrompt("direct-storyboard")},
			{"role": "user", "content": string(raw)},
		},
		"reasoning_effort": "low", "max_tokens": 9000,
	}, "")
	if err != nil {
		return "", nil, err
	}
	choices, _ := out["choices"].([]any)
	if len(choices) == 0 {
		return "", nil, errors.New("导演没有返回广告分镜")
	}
	first, _ := choices[0].(map[string]any)
	message, _ := first["message"].(map[string]any)
	content, _ := message["content"].(string)
	start, end := strings.Index(content, "{"), strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return "", nil, errors.New("广告分镜 JSON 格式无效")
	}
	var plan struct {
		Synopsis string `json:"synopsis"`
		Title    string `json:"title"`
		Shots    []struct {
			ID          string `json:"id"`
			Title       string `json:"title"`
			Duration    int    `json:"duration_seconds"`
			Description string `json:"description"`
			Camera      string `json:"camera"`
			EntryAction string `json:"entryAction"`
			ExitAction  string `json:"exitAction"`
			Prompt      string `json:"prompt"`
		} `json:"shots"`
	}
	if err = json.Unmarshal([]byte(content[start:end+1]), &plan); err != nil {
		return "", nil, fmt.Errorf("广告分镜 JSON 校验失败：%w", err)
	}
	if strings.TrimSpace(plan.Title) == "" || len(plan.Shots) < 1 {
		return "", nil, errors.New("广告分镜缺失")
	}
	shots := make([]Shot, len(plan.Shots))
	sum := 0
	cursor := 0.0
	for i, item := range plan.Shots {
		if item.Duration <= 0 || len([]rune(item.Prompt)) > 8000 {
			return "", nil, errors.New("分镜时长或指令无效")
		}
		if err = validateCommercialReferences(project, item.Prompt); err != nil {
			return "", nil, err
		}
		id := item.ID
		if id == "" {
			id = fmt.Sprintf("S%02d", i+1)
		}
		title := item.Title
		if title == "" {
			title = fmt.Sprintf("%s · 镜%d", plan.Title, i+1)
		}
		dur := item.Duration
		sum += dur
		shots[i] = Shot{
			ID: id, Title: title, Chapter: fmt.Sprintf("分镜 %d", i+1),
			Description: item.Description, Camera: item.Camera,
			EntryAction: item.EntryAction, ExitAction: item.ExitAction,
			Prompt: item.Prompt, Transition: "cut", Status: "pending", Attempt: 1,
			Duration: dur, EditFrames: dur * 24, EditSeconds: float64(dur), TimelineStart: cursor,
		}
		cursor += float64(dur)
	}
	if sum != project.Duration {
		return "", nil, fmt.Errorf("分镜总时长 %d 秒，需要 %d 秒", sum, project.Duration)
	}
	packed, err := packCommerceGenerateGroups(shots, maxSeg)
	if err != nil {
		return "", nil, err
	}
	return plan.Synopsis, packed, nil
}

func validateCommercialReferences(p *Project, prompt string) error {
	if sceneID(p) != "commerce" {
		return nil
	}
	count := 0
	for _, a := range p.Assets {
		if a.Role != "music" {
			count++
		}
	}
	return advertising.ValidateReferences(prompt, count)
}
func advertisingVersion(p *Project) string {
	if sceneID(p) == "commerce" {
		return advertising.Version()
	}
	return ""
}
func commercialTextRequested(p *Project) bool {
	text := strings.ToLower(p.CustomPrompt)
	for _, phrase := range []string{"无字幕", "不要字幕", "不加字幕", "不新增字幕", "不添加字幕", "不需要字幕", "无文字", "无新增画面文字", "不新增画面文字", "no subtitles", "no captions"} {
		if strings.Contains(text, phrase) {
			return false
		}
	}
	return strings.Contains(text, "字幕") || strings.Contains(text, "字卡") || strings.Contains(text, "片尾文字")
}

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

const commerceDirectMode = "commerce-direct-15s-v3"

func isDirectCommerce(p *Project) bool {
	return sceneID(p) == "commerce" && p.GenerationMode == commerceDirectMode
}

func validateCommerceAction(p *Project, action string) error {
	if sceneID(p) != "commerce" || action == "render" || action == "review" {
		return nil
	}
	if p.Duration != 15 {
		return errors.New("电商 v3 直出15秒，请在导演手记保存15秒设置后重新编排")
	}
	if (action == "generate" || action == "shot") && len(p.Shots) > 0 && (!isDirectCommerce(p) || p.PromptPolicy != advertising.Version() || len(p.Shots) != 1) {
		return errors.New("这是旧版电商分镜，请先重新编排为 v3 的15秒整条广告")
	}
	return nil
}

func (p *Provider) planCommerceDirect(ctx context.Context, project *Project) (string, []Shot, error) {
	if err := validateCommerceAction(project, "plan"); err != nil {
		return "", nil, err
	}
	brief := map[string]any{"title": project.Title, "facts": project.Brief, "custom_prompt": project.CustomPrompt, "duration_seconds": 15, "ratio": project.Ratio, "occasion": occasion(project), "style": project.Style, "assets": creativeAssets(project), "ending_text": project.EndingText, "prompt_policy": advertising.Version(), "native_audio": true}
	raw, _ := json.Marshal(brief)
	out, err := p.request(ctx, "POST", "/v1/chat/completions", map[string]any{"model": p.config.LLMModel, "messages": []map[string]any{{"role": "system", "content": advertising.SystemPrompt("direct")}, {"role": "user", "content": string(raw)}}, "reasoning_effort": "low", "max_tokens": 6500}, "")
	if err != nil {
		return "", nil, err
	}
	choices, _ := out["choices"].([]any)
	if len(choices) == 0 {
		return "", nil, errors.New("导演没有返回整条广告指令")
	}
	first, _ := choices[0].(map[string]any)
	message, _ := first["message"].(map[string]any)
	content, _ := message["content"].(string)
	start, end := strings.Index(content, "{"), strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return "", nil, errors.New("广告指令 JSON 格式无效")
	}
	var plan struct {
		Synopsis    string `json:"synopsis"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Camera      string `json:"camera"`
		EntryAction string `json:"entryAction"`
		ExitAction  string `json:"exitAction"`
		Prompt      string `json:"prompt"`
	}
	if err = json.Unmarshal([]byte(content[start:end+1]), &plan); err != nil {
		return "", nil, fmt.Errorf("广告指令 JSON 校验失败：%w", err)
	}
	if strings.TrimSpace(plan.Title) == "" || len([]rune(plan.Prompt)) > 10000 {
		return "", nil, errors.New("广告标题缺失或完整指令超过10000字")
	}
	if err = validateCommercialReferences(project, plan.Prompt); err != nil {
		return "", nil, err
	}
	shot := Shot{ID: "S01", Title: plan.Title, Chapter: "15秒完整广告", Description: plan.Description, Camera: plan.Camera, EntryAction: plan.EntryAction, ExitAction: plan.ExitAction, Prompt: plan.Prompt, Transition: "cut", Status: "pending", Attempt: 1, Duration: 15, EditFrames: 360, EditSeconds: 15}
	return plan.Synopsis, []Shot{shot}, nil
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

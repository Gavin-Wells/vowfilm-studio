package studio

import (
	"errors"
	"fmt"
	"strings"
	"vowfilm/server/internal/templates"
)

func validateTemplateProject(p *Project) error {
	if p.CreationMode == "" || p.CreationMode == "agent" {
		if p.TemplateID != "" {
			return errors.New("Agent 创作不能绑定固定模板")
		}
		return nil
	}
	if p.CreationMode != "template" {
		return errors.New("请选择模板创作或 Agent 创作")
	}
	t, ok := templates.Find(p.TemplateID)
	if !ok {
		return errors.New("模板不存在或已下架")
	}
	if p.TemplateVersion != t.Version {
		return errors.New("模板版本已变化，请重新选择模板")
	}
	if p.Scene != t.Scene || p.Duration != t.Duration || p.Ratio != t.Ratio || p.Style != t.Style || p.Occasion != t.Occasion || p.WardrobeMode != t.WardrobeMode {
		return errors.New("模板规格与固定方案不一致")
	}
	return nil
}

func templatePlan(p *Project) (string, []Shot, error) {
	if err := validateTemplateProject(p); err != nil {
		return "", nil, err
	}
	t, ok := templates.Find(p.TemplateID)
	if !ok {
		return "", nil, fmt.Errorf("模板不存在或已下架")
	}
	shots := make([]Shot, 0, len(t.Shots))
	for i, item := range t.Shots {
		chapter := t.Chapters[item.Chapter]
		prompt := strings.Join([]string{
			"婚礼影片模板《" + t.Name + "》；", chapter.Title + "，" + chapter.Setting + "。",
			"本章造型：" + chapter.Look, item.Action,
			"人物面貌与身份全片一致，同章服装和配饰固定。造型只在两个章节的独立镜头之间剪切更换，当前镜头内禁止换装、变脸或人体变形。单一连续镜头，无字幕、无文字、无音乐。",
			"面貌以提供的人物素材为准，无人物参考时采用统一的两位虚构成年新人；本片跨时代情节均为艺术化表达，不声称是真实人生经历。运镜：" + item.Camera + "。",
			"只使用用户提供的真实人物与故事资料，不补写姓名、日期或经历。资料仅用于人物细节，不改变此镜固定动作：" + p.Brief,
			"局部拍摄要求（不改变模板的场景、镜头顺序、服装与时长）：" + p.CustomPrompt,
		}, "")
		shots = append(shots, Shot{ID: fmt.Sprintf("S%02d", i+1), Title: item.Title, Chapter: chapter.Title, Description: item.Action, Camera: item.Camera, Prompt: prompt, Caption: item.Caption, Transition: "cut", EntryAction: item.Action, ExitAction: "动作自然停稳并把视线交给下一镜", TransitionReason: "固定模板按动作与方向连续剪接", Status: "pending", Attempt: 1, ActID: fmt.Sprintf("A%d", item.Chapter+1)})
	}
	return "以同一对主角串起古风初见、荷塘同游、中式誓约、复古重逢、现代相伴与当代婚礼；六幕十二镜头，分章换装，片尾留画三秒交还现场。跨时代情节为艺术化表达。", shots, nil
}

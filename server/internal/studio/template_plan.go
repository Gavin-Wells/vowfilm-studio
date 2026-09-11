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
	t, ok := templates.FindVersion(p.TemplateID, p.TemplateVersion)
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
	t, ok := templates.FindVersion(p.TemplateID, p.TemplateVersion)
	if !ok {
		return "", nil, fmt.Errorf("模板不存在或已下架")
	}
	shots := make([]Shot, 0, len(t.Shots))
	for i, item := range t.Shots {
		chapter := t.Chapters[item.Chapter]
		transition := item.Transition
		if transition == "" {
			transition = templateTransition(p, i)
		}
		entryAction := item.EntryAction
		if entryAction == "" {
			entryAction = item.Action
		}
		exitAction := item.ExitAction
		if exitAction == "" {
			exitAction = "动作自然停稳并把视线交给下一镜"
		}
		reason := item.TransitionReason
		if reason == "" {
			reason = templateTransitionReason(transition)
		}
		energy := ""
		if p.TemplateVersion == "3" {
			energy = "高能量婚庆预告片，144 BPM 强拍剪辑；开头第一拍就进入动作，人物必须主动走、转、拉、接或环绕，不站定摆拍；每个镜头至少有一次明确位移，结尾在强拍动作点交给下一镜。"
		}
		prompt := strings.Join([]string{
			"婚礼影片模板《" + t.Name + "》；", chapter.Title + "，" + chapter.Setting + "。",
			"本章造型：" + chapter.Look, item.Action,
			energy,
			"人物面貌与身份全片一致，同章服装和配饰固定。造型只在两个章节的独立镜头之间剪切更换，当前镜头内禁止换装、变脸或人体变形。单一连续镜头，无字幕、无文字、无音乐。",
			"面貌以提供的人物素材为准，无人物参考时采用统一的两位虚构成年新人；本片跨时代情节均为艺术化表达，不声称是真实人生经历。运镜：" + item.Camera + "。画面必须从动作开始，保持连续可见的身体或摄影机运动，结尾动作停稳；避免静止摆拍、空镜和全程慢推。",
			"只使用用户提供的真实人物与故事资料，不补写姓名、日期或经历。资料仅用于人物细节，不改变此镜固定动作：" + p.Brief,
			"局部拍摄要求（不改变模板的场景、镜头顺序、服装与时长）：" + p.CustomPrompt,
		}, "")
		shots = append(shots, Shot{ID: fmt.Sprintf("S%02d", i+1), Title: item.Title, Chapter: chapter.Title, Description: item.Action, Camera: item.Camera, Prompt: prompt, Caption: item.Caption, Transition: transition, EntryAction: entryAction, ExitAction: exitAction, TransitionReason: reason, EditFrames: item.EditFrames, Status: "pending", Attempt: 1, ActID: fmt.Sprintf("A%d", item.Chapter+1)})
	}
	return "以同一对主角串起古风初见、荷塘同游、中式誓约、复古重逢、现代相伴与当代婚礼；六幕十二镜头，分章换装，片尾留画三秒交还现场。跨时代情节为艺术化表达。", shots, nil
}

// The fixed template uses a restrained transition rhythm: cuts keep the story
// moving, matches connect adjacent gestures, and chapter boundaries get one
// short dissolve or dip-to-white instead of a stack of decorative effects.
func templateTransition(p *Project, index int) string {
	if p.TemplateVersion == "1" {
		return "cut"
	}
	switch index {
	case 1, 5, 8:
		return "match"
	case 2, 7:
		return "dissolve"
	case 4, 10:
		return "dipwhite"
	default:
		return "cut"
	}
}

func templateTransitionReason(transition string) string {
	switch transition {
	case "match":
		return "用相同方向的手势或视线承接下一镜"
	case "dissolve":
		return "章节情绪转换，短叠化连接时间与场景"
	case "dipwhite":
		return "誓约或婚礼章节节点，用短闪白提升仪式感"
	default:
		return "动作完成后利落切入下一镜"
	}
}

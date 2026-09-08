package studio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Provider struct {
	config Config
	client *http.Client
}

func newProvider(c Config) *Provider {
	return &Provider{c, &http.Client{Timeout: 240 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (p *Provider) request(ctx context.Context, method, path string, body any, idempotency string) (map[string]any, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(p.config.BaseURL, "/")+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	req.Header.Set("Content-Type", "application/json")
	if idempotency != "" {
		req.Header.Set("Idempotency-Key", idempotency)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("模型服务连接失败: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err = json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("模型服务返回了无效响应（HTTP %d）", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := out["message"].(string)
		if msg == "" {
			msg, _ = out["error"].(string)
		}
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		msg = strings.ReplaceAll(msg, p.config.APIKey, "[redacted]")
		if len(msg) > 800 {
			msg = msg[:800]
		}
		return nil, errors.New(msg)
	}
	return out, nil
}

const directorPrompt = `你是婚礼影片导演和分镜编剧。只返回一个合法 JSON 对象，不要 Markdown。
字段为 synopsis（100字内中文故事梗概）、shots（指定数量的镜头数组）。
每个镜头必须有 title（短中文标题）、chapter（序章/相遇/相伴/誓约/余生）、description（中文画面概述）、camera（短中文运镜）、caption（一句15字以内原创中文心语）、transition（cut、match、dipwhite或dissolve），entryAction（开头的动作与构图）、exitAction（结尾的动作与构图）、transitionReason（如何承接下一镜头）、prompt（详细中文视频生成指令）。
要求：严格遵守输入的风格指引。相邻镜头必须在动作、运动方向、视线或构图形状上有明确承接；先想好上一个镜头如何结束、下一个如何开始。至少80%为cut或match，最多两处有叙事理由的dissolve或dipwhite，不得连续叠化。禁止每个镜头都只有慢推或背影，必须包含人物笑容、互动和事件发展。镜头章节有开场、推进、转折、高潮和收尾。远景、中景和物件细节交替，避免重复画面。每个片段只安排一个主要动作和一种运镜，为剪辑留出稳定开头和结尾。固定人物造型、发型、左右位置和场景光线。不得编造用户未提供的具体经历、日期和地点。所有字幕由后期添加，视频生成指令必须写明无字幕无文字，单一连续镜头，不要背景音乐。不要输出外部素材URL、API参数或自行指定未提供的真实身份。
虚构演示时，人物为成年虚构新人：中国成年新人，新娘黑色低盘发、无肩带直线抹胸象牙白丝绸A字婚纱；新郎黑色短发、深藏蓝无图案普通男士西服、白衬衫无领带。展示自然笑容，固定同一造型，禁止变成长袖婚纱或更换西装颜色。不要模仿真实名人。`

func (p *Provider) Plan(ctx context.Context, project *Project) (string, []Shot, error) {
	count := project.Duration / 60 * profileFor(project.Style).ShotsPerMinute
	if !rhythmicStyle(project.Style) {
		count = project.Duration / 60 * 8
	}
	var all []Shot
	synopsis := ""
	for offset := 0; offset < count; offset += 8 {
		batch := 8
		if count-offset < batch {
			batch = count - offset
		}
		previous := "第一批镜头，从故事开场开始。"
		if len(all) > 0 {
			last := all[len(all)-1]
			previous = "上一镜头：" + last.Title + "；画面：" + last.Description + "；结尾：" + last.ExitAction + "。继续发展故事，不要重新开场。"
		}
		summary, shots, err := p.planBatch(ctx, project, batch, offset, count, previous)
		if err != nil {
			return "", nil, err
		}
		if synopsis == "" {
			synopsis = summary
		}
		all = append(all, shots...)
	}
	decorative := 0
	for i := range all {
		if (project.Style == "joyful" || project.Style == "travel" || project.Style == "editorial") && all[i].Transition == "dissolve" {
			all[i].Transition = "match"
		}
		if overlapFrames(all[i].Transition) > 0 {
			decorative++
			if decorative > 2 {
				all[i].Transition = "cut"
			}
		}
	}
	return synopsis, all, nil
}
func (p *Provider) planBatch(ctx context.Context, project *Project, count, offset, total int, previous string) (string, []Shot, error) {
	profile := profileFor(project.Style)
	assetInfo := []map[string]string{}
	for _, a := range project.Assets {
		assetInfo = append(assetInfo, map[string]string{"role": a.Role, "name": a.Name})
	}
	brief := map[string]any{"title": project.Title, "brief": project.Brief, "duration_seconds": project.Duration, "shot_count": count, "total_shot_count": total, "global_start_index": offset + 1, "global_end_index": offset + count, "previous_shot_context": previous, "batch_instruction": "本次只输出指定全片范围内的镜头。根据全片位置推进开场、相遇、心动、庆祝与收尾，不要在每批重新开场或提前结尾。每个镜头prompt控制在80–120汉字，entryAction/exitAction各15字以内，transitionReason20字以内。", "style": project.Style, "ratio": project.Ratio, "fictional_demo": project.Demo, "assets": assetInfo, "style_direction": profile.Direction, "music_direction": profile.Music, "bpm": profile.BPM, "story_structure": "开场0–13%，推进13–40%，转折40–67%，庆祝高潮67–87%，收尾87–100%"}
	raw, _ := json.Marshal(brief)
	if project.Demo && project.Style == "garden" && project.Duration == 60 && offset == 0 {
		raw = append(raw, []byte("\n第1镜头必须为：无人花园空镜，摄影机沿白色玫瑰花瓣小径缓慢前推，远处石砌别墅与白玫瑰花门，清晨金色光线、橄榄树、白色薄纱；用于承接已准备的开场素材。")...)
	}
	out, err := p.request(ctx, "POST", "/v1/chat/completions", map[string]any{"model": p.config.LLMModel, "messages": []map[string]any{{"role": "system", "content": directorPrompt}, {"role": "user", "content": string(raw)}}, "reasoning_effort": "low", "max_tokens": 6500}, "")
	if err != nil {
		return "", nil, err
	}
	choices, _ := out["choices"].([]any)
	if len(choices) == 0 {
		return "", nil, errors.New("导演没有返回可用的分镜")
	}
	first, _ := choices[0].(map[string]any)
	message, _ := first["message"].(map[string]any)
	content, _ := message["content"].(string)
	start, end := strings.Index(content, "{"), strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return "", nil, errors.New("导演返回的分镜格式不正确")
	}
	var plan struct {
		Synopsis string `json:"synopsis"`
		Shots    []Shot `json:"shots"`
	}
	if err = json.Unmarshal([]byte(content[start:end+1]), &plan); err != nil {
		return "", nil, fmt.Errorf("分镜 JSON 校验失败: %w", err)
	}
	if len(plan.Shots) != count {
		return "", nil, fmt.Errorf("导演返回 %d 个镜头，需要 %d 个；请重新编排", len(plan.Shots), count)
	}
	for i := range plan.Shots {
		s := &plan.Shots[i]
		if s.Title == "" || s.Prompt == "" || len([]rune(s.Prompt)) > 5000 {
			return "", nil, errors.New("镜头标题或生成指令不完整")
		}
		s.ID = fmt.Sprintf("S%02d", offset+i+1)
		s.Status = "pending"
		s.Attempt = 1
		s.Reserved = false
		s.TaskID = ""
		s.VideoURL = ""
		s.VideoFile = ""
		s.ThumbnailURL = ""
		s.Error = ""
		switch s.Transition {
		case "cut", "match", "dipwhite", "dissolve":
		default:
			s.Transition = "cut"
		}
		if project.Demo && rhythmicStyle(project.Style) {
			s.Prompt = "统一虚构人物造型：中国成年新娘，黑色低盘发，无肩带直线抹胸象牙白丝绸A字婚纱；中国成年新郎，黑色短发，深藏蓝无图案普通男士西服，白衬衫无领带，无肩章袖章、无制服徽章。绝不改变服装发型。" + s.Prompt
		}
		s.Prompt += "。开头衔接：" + s.EntryAction + "。结尾衔接：" + s.ExitAction + "。按正常速度自然运动，不要全程慢动作；无字幕、无文字。"
	}
	return plan.Synopsis, plan.Shots, nil
}
func (p *Provider) Submit(ctx context.Context, project *Project, shot Shot, refs []map[string]any) (string, error) {
	content := []map[string]any{{"type": "text", "text": shot.Prompt}}
	content = append(content, refs...)
	out, err := p.request(ctx, "POST", "/v1/videos/generate", map[string]any{"model": p.config.VideoModel, "content": content, "duration": shot.Duration, "ratio": project.Ratio, "resolution": "720p", "generate_audio": false, "watermark": false}, fmt.Sprintf("vowfilm:%s:r%d:%s:a%d", project.ID, project.Revision, shot.ID, shot.Attempt))
	if err != nil {
		return "", err
	}
	id, _ := out["task_id"].(string)
	if id == "" {
		return "", errors.New("视频服务未返回任务 ID")
	}
	return id, nil
}
func (p *Provider) Poll(ctx context.Context, id string) (string, string, error) {
	out, err := p.request(ctx, "GET", "/v1/tasks/"+id, nil, "")
	if err != nil {
		return "", "", err
	}
	status, _ := out["status"].(string)
	if status == "failed" || status == "cancelled" || status == "expired" {
		raw, _ := json.Marshal(out["error"])
		msg := string(raw)
		if msg == "null" {
			msg = "视频生成未成功，请查看制作记录后重试"
		}
		return status, "", errors.New(msg)
	}
	if status == "succeeded" || status == "completed" {
		results, _ := out["results"].([]any)
		for _, v := range results {
			m, _ := v.(map[string]any)
			if u, ok := m["url"].(string); ok && u != "" {
				return "succeeded", u, nil
			}
		}
		if id := archivedAssetID(out); id != "" {
			return "succeeded", "https://cdn.embervale.cn/assets/" + id + "/source.mp4", nil
		}
		return status, "", errors.New("视频任务完成但未返回文件")
	}
	return status, "", nil
}

func (p *Provider) Review(ctx context.Context, project *Project) (string, map[string]string, error) {
	shots := []map[string]string{}
	for _, s := range project.Shots {
		shots = append(shots, map[string]string{"id": s.ID, "title": s.Title, "description": s.Description, "prompt": s.Prompt, "caption": s.Caption})
	}
	raw, _ := json.Marshal(map[string]any{"title": project.Title, "brief": project.Brief, "shots": shots})
	out, err := p.request(ctx, "POST", "/v1/chat/completions", map[string]any{"model": p.config.LLMModel, "messages": []map[string]string{{"role": "system", "content": "你是婚礼电影的终审导演。已有分镜和部分已生成镜头，请审阅整体叙事，重写简洁优美的中文故事梗概与逐镜头字幕。保持已有画面语义，不改动人物设定，不添加未提供的真实经历。字幕每条最多15个汉字，形成从相遇到相守的自然推进，避免重复空泛抒情。只返回JSON对象：synopsis（100字以内中文）、captions（镜头id到字幕的对象，必须覆盖所有镜头）。"}, {"role": "user", "content": string(raw)}}, "reasoning_effort": "low", "max_tokens": 2200}, "")
	if err != nil {
		return "", nil, err
	}
	choices, _ := out["choices"].([]any)
	if len(choices) == 0 {
		return "", nil, errors.New("GPT-6 未返回审阅结果")
	}
	first, _ := choices[0].(map[string]any)
	message, _ := first["message"].(map[string]any)
	text, _ := message["content"].(string)
	start, end := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return "", nil, errors.New("GPT-6 审阅结果格式不正确")
	}
	var result struct {
		Synopsis string            `json:"synopsis"`
		Captions map[string]string `json:"captions"`
	}
	if err = json.Unmarshal([]byte(text[start:end+1]), &result); err != nil {
		return "", nil, err
	}
	for _, s := range project.Shots {
		if result.Captions[s.ID] == "" || len([]rune(result.Captions[s.ID])) > 25 {
			return "", nil, errors.New("GPT-6 返回的字幕不完整或过长")
		}
	}
	return result.Synopsis, result.Captions, nil
}

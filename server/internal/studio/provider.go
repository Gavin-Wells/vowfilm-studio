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
	return &Provider{c, &http.Client{Timeout: 150 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}
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
每个镜头必须有 title（短中文标题）、chapter（序章/相遇/相伴/誓约/余生）、description（中文画面概述）、camera（短中文运镜）、caption（一句15字以内原创中文心语）、transition（cut或dissolve）、prompt（详细中文视频生成指令）。
要求：按清晰的情绪推进安排镜头；约70%的镜头自然切接，其余使用柔和叠化。远景、中景和物件细节交替，避免重复画面。每个片段只安排一个主要动作和一种运镜，为剪辑留出稳定开头和结尾。固定人物造型、发型、左右位置和场景光线。不得编造用户未提供的具体经历、日期和地点。所有字幕由后期添加，视频生成指令必须写明无字幕无文字，单一连续镜头，不要背景音乐。不要输出外部素材URL、API参数或自行指定未提供的真实身份。
虚构演示时，人物为成年虚构新人：新娘黑色低盘发、象牙白简洁缎面婚纱；新郎黑色短发、深墨绿西装。优先背影、侧后方和远景以及花束、戒指、光线等细节，保持同一造型。不要模仿真实名人。`

func (p *Provider) Plan(ctx context.Context, project *Project) (string, []Shot, error) {
	count := project.Duration / 60 * 8
	assetInfo := []map[string]string{}
	for _, a := range project.Assets {
		assetInfo = append(assetInfo, map[string]string{"role": a.Role, "name": a.Name})
	}
	brief := map[string]any{"title": project.Title, "brief": project.Brief, "duration_seconds": project.Duration, "shot_count": count, "style": project.Style, "ratio": project.Ratio, "fictional_demo": project.Demo, "assets": assetInfo}
	raw, _ := json.Marshal(brief)
	if project.Demo && project.Style == "garden" && project.Duration == 60 {
		raw = append(raw, []byte("\n第1镜头必须为：无人花园空镜，摄影机沿白色玫瑰花瓣小径缓慢前推，远处石砌别墅与白玫瑰花门，清晨金色光线、橄榄树、白色薄纱；用于承接已准备的开场素材。")...)
	}
	out, err := p.request(ctx, "POST", "/v1/chat/completions", map[string]any{"model": p.config.LLMModel, "messages": []map[string]any{{"role": "system", "content": directorPrompt}, {"role": "user", "content": string(raw)}}, "max_tokens": 11000}, "")
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
		s.ID = fmt.Sprintf("S%02d", i+1)
		s.Status = "pending"
		s.Attempt = 1
		s.Reserved = false
		s.TaskID = ""
		s.VideoURL = ""
		s.VideoFile = ""
		s.ThumbnailURL = ""
		s.Error = ""
		if s.Transition != "cut" {
			s.Transition = "dissolve"
		}
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
	out, err := p.request(ctx, "POST", "/v1/chat/completions", map[string]any{"model": p.config.LLMModel, "messages": []map[string]string{{"role": "system", "content": "你是婚礼电影的终审导演。已有分镜和部分已生成镜头，请审阅整体叙事，重写简洁优美的中文故事梗概与逐镜头字幕。保持已有画面语义，不改动人物设定，不添加未提供的真实经历。字幕每条最多15个汉字，形成从相遇到相守的自然推进，避免重复空泛抒情。只返回JSON对象：synopsis（100字以内中文）、captions（镜头id到字幕的对象，必须覆盖所有镜头）。"}, {"role": "user", "content": string(raw)}}, "max_tokens": 2200}, "")
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

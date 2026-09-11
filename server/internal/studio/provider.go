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
	"vowfilm/server/internal/advertising"
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

const directorPrompt = `你是婚礼现场影片的分镜导演，只返回JSON：synopsis（100字内）、shots（指定数量镜头）。
每镜必须有title、chapter、description、camera、caption（15字以内）、transition（cut/match/dipwhite/dissolve）、entryAction、exitAction、transitionReason、prompt（80–160字中文单镜头指令）。
严格执行输入的treatment与shot_assignments：每镜所在章节、场景、造型已经明确分配，不能自行替换；同章衣服保持，章边允许按方案换装，人物面貌与身份不变。custom_prompt中的具体内容高于style_defaults；用户禁止的群像、道具、场景或镜头不能因默认风格再出现。实现mustHave并避开avoid，不能把用户Prompt仅用于摘要。
prompt必须写成可直接提交视频模型的完整段落，并按以下顺序包含：【本段提示词时长】、【环境与视觉分析】、【拍摄要求】、【道具动作账本】、【禁项】、【参考素材】。引用素材时只使用输入assets给出的固定编号（@图片N或兼容的<图片N>），不得凭文件名猜测图片内容，不得创建@音频N。
每镜一个主要动作和一种运镜，相邻镜头有动作、方向、视线、道具或构图承接。章边换装用遮挡/同向旋转/道具特写，两段分别生成再切接，禁止衣服和人体液化变形。结尾镜头留出稳定亮相，适合大屏片尾留场。至少80%为cut/match，至多两处有理由的叠化或闪白，不得连续叠化。人物要有互动、表情和关系推进，远中近景交替，不全程慢推、空镜和背影。慢动作仅在用户要求或少量强调时使用。
只使用用户提供的真实经历、姓名日期，不编造事实；资料不足用象征性情节。所有文字后期添加，视频prompt必须注明单一连续镜头、无字幕无文字无音乐。不要输出API参数、URL或文件路径。`

func (p *Provider) Plan(ctx context.Context, project *Project) (string, []Shot, error) {
	if sceneID(project) == "commerce" {
		return p.planCommerceDirect(ctx, project)
	}
	count := shotCount(project)
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
	if err := applyContinuity(project, all); err != nil {
		return "", nil, err
	}
	return synopsis, all, nil
}
func (p *Provider) planBatch(ctx context.Context, project *Project, count, offset, total int, previous string) (string, []Shot, error) {
	profile := profileForProject(project)
	assetInfo := creativeAssets(project)
	assignments := []map[string]any{}
	for i := offset + 1; i <= offset+count; i++ {
		act := actForShot(project.Treatment, i)
		if act != nil {
			assignments = append(assignments, map[string]any{"shot_index": i, "act": act, "look": lookByID(project.Treatment, act.LookID)})
		}
	}
	brief := map[string]any{"scene": sceneID(project), "scene_direction": sceneFor(project).Direction, "title": project.Title, "facts": project.Brief, "custom_prompt": project.CustomPrompt, "wardrobe_prompt": project.WardrobePrompt, "duration_seconds": project.Duration, "shot_count": count, "total_shot_count": total, "global_start_index": offset + 1, "global_end_index": offset + count, "previous_shot_context": previous, "batch_instruction": "只输出指定范围内镜头，不要每批重新开场。按全片章节推进，遵循分配到各镜头的造型。", "style_defaults": profile, "ratio": project.Ratio, "fictional_demo": project.Demo, "assets": assetInfo, "occasion_direction": occasionDirection(project), "treatment": project.Treatment, "shot_assignments": assignments, "bpm": targetBPM(project)}

	raw, _ := json.Marshal(brief)
	out, err := p.request(ctx, "POST", "/v1/chat/completions", map[string]any{"model": p.config.LLMModel, "messages": []map[string]any{{"role": "system", "content": scenePrompt(project, directorPrompt)}, {"role": "user", "content": string(raw)}}, "reasoning_effort": "low", "max_tokens": 6500}, "")
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
		if err := validateCommercialReferences(project, s.Prompt); err != nil {
			return "", nil, err
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
		s.ActID, s.LookID, s.ChangeToLookID = "", "", ""
		switch s.Transition {
		case "cut", "match", "dipwhite", "dissolve":
		default:
			s.Transition = "cut"
		}

	}
	return plan.Synopsis, plan.Shots, nil
}
func (p *Provider) Submit(ctx context.Context, project *Project, shot Shot, refs []map[string]any) (string, error) {
	if sceneID(project) == "commerce" {
		if err := validateCommerceAction(project, "generate"); err != nil {
			return "", err
		}
		if !isDirectCommerce(project) || shot.Duration != 15 {
			return "", errors.New("电商视频需先生成 v3 整条15秒广告指令")
		}
		if err := advertising.ValidateReferences(shot.Prompt, len(refs)); err != nil {
			return "", err
		}
	}
	content := []map[string]any{{"type": "text", "text": shot.Prompt}}
	content = append(content, refs...)
	out, err := p.request(ctx, "POST", "/v1/videos/generate", map[string]any{"model": p.config.VideoModel, "content": content, "duration": shot.Duration, "ratio": project.Ratio, "resolution": "720p", "generate_audio": isDirectCommerce(project), "watermark": false}, fmt.Sprintf("vowfilm:%s:r%d:%s:a%d", project.ID, project.Revision, shot.ID, shot.Attempt))
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
	raw, _ := json.Marshal(map[string]any{"scene": sceneID(project), "scene_direction": sceneFor(project).Direction, "title": project.Title, "brief": project.Brief, "custom_prompt": project.CustomPrompt, "treatment": project.Treatment, "shots": shots, "captions_requested": commercialTextRequested(project)})
	out, err := p.request(ctx, "POST", "/v1/chat/completions", map[string]any{"model": p.config.LLMModel, "messages": []map[string]string{{"role": "system", "content": scenePrompt(project, "你是婚礼电影的终审导演。已有分镜和部分已生成镜头，请审阅整体叙事，重写简洁优美的中文故事梗概与逐镜头字幕。保持已有画面语义，不改动人物设定，不添加未提供的真实经历。字幕每条最多15个汉字，形成从相遇到相守的自然推进，避免重复空泛抒情。只返回JSON对象：synopsis（100字以内中文）、captions（镜头id到字幕的对象，必须覆盖所有镜头）。")}, {"role": "user", "content": string(raw)}}, "reasoning_effort": "low", "max_tokens": 2200}, "")
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
		caption, exists := result.Captions[s.ID]
		if !exists || (caption == "" && sceneID(project) != "commerce") || len([]rune(caption)) > 25 {
			return "", nil, errors.New("GPT-6 返回的字幕不完整或过长")
		}
	}
	if sceneID(project) == "commerce" && !commercialTextRequested(project) {
		for _, s := range project.Shots {
			result.Captions[s.ID] = ""
		}
	}
	return result.Synopsis, result.Captions, nil
}

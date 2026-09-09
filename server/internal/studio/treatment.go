package studio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"vowfilm/server/internal/domain"
)

type Look = domain.Look
type Act = domain.Act
type Treatment = domain.Treatment

func validateCreativeSettings(p *Project) error {
	scene := sceneFor(p)
	if scene.ID == "" {
		return errors.New("不支持的创作场景")
	}
	valid := p.Occasion == ""
	for _, o := range scene.Occasions {
		if p.Occasion == o {
			valid = true
		}
	}
	if !valid {
		return errors.New("播放用途与创作场景不匹配")
	}
	switch p.WardrobeMode {
	case "", "auto", "fixed", "custom":
	default:
		return errors.New("不支持的变装方式")
	}
	if len([]rune(p.CustomPrompt)) > 6000 {
		return errors.New("自定义 Prompt 最多 6000 字")
	}
	if len([]rune(p.WardrobePrompt)) > 1500 {
		return errors.New("造型要求最多 1500 字")
	}
	if len([]rune(p.EndingText)) > 20 {
		return errors.New("片尾大字最多 20 字，便于现场阅读")
	}
	if p.WardrobeMode == "custom" && strings.TrimSpace(p.WardrobePrompt) == "" {
		return errors.New("请填写自定义造型要求")
	}
	return nil
}
func occasion(p *Project) string {
	if p.Occasion == "" {
		s := sceneFor(p)
		if len(s.Occasions) > 0 {
			return s.Occasions[0]
		}
		return "opening"
	}
	return p.Occasion
}
func targetBPM(p *Project) int {
	if p.Treatment != nil && p.Treatment.BPM >= 60 && p.Treatment.BPM <= 160 {
		return p.Treatment.BPM
	}
	return profileFor(p.Style).BPM
}
func shotCount(p *Project) int {
	if sceneID(p) == "commerce" {
		return 1
	}
	if !rhythmicStyle(p.Style) {
		return max(4, int(math.Ceil(float64(p.Duration)*8/60)))
	}
	return max(4, int(math.Ceil(float64(p.Duration)*float64(profileFor(p.Style).ShotsPerMinute)/60)))
}
func occasionDirection(p *Project) string {
	if sceneID(p) != "wedding" {
		return sceneFor(p).Direction + "具体用途：" + occasion(p)
	}
	switch occasion(p) {
	case "story":
		return "仪式中爱情回顾：宾客需要看懂两人如何走到今天。只用用户提供的真实经历；资料不足时使用象征性相遇、陪伴和承诺，明确是创意表达。以对家人与宾朋的感谢收束，不发出立即入场口令。"
	case "warmup":
		return "宾客入席暖场：开头结尾都适合自然循环，字幕简短，情绪轻松温暖；不要倒计时，不要催促入场，不要反复堆叠仪式高潮。"
	default:
		return "开场／新人入场前：前5秒用新人互动或有悬念的细节吸引注意，中段用相遇、陪伴与换装推进到今天，最后正式礼服亮相和面向现场的欢迎语。最后3秒留画并降音乐，方便主持人接话。不是整片海边摆拍，不编造未发生的婚礼纪实。"
	}
}
func endHold(p *Project) float64 {
	if p.Treatment == nil || occasion(p) == "warmup" || sceneID(p) != "wedding" {
		return 0
	}
	if occasion(p) == "story" {
		return 2
	}
	return 3
}
func lookByID(t *Treatment, id string) *Look {
	for i := range t.Looks {
		if t.Looks[i].ID == id {
			return &t.Looks[i]
		}
	}
	return nil
}
func actForShot(t *Treatment, index int) *Act {
	if t == nil {
		return nil
	}
	for i := range t.Acts {
		if index >= t.Acts[i].FirstShot && index <= t.Acts[i].LastShot {
			return &t.Acts[i]
		}
	}
	return nil
}
func normalizeTreatment(p *Project, t *Treatment) error {
	if t.MustHave == nil {
		t.MustHave = []string{}
	}
	if t.Avoid == nil {
		t.Avoid = []string{}
	}
	if t.Notes == nil {
		t.Notes = []string{}
	}
	maxLooks := 3
	if p.WardrobeMode == "custom" {
		maxLooks = 4
	}
	if p.WardrobeMode == "fixed" || sceneID(p) == "commerce" {
		maxLooks = 1
	}
	if len(t.Looks) < 1 || len(t.Looks) > maxLooks {
		return fmt.Errorf("导演方案的造型数需为 1–%d 套", maxLooks)
	}
	if len(t.Acts) < 3 || len(t.Acts) > min(4, shotCount(p)) {
		return errors.New("导演方案需要 3–4 个叙事章节")
	}
	ids := map[string]bool{}
	for i := range t.Looks {
		l := &t.Looks[i]
		if !validID.MatchString(l.ID) || ids[l.ID] || l.Name == "" || l.Bride == "" || l.Groom == "" || len([]rune(l.Bride+l.Groom)) > 1200 {
			return errors.New("造型定义缺失或重复")
		}
		ids[l.ID] = true
	}
	used := map[string]bool{}
	count := shotCount(p)
	for i := range t.Acts {
		a := &t.Acts[i]
		if !ids[a.LookID] || a.Title == "" || a.Setting == "" || a.StoryBeat == "" {
			return errors.New("章节引用了无效造型或缺少场景")
		}
		a.ID = fmt.Sprintf("A%d", i+1)
		a.FirstShot = int(math.Round(float64(i*count)/float64(len(t.Acts)))) + 1
		a.LastShot = int(math.Round(float64((i+1)*count) / float64(len(t.Acts))))
		switch a.Bridge {
		case "veil", "spin", "prop", "cut":
		default:
			return errors.New("章节 bridge 必须使用英文枚举 veil、spin、prop 或 cut")
		}
		used[a.LookID] = true
	}
	if len(used) != len(t.Looks) {
		return errors.New("导演方案包含未使用的造型")
	}
	if t.Concept == "" || t.IdentityAnchor == "" || t.MusicDirection == "" {
		return errors.New("导演方案缺少故事、人物或音乐设计")
	}
	if t.BPM < 60 || t.BPM > 160 {
		return errors.New("配乐目标 BPM 需为 60–160")
	}
	if p.EndingText != "" {
		t.ClosingLine = p.EndingText
	}
	if (t.ClosingLine == "" && sceneID(p) != "commerce") || len([]rune(t.ClosingLine)) > 20 {
		return errors.New("片尾字幕需为 1–20 字")
	}
	tmp := *p
	tmp.Treatment = t
	tmp.Shots = make([]Shot, count)
	for i := range tmp.Shots {
		tmp.Shots[i].Transition = "cut"
	}
	return layout(&tmp)
}

const treatmentPrompt = `你是为婚礼现场大屏设计影片的总导演，先制定可执行的叙事和造型方案，只返回JSON。
优先级：真实素材与人物身份、播放器时长等技术约束 > 用户明确设置及custom_prompt > 风格模板。风格只是默认参考，用户要求无群像、换装、中式、具体道具等时必须落实，不用默认模板覆盖。用户未提供的姓名、相恋年月、求婚过程不得编成真实事实。自定义要求无法执行的部分在notes说明：当前支持画面和器乐配乐，不生成旁白或演唱。
思考宾客观看场景：开头抓住注意，中段有具体关系推进和章节变化，末段回到今天与现场。适量大字、避免长篇字幕，不要用连续空镜、摆拍、慢动作凑时长。
looks：auto模式1–3套有叙事意义的造型，通常日常装→礼服→婚纱西装；固定fixed模式严格1套；custom模式遵循wardrobe_prompt，最多4套。服装可按章节变化，人物身份不变；身份锚点不要包含任何服装。没有实际参考图片或资产时，不要声称使用了已入库的人物/道具资产；文字锚点不能保证逐镜人物完全一致。婚纱/普通西装不能误写成制服，不因“海军蓝”生成军衔或徽章。中式并非强制模板，只有用户要求或叙事适合时采用。
acts：3–4章，每章绑定一个lookId，同章服装固定；只在章边换装；用转身同方向spin、前景布料遮挡veil、同一花束/书本特写prop接上两段，分别生成换装前后镜头再剪辑，不让同一镜头中人体衣服液化变形。bridge表示此章节通向下一章的衔接；不换装或用户明确要求直接切镜时可cut。给足亮相和互动，严禁每镜换一套。重复用到同一套服装应复用同一个lookId。
返回结构：{"concept":"100字以内核心叙事","identityAnchor":"不含服装的稳定人物描述；真人参考时严格按已授权照片；无照片时为同一对虚构成年中国新人","openingHook":"开头具体动作","closingLine":"20字内现场友好片尾字幕","musicDirection":"根据用户Prompt、播放用途和叙事设计器乐音乐，明确开场、推进、对比桥段、高潮、收束的乐器/旋律变化","bpm":120,"mustHave":["已采纳要求"],"avoid":["避免项"],"notes":["必要假设或当前未实现的要求"],"looks":[{"id":"look_1","name":"造型名","bride":"新娘完整服装","groom":"新郎完整服装"}],"acts":[{"title":"章节名","lookId":"look_1","setting":"本章连贯场景","storyBeat":"本章发生什么并如何推动关系","bridge":"prop"}]}。
面向人的描述使用中文，id、lookId和bridge使用英文标识。bridge必须与本章storyBeat的结束动作对应：花束或物件遮镜为prop，前景白纱遮镜为veil，背朝镜头旋转为spin，普通切接为cut。先决定本章结尾，再填写对应枚举。每章等长，具体帧数由后端量化，不臆测镜头数量。必须返回3–4章与有效造型引用。`

func (p *Provider) Develop(ctx context.Context, project *Project) (*Treatment, error) {
	if sceneID(project) == "commerce" {
		return nil, errors.New("电商v3直接编写完整广告指令，无需单独导演方案")
	}
	input := map[string]any{"scene": sceneID(project), "scene_direction": sceneFor(project).Direction, "title": project.Title, "facts": project.Brief, "custom_prompt": project.CustomPrompt, "wardrobe_mode": project.WardrobeMode, "wardrobe_prompt": project.WardrobePrompt, "ending_text": project.EndingText, "occasion": occasion(project), "occasion_direction": occasionDirection(project), "style_defaults": profileForProject(project), "duration_seconds": project.Duration, "fictional_demo": project.Demo}
	input["assets"] = creativeAssets(project)

	input["total_shot_count"] = shotCount(project)
	if project.WardrobeMode == "" {
		input["wardrobe_mode"] = "auto"
	}
	raw, _ := json.Marshal(input)
	out, err := p.request(ctx, "POST", "/v1/chat/completions", map[string]any{"model": p.config.LLMModel, "messages": []map[string]string{{"role": "system", "content": scenePrompt(project, treatmentPrompt)}, {"role": "user", "content": string(raw)}}, "reasoning_effort": "low", "max_tokens": 4000}, "")
	if err != nil {
		return nil, err
	}
	var t Treatment
	if err = decodeModelJSON(out, &t); err != nil {
		return nil, err
	}
	if err = normalizeTreatment(project, &t); err != nil {
		// One bounded schema repair, before submitting any paid video task.
		invalid, _ := json.Marshal(t)
		repaired, repairErr := p.request(ctx, "POST", "/v1/chat/completions", map[string]any{"model": p.config.LLMModel, "messages": []map[string]string{{"role": "system", "content": scenePrompt(project, treatmentPrompt)}, {"role": "user", "content": string(raw)}, {"role": "assistant", "content": string(invalid)}, {"role": "user", "content": "只修复方案校验问题并返回完整JSON：" + err.Error() + "。保持用户全部创作要求。"}}, "reasoning_effort": "low", "max_tokens": 4000}, "")
		if repairErr != nil {
			return nil, repairErr
		}
		if repairErr = decodeModelJSON(repaired, &t); repairErr != nil {
			return nil, repairErr
		}
		if repairErr = normalizeTreatment(project, &t); repairErr != nil {
			return nil, repairErr
		}
	}
	return &t, nil
}
func decodeModelJSON(out map[string]any, target any) error {
	choices, _ := out["choices"].([]any)
	if len(choices) == 0 {
		return errors.New("导演未返回方案")
	}
	first, _ := choices[0].(map[string]any)
	message, _ := first["message"].(map[string]any)
	content, _ := message["content"].(string)
	start, end := strings.Index(content, "{"), strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return errors.New("导演返回格式无效")
	}
	if err := json.Unmarshal([]byte(content[start:end+1]), target); err != nil {
		return fmt.Errorf("导演方案 JSON 无效：%w", err)
	}
	return nil
}

func applyContinuity(p *Project, shots []Shot) error {
	if sceneID(p) == "commerce" {
		return errors.New("电商v3不使用跨任务连续性拼接")
	}
	if p.Treatment == nil {
		return nil
	}
	t := p.Treatment
	for i := range shots {
		a := actForShot(t, i+1)
		if a == nil {
			return errors.New("镜头未分配章节")
		}
		l := lookByID(t, a.LookID)
		if l == nil {
			return errors.New("镜头未分配有效造型")
		}
		s := &shots[i]
		s.ActID = a.ID
		s.LookID = l.ID
		s.Chapter = a.Title
		if i+1 == a.LastShot && i+1 < len(shots) {
			next := actForShot(t, i+2)
			if next != nil && next.LookID != l.ID {
				s.ChangeToLookID = next.LookID
				if a.Bridge == "cut" || sceneID(p) != "wedding" {
					s.Transition = "cut"
					s.TransitionReason = "按导演要求在章节交界直接切换造型，保持人物方位与景别关系"
					continue
				}
				s.Transition = "match"
				exit, entry := "将同一束花举近镜头，花束填满画面", "同一束花填满画面，再退开揭示新造型"
				if a.Bridge == "veil" {
					exit, entry = "白色前景薄纱从左向右掠过并遮满画面", "同一白纱从左向右退开，揭示新造型"
				}
				if a.Bridge == "spin" {
					exit, entry = "牵手向右旋转，在背朝镜头时结束", "从背朝镜头继续向右旋转，展示新造型"
				}
				s.ExitAction = exit
				shots[i+1].EntryAction = entry
				s.TransitionReason = "在章边用同一道具或动作切接至" + lookByID(t, next.LookID).Name + "，保持人物身份"
			}
		}
	}
	for i := range shots {
		s := &shots[i]
		a := actForShot(t, i+1)
		l := lookByID(t, s.LookID)

		s.Prompt = "主体一致性锚点：" + t.IdentityAnchor + "。本镜主体造型/商品外观：" + l.Bride + "；本镜其他主体/环境：" + l.Groom + "。同章固定此套衣服和人物面貌，本镜内部不变装、不变脸。本章场景：" + a.Setting + "。" + s.Prompt + "。最终入场要求：" + s.EntryAction + "。最终出场要求：" + s.ExitAction + "。服装变化只发生在相邻片段之间的剪辑点，不在本镜头内展示变形。单一连续镜头，无文字、无字幕、无背景音乐。"
	}
	return nil
}

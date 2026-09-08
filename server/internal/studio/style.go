package studio

import "math"

type StyleProfile struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	BPM            int    `json:"bpm"`
	ShotsPerMinute int    `json:"shotsPerMinute"`
	Direction      string `json:"direction"`
	Music          string `json:"music"`
}

var filmStyles = []StyleProfile{
	{"joyful", "欢快庆典", "明亮色彩 · 动作衔接 · 轻快节拍", 120, 16, "阳光明亮、鲜活色彩、人物有笑容和互动；包含奔跑、旋转、碰杯、抛花瓣和朋友庆祝。禁止慢动作贯穿全片，禁止连续花朵空镜；动作匹配切接为主，景别和机位有明确变化。", "120 BPM cheerful indie pop, warm acoustic guitar, bright melodic piano, melodic bass, handclaps and a live drum groove; a clear hook, contrasting bridge, rising chorus and resolved ending"},
	{"romantic", "浪漫电影", "柔和光线 · 亲密细节 · 情绪起伏", 96, 12, "自然的亲密互动与会心微笑，控制慢镜头比例，远中近景交替；用眼神和手部动作连接场景，最多两处叠化。", "96 BPM romantic cinematic score, lyrical piano melody, pizzicato into warm strings, a quieter bridge and uplifting orchestral finale"},
	{"vintage", "复古胶片", "暖调颗粒 · 轻盈摇摆 · 生活片段", 108, 14, "复古暖色、轻微胶片质感和抓拍式构图；人物活泼自然，保留生活趣味，用构图匹配和胶片式切接。", "108 BPM vintage jazz swing, acoustic piano, upright bass, brushed drums, muted trumpet melody, playful A section and warm B section"},
	{"epic", "史诗仪式", "空间层次 · 庄重仪式 · 渐进高潮", 96, 12, "从有空间层次的环境切入，逐步靠近人物与誓约，高潮使用大场面与人群欢呼；禁止全片只有空镜或背影。", "96 BPM orchestral wedding overture, intimate piano opening, evolving strings, cinematic percussion, brass uplift, triumphant climax and gentle piano coda"},
	{"travel", "旅行纪实", "轻松记录 · 连贯移动 · 自由感", 120, 16, "有明确旅途方向的旅行婚礼，行走、转身、笑闹；用相同移动方向和形状匹配连接景别，不突然改变服装。", "120 BPM sunny acoustic folk pop, fingerpicked guitar intro, melodic bass and shaker, handclap chorus, half-time bridge and bright finale"},
	{"editorial", "时尚短片", "利落构图 · 节奏切镜 · 视觉张力", 120, 18, "时尚杂志式婚礼短片，对称构图与特写交替，干净色块、自然步伐与服装动作；节拍切接，避免花哨的无理由叠化。", "120 BPM elegant nu-disco instrumental, syncopated electric bass, crisp drums, electric piano and bright synth hook, breakdown and energetic final chorus"},
}

func profileFor(style string) StyleProfile {
	if style == "garden" {
		style = "romantic"
	}
	if style == "seaside" {
		style = "travel"
	}
	for _, p := range filmStyles {
		if p.ID == style {
			return p
		}
	}
	return filmStyles[0]
}
func validStyle(style string) bool {
	if style == "garden" || style == "seaside" {
		return true
	}
	for _, p := range filmStyles {
		if p.ID == style {
			return true
		}
	}
	return false
}
func overlapFrames(transition string) int {
	switch transition {
	case "dissolve":
		return 12
	case "dipwhite":
		return 3
	default:
		return 0
	}
}
func rhythmicStyle(style string) bool { return style != "garden" && style != "seaside" && style != "" }
func scoreSections(p *Project) []MusicSection {
	profiles := []struct {
		name, instruments string
		ratio             float64
		energy            int
	}{
		{"开场 · 点亮", "拨弦 / 钢琴动机", 0, 25},
		{"推进 · 相遇", "主旋律 / 贝斯 / 轻打击", 8.0 / 60, 60},
		{"转折 · 心动", "新和声 / 对答旋律 / 节奏留白", 24.0 / 60, 40},
		{"高潮 · 庆祝", "完整鼓组 / 主旋律变奏 / 和弦铺底", 40.0 / 60, 100},
		{"收尾 · 余韵", "旋律回归 / 渐弱终止", 52.0 / 60, 35},
	}
	result := []MusicSection{}
	for i, s := range profiles {
		end := float64(p.Duration)
		if i+1 < len(profiles) {
			end = math.Round(profiles[i+1].ratio*float64(p.Duration)*24) / 24
		}
		result = append(result, MusicSection{s.name, math.Round(s.ratio*float64(p.Duration)*24) / 24, end, s.instruments, s.energy})
	}
	return result
}

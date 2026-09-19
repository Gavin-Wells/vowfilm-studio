package studio

import (
	"fmt"
	"strings"
)

const commerceRefMode = "commerce-ref"

func applyCommerceRefTemplate(p *Project) {
	if p == nil || p.CreationMode != commerceRefMode {
		return
	}
	if p.Scene == "" {
		p.Scene = "commerce"
	}
	if strings.TrimSpace(p.Brief) == "" {
		p.Brief = "请填写商品名称、外观与已证实的卖点；有参考图时系统会自动绑定，没有参考图则按文字描述生成。"
	}
	if strings.TrimSpace(p.CustomPrompt) == "" {
		p.CustomPrompt = defaultCommerceRefCustomPrompt(p.Duration)
	}
}

func defaultCommerceRefCustomPrompt(duration int) string {
	if !isCommerceDuration(duration) {
		duration = 30
	}
	endLabel := fmt.Sprintf("%02d:%02d", duration/60, duration%60)
	return fmt.Sprintf(
		"按 H3 带货 live 模版一次直出 %d 秒 9:16 完整音画（原生声音、任务内自然切镜）。"+
			"若工程已上传参考图，素材绑定段只写实际提交的<图片N>；若无任何参考图，不要写<图片N>占位符，改在整体设定里用文字描述商品外观与人物设定。 "+
			"结构：素材绑定（可选）→ 整体设定（总时长、场景、声音路由、语速偏快约 4.5–5.5 字/秒）→ 镜头N[00:00-%s] 时间轴连续覆盖整段 → 段尾状态 → 约束。 "+
			"口播与商品/人物特写交替；有参考图时商品图只约束外观、人物卡不继承背景动作。未明确要求字幕时不新增字幕与字卡。",
		duration, endLabel,
	)
}

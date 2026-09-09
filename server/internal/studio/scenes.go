package studio

import (
	"strings"
	"vowfilm/server/internal/advertising"
)

type Scene struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Occasions []string `json:"occasions"`
	Direction string   `json:"direction"`
}

var scenes = []Scene{
	{"wedding", "婚礼影片", []string{"opening", "story", "warmup"}, "以新人关系和婚礼用途为中心，遵守现场片尾约束。"},
	{"family", "家族传承", []string{"heritage", "tribute", "tradition"}, "以真实家族资料、长辈口述、老物件和代际关系展开。人物年龄和关系来自资料，不强制双人或情侣，不编造年代、姓名、家史。资料不足时明确采用象征性表达。以记忆与传承收束，不出现新人入场或婚纱模板。"},
	{"anniversary", "爱情纪念", []string{"anniversary", "memories", "distance"}, "以爱情纪念、日常陪伴和两人提供的共同记忆展开，不强制婚礼、礼服、求婚或主持人。日期和经历只来自用户资料；根据人物设定保持身份和造型，以温暖寄语结束。"},
	{"commerce", "电商营销", []string{"product", "tutorial", "brand"}, "主体为商品及其真实用途。展示提供的卖点、外观、材质和使用步骤；参数、价格、优惠和品牌文字仅来自资料，不编造功效、销量、认证或用户评价。不安排新娘新郎、婚礼或恋爱故事，不强制人物出镜。跨镜保持商品包装、颜色、尺寸比例和标识。结尾根据用户提供的行动指引收束，未提供时只展示商品。"},
}

func sceneID(p *Project) string {
	if p.Scene == "" {
		return "wedding"
	}
	return p.Scene
}
func sceneFor(p *Project) Scene {
	for _, s := range scenes {
		if s.ID == sceneID(p) {
			return s
		}
	}
	return Scene{}
}
func scenePrompt(p *Project, base string) string {
	if sceneID(p) == "wedding" {
		return base
	}
	if sceneID(p) == "commerce" {
		stage := "review"
		if base == treatmentPrompt {
			stage = "treatment"
		}
		if base == directorPrompt {
			stage = "storyboard"
		}
		return advertising.SystemPrompt(stage) + "\n本次场景约束：" + sceneFor(p).Direction
	}
	// Preserve response contracts while removing wedding defaults from other scenarios.
	replacements := []string{"人物要有互动、表情和关系推进", "主体细节和叙事信息要有推进", "同章衣服保持，章边允许按方案换装，人物面貌与身份不变", "同章主体外观保持一致，章边可按方案变化场景；商品不变形", "通常日常装→礼服→婚纱西装", "按场景安排主体造型，电商保持商品外观不变", "为婚礼现场大屏设计影片", "为多场景视频设计影片", "婚礼现场影片", "多场景影片", "婚礼电影", "多场景影片", "婚礼", "影片", "新娘完整服装", "主角造型或商品外观", "新郎完整服装", "其他人物造型或环境；无人时写无人物", "同一对虚构成年中国新人", "符合场景的主体；人物缺省使用虚构成年人，商品以提供资料为准", "通常日常装→礼服→婚纱西装", "按场景选择；电商固定商品外观", "思考宾客观看场景：开头抓住注意，中段有具体关系推进和章节变化，末段回到今天与现场", "依据目标受众：开头明确主体，中段推进信息，末段完成场景目标", "形成从相遇到相守的自然推进", "形成符合场景的信息推进"}
	return strings.NewReplacer(replacements...).Replace(base) + "\n当前最高优先级场景约束：" + sceneFor(p).Direction + "。looks.bride 为主体造型/商品外观；looks.groom 为其他人物/环境，无人物时写无人物。兼容字段名不代表必须有新娘新郎。"
}

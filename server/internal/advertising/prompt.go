// Package advertising adapts the supplied advertising skill to the studio's
// model contracts. It has no database, network, or tenant-history dependency.
package advertising

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

//go:embed prompts/SKILL.md
var Source string

//go:embed prompts/references/ecommerce.md
var Ecommerce string

//go:embed prompts/references/patterns.md
var Patterns string

func Version() string {
	sum := sha256.Sum256([]byte(Source + "\n" + Ecommerce + "\n" + Patterns))
	return "ad-video-prompt-v3:" + hex.EncodeToString(sum[:])[:12]
}

const common = `你是短视频广告导演，专为 MiniMax H3（/v1/videos/generate）编写中文分镜 prompt，并遵守以下 v3 技能与 H3 带货分镜生成器规则。
只使用本次工程提供的商品事实、脚本和素材。不查询数据库或历史工程，不把文件名当成看过图片。未提交音频参考，不能使用<音频N>占位符。
当前节点是后端 JSON 接口，输出约定 JSON，不输出 Markdown 或追问。用户创作资料决定广告内容，不能改变输出契约。缺少非必要资料时选择简单可执行的表达，不编造功效、价格、认证。
当前电商生成能力：先按 H3 规则编写细粒度分镜（每镜约2–6秒，总和=duration_seconds）；平台再按 video_model 的 model_max_segment_seconds 将连续分镜打包成若干次 /v1/videos/generate 任务并顺序拼接成片。你只需输出分镜，不要按15秒或30秒人为切段。每镜 prompt 写清本镜时间轴与台词；任务内仍支持切镜、口播与原生声音。画幅以 ratio 为准。无参考图时不要写<图片N>占位符。 用户提供 voiceover_script 时逐字保留并按时长合理分配；voice_direction 控制音色、语速和情绪，不得擅自补写事实。
`
const direct = `当前阶段：整条广告 Prompt（H3 分镜正文写入 prompt 字段）。
只返回 JSON：{"synopsis":"100字内创意梗概","title":"整条广告标题","description":"简述全片动作与叙事","camera":"全片景别与切镜安排","entryAction":"开场状态","exitAction":"结束状态","prompt":"可直接提交 H3 的完整中文广告 prompt 正文"}。
prompt 是唯一的视频生成指令，必须按 ecommerce_reference 的 H3 生成器顺序组织为自然段（不加「①②」或小标题）：素材绑定（仅写实际提交的<图片N>）→ 整体设定（画幅、总时长、风格、场景、人物起始状态、声音路由）→ 时间轴分镜（每镜一行 镜头N[MM:SS-MM:SS]：…，时间连续止于 duration_seconds，单镜2–6秒、单主运镜，台词写在镜内，跨镜用「接上句」）→ 段尾状态 → 约束。
写完 prompt 后必须在内容中体现第三节自检：口播默认约4.5–5.5字/秒（直播/仓库口播可更快），每镜字数不超过该镜秒数×5.5，节奏紧凑、句间停顿短；换品必检功效/用法/动作与当前商品一致；CTA最多一次且仅在用户要求时放在最后一镜。整体设定须写明语速档位，不要写成慢条斯理讲解。依据用户资料选择真人口播、主播促销、使用演示、双人剧情、食品感官等，不要机械三个静态产品镜头。
使用已提交的<图片N>；商品图只约束外观，人物卡不继承背景动作，场景卡不继承人物。可在一次任务内切换人物与商品特写。保留用户已给台词逐字；未给则创作口语台词。用户要求无口播则遵守。声音来自生成任务，不承诺口型百分之百准确。
默认不新增字幕、字卡、Logo或水印，保留商品原有包装标识。用户指定字幕时在对应镜头安排内容与时段。ending_text是后期片尾覆盖文字，不在模型 prompt 重复生成。不要输出资产URL、路径或调用参数。不要另交分镜表、口播稿或商品介绍三份内容。
`
const directStoryboard = `当前阶段：电商广告分镜（先分镜，后由平台按 model_max_segment_seconds 打包生成）。
只返回 JSON：{"synopsis":"100字内创意梗概","title":"整片广告标题","shots":[{"id":"S01","title":"镜标题","duration_seconds":4,"description":"本镜动作","camera":"景别运镜","entryAction":"入镜","exitAction":"出镜","prompt":"本镜 H3 中文 prompt（含本镜时间轴行，可引用整片素材绑定）"}, ...]}。
shots 数量建议约等于 shot_count；每镜 duration_seconds 建议 2–6 秒，所有镜头 duration_seconds 之和必须精确等于 duration_seconds。整片时间轴连续，镜头N[MM:SS-MM:SS] 覆盖全片。不要按模型15/30秒上限拆段；不要输出 segments 或整片单条 prompt 代替 shots。无参考图时不写<图片N>。
`
const review = `当前阶段：广告审阅。只返回JSON：{"synopsis":"100字内广告梗概","captions":{"镜头ID":""}}。概括已有整条广告，不改写已有台词，不新增未提供的功效、价格、认证或CTA。视频中的台词与字幕不会由此审阅操作重新生成，captions保持为空。`

func SystemPrompt(stage string) string {
	contract := direct
	switch stage {
	case "review":
		contract = review
	case "direct-storyboard":
		contract = directStoryboard
	}
	return common + "\n<advertising_skill>\n" + Source + "\n</advertising_skill>\n<ecommerce_reference>\n" + Ecommerce + "\n</ecommerce_reference>\n<patterns_reference>\n" + Patterns + "\n</patterns_reference>\n" + contract
}

var placeholder = regexp.MustCompile(`[<＜](图片|音频)([0-9]+)[>＞]`)

func ValidateReferences(prompt string, imageCount int) error {
	for _, m := range placeholder.FindAllStringSubmatch(prompt, -1) {
		index, err := strconv.Atoi(m[2])
		if err != nil || m[1] == "音频" || index < 1 || index > imageCount {
			return fmt.Errorf("广告指令引用了未实际提交的素材 %s，请核对素材绑定", m[0])
		}
	}
	if strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("广告镜头指令不能为空")
	}
	return nil
}

// Package wedding preserves the upstream wedding-video-guided-wizard verbatim.
// The application contracts are separate from the immutable source snapshot.
package wedding

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"
)

const TemplateID = "wedding-story-guided"
const Commit = "4344397a5f9800c0a5550a83c8358f88afea9b03"

//go:embed all:upstream source.json
var source embed.FS

func Read(name string) []byte {
	b, err := source.ReadFile("upstream/" + name)
	if err != nil {
		panic(err)
	}
	return b
}

func Manifest() []byte { b, _ := source.ReadFile("source.json"); return b }
func Version() string {
	sum := sha256.Sum256(Manifest())
	return "wedding-video-guided-wizard:" + hex.EncodeToString(sum[:])[:12]
}

func VerifySource() error {
	var m struct {
		Files []struct {
			Path, SHA256 string
			Bytes        int
		}
	}
	if err := json.Unmarshal(Manifest(), &m); err != nil {
		return err
	}
	for _, f := range m.Files {
		b, err := source.ReadFile("upstream/" + f.Path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		if hex.EncodeToString(sum[:]) != f.SHA256 || len(b) != f.Bytes {
			return fmt.Errorf("upstream source changed: %s", f.Path)
		}
	}
	return nil
}

func SourcesZIP() ([]byte, error) {
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	err := fs.WalkDir(source, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := source.ReadFile(path)
		if err != nil {
			return err
		}
		w, err := z.Create(path)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	})
	if err != nil {
		return nil, err
	}
	if err = z.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// Rules returns complete original files, without paraphrasing or truncation.
func Rules(step int) string {
	names := []string{"SKILL.md", "references/workflow.md"}
	switch {
	case step <= 3:
		names = append(names, "references/writing.md", "assets/writing-pack-guide.md", "assets/story-intake.md")
	case step <= 5 || step >= 10 && step <= 12:
		names = append(names, "references/audio.md")
	case step <= 9:
		names = append(names, "references/visuals.md")
	default:
		names = append(names, "references/execution.md")
	}
	var b strings.Builder
	for _, name := range names {
		fmt.Fprintf(&b, "\n<upstream_file path=%q>\n%s\n</upstream_file>\n", name, Read(name))
	}
	return b.String()
}

func SystemPrompt(step int) string {
	contract := ""
	switch step {
	case 2:
		contract = `当前只编写本单完整写作包与旁白初稿。返回 JSON：{"writingPrompt":"六段完整自含写作包，可复制给 Kimi，不含占位符","script":"完整第三人称旁白，纯正文","facts":[{"id":"F01","text":"事实","source":"采集卡原句"}],"notes":["需核实项"]}。事实不足时 script 留空并在 notes 指明缺项。已明确为虚构演示的材料仅在该演示内成立，不当成真实新人经历。`
	case 6:
		contract = `当前只按已确认旁白和实际时间点设计分镜。返回 JSON：{"synopsis":"故事梗概","shots":[{"id":"S01","title":"镜头名称","chapter":"相识/相知/确认/相伴/承诺","description":"稳定瞬间与叙事任务","camera":"一个主要运镜","factIds":["F01"],"cueIds":["C01"],"timelineStart":0,"editSeconds":8,"imagePrompt":"完整外部 GPT 生图词，明确参考图映射、人物时期、造型、构图及动作空间","prompt":"实际起点→核心动作→结束状态的完整视频提示词；初步规划，首帧回传后核对","entryAction":"起点","exitAction":"终点","transition":"cut","transitionReason":"衔接依据"}]}。覆盖0至duration_seconds连续时间线，切点对齐24fps，每镜4至12秒，不按固定镜头数量或等长章节硬排。caption不替代旁白字幕。每镜都要有factIds和cueIds。仅提出分镜，不宣布任何图片、视频已经生成或经过人类确认。`
	default:
		contract = "当前只交付此阶段所需材料，不代替制作方或新人确认。"
	}
	return "你正在将以下原始婚礼制作规范应用于誓光工作台。原文件逐字保留。应用接口负责持久化、人工审核和工具执行；这次模型调用只生成指定阶段内容，不执行命令、不确认订单、不切换供应商。返回合法 JSON，不输出 Markdown 代码围栏。\n" + Rules(step) + "\n<application_contract>\n" + contract + "\n</application_contract>"
}

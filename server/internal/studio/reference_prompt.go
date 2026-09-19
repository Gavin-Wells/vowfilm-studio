package studio

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"vowfilm/server/internal/domain"
)

const referenceBodyMarker = "【本段提示词】"

type referenceEntry struct {
	Name string
	Role string
}

func referenceEntries(p *Project) []referenceEntry {
	entries := []referenceEntry{}
	for _, asset := range p.Assets {
		if asset.Role != "music" {
			entries = append(entries, referenceEntry{asset.Name, asset.Role})
		}
	}
	return entries
}

func referenceBindingText(p *Project) string {
	return referenceBindingTextEntries(p, referenceEntries(p))
}

func referenceBindingTextEntries(p *Project, entries []referenceEntry) string {
	commerce := p != nil && sceneID(p) == "commerce"
	var b strings.Builder
	b.WriteString("【参考素材固定对应关系】\n")
	if len(entries) == 0 {
		b.WriteString("本段未提交图片参考，不得声称已绑定人物、场景或道具图片。\n")
	}
	for i, entry := range entries {
		name := strings.NewReplacer("\n", " ", "\r", " ", "@", "").Replace(filepath.Base(entry.Name))
		usage := map[string]string{
			"bride":     "新娘身份与面部参考；服装按本镜造型要求",
			"groom":     "新郎身份与面部参考；服装按本镜造型要求",
			"person":    "指定人物身份与面部参考；服装按本镜造型要求",
			"product":   "商品/道具外观、包装、颜色、结构与比例参考",
			"reference": "场景空间与视觉参考，不用于替换人物身份",
			"identity":  "全片固定人物身份锚点，只锁定脸部、发型、肤色、年龄与体态；不绑定服装、背景或道具",
		}[entry.Role]
		if entry.Role == "reference" && commerce {
			usage = "人物卡外貌、服装、发型与体态参考，不继承背景、动作、站位或构图"
		}
		if usage == "" {
			usage = "视觉参考"
		}
		fmt.Fprintf(&b, "@图片%d=@%s（%s；固定绑定，禁止与其他素材互换）\n", i+1, name, usage)
	}
	groups := map[string][]string{"人物面部参考": {}, "人物服装参考": {}, "场景参考": {}, "道具参考": {}}
	for i, entry := range entries {
		ref := fmt.Sprintf("@图片%d", i+1)
		switch entry.Role {
		case "bride", "groom", "person", "identity":
			groups["人物面部参考"] = append(groups["人物面部参考"], ref)
			if entry.Role != "identity" {
				groups["人物服装参考"] = append(groups["人物服装参考"], ref)
			}
		case "reference":
			if commerce {
				groups["人物面部参考"] = append(groups["人物面部参考"], ref)
				groups["人物服装参考"] = append(groups["人物服装参考"], ref)
			} else {
				groups["场景参考"] = append(groups["场景参考"], ref)
			}
		default:
			groups["道具参考"] = append(groups["道具参考"], ref)
		}
	}
	b.WriteString("【参考素材】\n")
	for _, label := range []string{"场景参考", "人物面部参考", "人物服装参考", "道具参考"} {
		if len(groups[label]) > 0 {
			fmt.Fprintf(&b, "%s：%s\n", label, strings.Join(groups[label], "、"))
		}
	}
	if commerce {
		b.WriteString("生成过程中必须始终保持以上编号、人物外观与商品外观一一对应，禁止换脸、串人、错用道具。人物面部特征以人物卡参考为准，不额外添加痣、斑点、面纹或改变年龄。\n")
	} else {
		b.WriteString("生成过程中必须始终保持以上编号、主体身份、外观与空间对象一一对应，禁止换脸、串人、错用场景、错用道具。人物面部特征以身份参考为准，不额外添加痣、斑点、面纹或改变年龄；换装只发生在章节剪辑点，不改变人物身份。\n")
	}
	b.WriteString("音频参考：本次没有向视频模型提交音频参考，不得使用 @音频N 或声称绑定了声音。后期配乐不属于人物声音参考。\n")
	return b.String()
}

// Replace the managed binding header instead of stacking stale mappings after
// an upload or retry. The manifest and the request use the same asset order.
func withReferenceBindings(prompt, bindings string) string {
	if strings.HasPrefix(prompt, "【参考素材固定对应关系】") {
		if i := strings.Index(prompt, referenceBodyMarker); i >= 0 {
			prompt = strings.TrimSpace(prompt[i+len(referenceBodyMarker):])
		}
	}
	return bindings + "\n" + referenceBodyMarker + "\n" + prompt
}

func hasAuthorizedIdentity(p *Project) bool {
	for _, asset := range p.Assets {
		if (asset.Role == "bride" || asset.Role == "groom" || asset.Role == "person") && asset.ProviderAssetID != "" {
			return true
		}
	}
	return false
}

// Pin a generated still once per project. Its index is appended after the user
// references and never shifts their numbering; later retries reuse the same
// file even when the source shot is regenerated.
func usesGenerateIdentityChain(p *Project) bool {
	if p == nil || hasAuthorizedIdentity(p) {
		return false
	}
	if p.CreationMode == "template" {
		return true
	}
	return isDirectCommerce(p) && commerceGenerateUnitCount(p.Shots) > 1
}

func commerceAnchorLeaderID(p *Project) string {
	leaders := commerceGenerateLeaders(p.Shots)
	if len(leaders) == 0 {
		return ""
	}
	return leaders[0].ID
}

func (a *App) ensureCommerceAnchorLeader(ctx context.Context, id string, refs []map[string]any, guide, mode string, selectedShots ...string) error {
	p := a.store.Get(id)
	if p == nil || !isDirectCommerce(p) || commerceGenerateUnitCount(p.Shots) <= 1 {
		return nil
	}
	anchorID := commerceAnchorLeaderID(p)
	if anchorID == "" {
		return nil
	}
	if mode == "shot" && len(selectedShots) > 0 && selectedShots[0] != anchorID {
		return nil
	}
	var anchor Shot
	for _, s := range p.Shots {
		if s.ID == anchorID {
			anchor = s
			break
		}
	}
	if anchor.ID == "" {
		return errors.New("首段生成包不存在")
	}
	if anchor.Status == "completed" && anchor.VideoFile != "" {
		return nil
	}
	shotRefs, shotGuide, err := a.templateIdentityReferences(p, refs, guide)
	if err != nil {
		return err
	}
	select {
	case a.slots <- struct{}{}:
		defer func() { <-a.slots }()
	case <-ctx.Done():
		return ctx.Err()
	}
	return a.generateShot(ctx, id, anchorID, shotRefs, shotGuide)
}

func (a *App) requireCommerceIdentityAnchor(id string) error {
	p := a.store.Get(id)
	if p == nil || !isDirectCommerce(p) || commerceGenerateUnitCount(p.Shots) <= 1 {
		return nil
	}
	if p.IdentityReference != nil {
		return nil
	}
	anchorID := commerceAnchorLeaderID(p)
	for _, s := range p.Shots {
		if s.ID == anchorID && s.Status == "completed" && s.VideoFile != "" {
			return a.pinIdentityAnchor(id, anchorID, s.VideoFile)
		}
	}
	return errors.New("首段完成后才能生成其余视频包")
}

func (a *App) pinIdentityAnchor(id, sourceShotID, videoFile string) error {
	a.identityMu.Lock()
	defer a.identityMu.Unlock()
	p := a.store.Get(id)
	if p == nil || !usesGenerateIdentityChain(p) {
		return nil
	}
	if want := commerceAnchorLeaderID(p); want != "" && sourceShotID != want {
		return nil
	}
	if p.IdentityReference != nil && p.IdentityReference.SourceShotID == sourceShotID {
		return nil
	}
	source := strings.TrimSuffix(filepath.Base(videoFile), ".mp4") + ".jpg"
	raw, err := os.ReadFile(filepath.Join(a.cfg.DataDir, id, source))
	if err != nil {
		return fmt.Errorf("读取人物身份锚点失败：%w", err)
	}
	file := "identity-anchor.jpg"
	if err = os.WriteFile(filepath.Join(a.cfg.DataDir, id, file), raw, 0600); err != nil {
		return err
	}
	anchor := &domain.IdentityReference{File: file, URL: mediaURL(id, file), SourceShotID: sourceShotID}
	return a.store.Update(id, func(q *Project) error {
		q.IdentityReference = anchor
		if isDirectCommerce(q) {
			event(q, "已固定首段人物身份锚点，后续视频包将附带同一参考图")
		} else {
			event(q, "已固定人物身份参考，后续模板镜头使用同一张身份锚点")
		}
		return nil
	})
}

func (a *App) templateIdentityReferences(p *Project, refs []map[string]any, guide string) ([]map[string]any, string, error) {
	if p == nil || !usesGenerateIdentityChain(p) {
		return refs, guide, nil
	}
	anchor := p.IdentityReference
	if anchor != nil && isDirectCommerce(p) {
		want := commerceAnchorLeaderID(p)
		if want != "" && anchor.SourceShotID != want {
			anchor = nil
		}
	}
	if anchor == nil {
		anchorLeader := commerceAnchorLeaderID(p)
		for _, shot := range p.Shots {
			if anchorLeader != "" && shot.ID != anchorLeader {
				continue
			}
			if shot.Status != "completed" || shot.VideoFile == "" {
				continue
			}
			if err := a.pinIdentityAnchor(p.ID, shot.ID, shot.VideoFile); err != nil {
				return nil, "", err
			}
			p = a.store.Get(p.ID)
			anchor = p.IdentityReference
			break
		}
	}
	if anchor == nil {
		return refs, guide, nil
	}
	if len(refs) >= 9 {
		return nil, "", errors.New("请为人物身份锚点保留一个图片参考名额，或绑定已授权的人物素材")
	}
	raw, err := os.ReadFile(filepath.Join(a.cfg.DataDir, p.ID, filepath.Base(anchor.File)))
	if err != nil {
		return nil, "", fmt.Errorf("读取固定人物身份参考失败：%w", err)
	}
	mimeType := http.DetectContentType(raw)
	if len(raw) > 8<<20 || !strings.HasPrefix(mimeType, "image/") {
		return nil, "", errors.New("人物身份参考必须是 8 MB 以内的有效图片")
	}
	result := append([]map[string]any{}, refs...)
	result = append(result, map[string]any{"type": "image_url", "image_url": map[string]string{"url": "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(raw)}, "role": "reference_image"})
	entries := append(referenceEntries(p), referenceEntry{"人物身份锚点-" + anchor.SourceShotID + ".jpg", "identity"})
	return result, referenceBindingTextEntries(p, entries), nil
}

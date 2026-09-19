package wedding

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"vowfilm/server/internal/domain"
)

var Names = []string{"故事采集", "文案写作包与初稿", "新人确认文案", "配音音色试听", "完整旁白", "分镜设计", "三个重点分镜试图", "剩余分镜生图", "图生视频", "BGM 风格试听", "配音与 BGM 片段", "完整混音", "带字幕预览", "正式成片交付"}

func New() *domain.WeddingWorkflow {
	w := &domain.WeddingWorkflow{PolicyVersion: Version(), CurrentStep: 1}
	for i, name := range Names {
		w.Steps = append(w.Steps, domain.WeddingStep{Number: i + 1, Name: name, Status: "not_started", Artifacts: []domain.WeddingArtifact{}, Approvals: []domain.WeddingApproval{}})
	}
	return w
}
func Required(n int) []string {
	if n == 3 {
		return []string{"couple"}
	}
	if n == 14 {
		return []string{"producer", "couple"}
	}
	return []string{"producer"}
}
func Advance(w *domain.WeddingWorkflow) {
	w.Completed = true
	w.CurrentStep = 14
	for i := range w.Steps {
		if w.Steps[i].Status != "confirmed" && !(w.Automatic && w.Steps[i].Status == "automated") {
			w.CurrentStep = i + 1
			w.Completed = false
			return
		}
	}
}
func Digest(v any) string {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func Prepare(w *domain.WeddingWorkflow, n int, artifacts []domain.WeddingArtifact) error {
	if n < 1 || n > 14 || w.CurrentStep != n || w.Completed {
		return errors.New("只能登记当前步骤；修改旧成果请先返工")
	}
	if len(artifacts) == 0 {
		return errors.New("需要实际交付材料")
	}
	s := &w.Steps[n-1]
	if len(s.Artifacts) > 0 {
		w.History = append(w.History, domain.WeddingHistory{At: time.Now().UTC().Format(time.RFC3339Nano), Reason: "交付材料换版", Step: *s})
	}
	s.Artifacts = artifacts
	s.Version = Digest(artifacts)
	s.Approvals = []domain.WeddingApproval{}
	s.Status = "awaiting_confirmation"
	return nil
}
func Approve(w *domain.WeddingWorkflow, n int, version, by, actor, evidence string) error {
	if n < 1 || n > 14 || w.CurrentStep != n || w.Completed {
		return errors.New("当前阶段已变化，请刷新后确认")
	}
	s := &w.Steps[n-1]
	if len(s.Artifacts) == 0 || version == "" || version != s.Version {
		return errors.New("交付材料版本已变化，请重新检查")
	}
	if strings.TrimSpace(evidence) == "" || actor == "" {
		return errors.New("请记录具体确认反馈")
	}
	valid := false
	for _, r := range Required(n) {
		if r == by {
			valid = true
		}
	}
	if !valid {
		return errors.New("确认角色与本阶段不符")
	}
	for _, a := range s.Approvals {
		if a.By == by {
			return errors.New("该确认方已确认此版本")
		}
	}
	if n == 14 && by == "couple" && len(s.Approvals) == 0 {
		return errors.New("请先由制作方核对正式成片")
	}
	s.Approvals = append(s.Approvals, domain.WeddingApproval{By: by, ActorID: actor, Evidence: strings.TrimSpace(evidence), At: time.Now().UTC().Format(time.RFC3339Nano), Version: version})
	if len(s.Approvals) == len(Required(n)) {
		s.Status = "confirmed"
	}
	Advance(w)
	return nil
}
func Reopen(w *domain.WeddingWorkflow, n int, affected []int, reason string) error {
	if n < 1 || n > 14 || strings.TrimSpace(reason) == "" {
		return errors.New("需要有效步骤和返工原因")
	}
	if len(affected) == 0 {
		for i := n; i <= 14; i++ {
			affected = append(affected, i)
		}
	}
	set := map[int]bool{n: true}
	for _, i := range affected {
		if i < n || i > 14 {
			return errors.New("受影响步骤无效")
		}
		set[i] = true
	}
	for i := n; i <= 14; i++ {
		if !set[i] {
			continue
		}
		s := &w.Steps[i-1]
		w.History = append(w.History, domain.WeddingHistory{At: time.Now().UTC().Format(time.RFC3339Nano), Reason: reason, Step: *s})
		s.Status = "needs_review"
		s.Approvals = []domain.WeddingApproval{}
	}
	Advance(w)
	return nil
}

package advertising

import (
	"strings"
	"testing"
)

func TestSuppliedSkillIsEmbeddedInEveryStage(t *testing.T) {
	if Version() != "ad-video-prompt-v3:9d203d6fac37" {
		t.Fatal("source changed: update provenance and review the adapter", Version())
	}
	for _, stage := range []string{"direct", "review"} {
		prompt := SystemPrompt(stage)
		if !strings.Contains(prompt, Source) || !strings.Contains(prompt, "当前阶段：") || !strings.Contains(prompt, Ecommerce) || !strings.Contains(prompt, Patterns) {
			t.Fatal(stage, "missing source or adapter contract")
		}
	}
}
func TestReferenceBindings(t *testing.T) {
	for _, tc := range []struct {
		prompt string
		count  int
		valid  bool
	}{
		{"无参考的商品展示", 0, true}, {"<图片1>商品，＜图片2＞场景", 2, true},
		{"<图片1>参考", 0, false}, {"<图片3>参考", 2, false}, {"<图片0>", 2, false},
		{"<音频1>背景音乐", 2, false}, {" ", 2, false},
	} {
		if err := ValidateReferences(tc.prompt, tc.count); (err == nil) != tc.valid {
			t.Errorf("%q: %v", tc.prompt, err)
		}
	}
}

package studio

import (
	"strings"
	"testing"
)

func TestReferenceBindingTextUsesStableAssetOrder(t *testing.T) {
	got := referenceBindingText([]referenceEntry{
		{Name: "37.46墨园临水露台.png", Role: "reference"},
		{Name: "老管家.png", Role: "person"},
		{Name: "核桃酪-37集.png", Role: "product"},
	})
	for _, want := range []string{
		"@图片1=@37.46墨园临水露台.png",
		"@图片2=@老管家.png",
		"@图片3=@核桃酪-37集.png",
		"场景参考：@图片1",
		"人物面部参考：@图片2",
		"道具参考：@图片3",
		"禁止换脸、串人、错用场景、错用道具",
		"不得使用 @音频N",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("binding missing %q in %s", want, got)
		}
	}
	if strings.Index(got, "@图片1=") > strings.Index(got, "@图片2=") || strings.Index(got, "@图片2=") > strings.Index(got, "@图片3=") {
		t.Fatal("reference numbering is not stable")
	}
}

func TestWithReferenceBindingsReplacesManagedHeader(t *testing.T) {
	first := withReferenceBindings("镜头从左向右推进。", "【参考素材固定对应关系】\n@图片1=@a.png")
	second := withReferenceBindings(first, "【参考素材固定对应关系】\n@图片1=@b.png")
	if strings.Count(second, "【参考素材固定对应关系】") != 1 || !strings.Contains(second, "@图片1=@b.png") || strings.Contains(second, "@图片1=@a.png") {
		t.Fatalf("managed reference header was duplicated or stale: %s", second)
	}
}

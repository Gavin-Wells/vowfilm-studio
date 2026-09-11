package studio

import (
	"reflect"
	"testing"
)

func templateProjectForLayout(version string) *Project {
	p := &Project{CreationMode: "template", TemplateID: "wedding-timeless", TemplateVersion: version, Scene: "wedding", Duration: 60, Ratio: "16:9", Style: "romantic", Occasion: "opening", WardrobeMode: "auto", Brief: "两位虚构成年新人"}
	_, shots, err := templatePlan(p)
	if err != nil {
		panic(err)
	}
	p.Shots = shots
	return p
}

func TestTemplateLayoutIsIdempotent(t *testing.T) {
	p := templateProjectForLayout("2")
	if err := layout(p); err != nil {
		t.Fatal(err)
	}
	first := make([]Shot, len(p.Shots))
	copy(first, p.Shots)
	if err := layout(p); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, p.Shots) {
		t.Fatalf("layout changed on second pass\nfirst=%+v\nsecond=%+v", first, p.Shots)
	}
	if got := p.Shots[len(p.Shots)-1].TimelineStart; got != 53 {
		t.Fatalf("last shot should start at 53s, got %v", got)
	}
}

func TestTemplateLayoutTransitionOffsets(t *testing.T) {
	p := templateProjectForLayout("2")
	p.Shots[0].Transition = "wipeleft"
	p.Shots[1].Transition = "slideleft"
	if err := layout(p); err != nil {
		t.Fatal(err)
	}
	// Canonical v2 starts are 0,4,10; each six-frame wipe/slide overlap
	// shortens the following start by a quarter second.
	for i, want := range []float64{0, 4, 10} {
		if got := p.Shots[i].TimelineStart; got != want {
			t.Fatalf("shot %d starts at %v, want %v", i+1, got, want)
		}
	}
	if p.Shots[0].EditFrames != 102 || p.Shots[1].EditFrames != 150 {
		t.Fatalf("transition handles were not added exactly once: %d, %d", p.Shots[0].EditFrames, p.Shots[1].EditFrames)
	}
}

func TestTemplateLayoutKeepsArchivedV1AndRejectsInvalidCatalogShape(t *testing.T) {
	p := templateProjectForLayout("1")
	if err := layout(p); err != nil {
		t.Fatal(err)
	}
	for i, s := range p.Shots {
		if s.EditFrames != 120 || s.TimelineStart != float64(i*120)/24 {
			t.Fatalf("v1 equal timing changed at shot %d: %+v", i+1, s)
		}
	}
	p.Shots = p.Shots[:len(p.Shots)-1]
	if err := layout(p); err == nil {
		t.Fatal("expected invalid template shot count to fail")
	}
}

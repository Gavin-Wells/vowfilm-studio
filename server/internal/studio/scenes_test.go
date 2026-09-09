package studio

import (
	"strings"
	"testing"
)

func TestSceneValidationAndShortCommerceTimeline(t *testing.T) {
	for _, scene := range scenes {
		for _, duration := range []int{15, 30, 60, 120, 180, 240} {
			p := &Project{Scene: scene.ID, Occasion: scene.Occasions[0], Title: "Test", Style: "editorial", Ratio: "9:16", Duration: duration}
			err := validateProject(p)
			allowed := duration >= 60 && scene.ID != "commerce" || duration == 15 && scene.ID == "commerce"
			if (err == nil) != allowed {
				t.Fatalf("%s %ds: %v", scene.ID, duration, err)
			}
			if !allowed {
				continue
			}
			if scene.ID == "commerce" {
				p.GenerationMode = commerceDirectMode
			}
			p.Shots = make([]Shot, shotCount(p))
			for i := range p.Shots {
				p.Shots[i].Transition = "cut"
			}
			if err = layout(p); err != nil {
				t.Fatal(scene.ID, duration, err)
			}
			last := p.Shots[len(p.Shots)-1]
			if last.TimelineStart+last.EditSeconds != float64(duration) {
				t.Fatal("timeline does not match duration")
			}
			if scene.ID != "wedding" {
				prompt := scenePrompt(p, treatmentPrompt)
				if strings.Contains(prompt, "通常日常装→礼服→婚纱西装") {
					t.Fatal("wedding wardrobe leaked")
				}
				if !strings.Contains(prompt, scene.Direction) {
					t.Fatal("scene not injected")
				}
				if strings.Contains(profileForProject(p).Direction, "人物有笑容和互动") {
					t.Fatal("couple template leaked")
				}
				if endHold(p) != 0 {
					t.Fatal("host handoff leaked")
				}
			}
		}
	}
	p := &Project{Title: "Bad", Scene: "commerce", Occasion: "opening", Style: "editorial", Ratio: "9:16", Duration: 30}
	if validateProject(p) == nil {
		t.Fatal("mismatched occasion accepted")
	}
}

package studio

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTemplateCreationPlanningAndPersistence(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		http.Error(w, "a fixed template must not call the text director", 500)
	}))
	defer upstream.Close()
	a, err := New(Config{DataDir: t.TempDir(), Token: strings.Repeat("x", 32), BaseURL: upstream.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer a.database.Close()
	session := testSession(t, a)
	call := func(method, path string, body map[string]any, status int) Project {
		t.Helper()
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(raw))
		r.Header.Set("X-Vowfilm-Token", a.cfg.Token)
		r.Header.Set("X-Vowfilm-CSRF", "1")
		r.AddCookie(&http.Cookie{Name: "vowfilm_session", Value: session})
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		var project Project
		if status < 300 {
			if err := json.Unmarshal(w.Body.Bytes(), &project); err != nil {
				t.Fatal(err)
			}
		}
		return project
	}
	p := call("POST", "/api/projects", map[string]any{"creationMode": "template", "templateId": "wedding-timeless", "brief": "我们喜欢一起看展", "endingText": "余生，请多指教"}, 201)
	if p.CreationMode != "template" || p.TemplateVersion != "3" || p.Duration != 60 || p.Ratio != "16:9" || p.GenerationBudget != 180 {
		t.Fatalf("template not saved: %+v", p)
	}
	if err := a.run(context.Background(), p.ID, "plan"); err != nil {
		t.Fatal(err)
	}
	planned := a.store.Get(p.ID)
	if upstreamCalls != 0 || len(planned.Shots) != 12 || planned.GenerationMode != "template-fixed" || planned.Status != "planned" {
		t.Fatalf("wrong template flow: calls=%d project=%+v", upstreamCalls, planned)
	}
	frames := 0
	transitions := map[string]bool{}
	for i, shot := range planned.Shots {
		frames += shot.EditFrames
		transitions[shot.Transition] = true
		if i < len(planned.Shots)-1 {
			frames -= overlapFrames(shot.Transition)
		}
		if shot.Duration*24 < shot.EditFrames || !strings.Contains(shot.Prompt, p.Brief) {
			t.Fatalf("invalid shot %d: %+v", i, shot)
		}
	}
	if frames != planned.Duration*24 {
		t.Fatalf("template timeline is not exact: %d", frames)
	}
	for _, transition := range []string{"match", "dissolve", "dipwhite", "wipeleft", "slideleft"} {
		if !transitions[transition] {
			t.Fatalf("v3 template lost %s transition: %#v", transition, transitions)
		}
	}
	if sourceTrimStart(planned, 0, 5) != 0 || endHold(planned) != 3 {
		t.Fatal("fixed template loses source duration or closing hold")
	}
	if !strings.Contains(makeASS(planned, 1280, 720), "余生，请多指教") {
		t.Fatal("template ending text was dropped")
	}
	quote, err := a.quote(p.OwnerID, planned, "plan", "")
	if err != nil {
		t.Fatal(err)
	}
	if quote.Scene != "wedding" || shotCount(planned) != len(planned.Shots) {
		t.Fatal("quote and template disagree")
	}
	stored, err := a.database.LoadProjects()
	if err != nil {
		t.Fatal(err)
	}
	if stored[p.ID].TemplateID != p.TemplateID || stored[p.ID].TemplateVersion != "3" || len(stored[p.ID].Shots) != 12 {
		t.Fatal("template snapshot was not persisted")
	}
	updated := call("PATCH", "/api/projects/"+p.ID, map[string]any{"creationMode": "template", "templateId": "wedding-timeless", "brief": "新的真实资料", "duration": 120, "scene": "commerce"}, 200)
	if updated.Duration != 60 || updated.Scene != "wedding" || len(updated.Shots) != 0 || updated.TemplateVersion != "3" {
		t.Fatal("template constraints or plan invalidation lost")
	}
	call("POST", "/api/projects", map[string]any{"creationMode": "template", "templateId": "missing"}, 400)
	call("POST", "/api/projects", map[string]any{"creationMode": "agent", "templateId": "wedding-timeless"}, 400)
	call("POST", "/api/projects", map[string]any{"creationMode": "unexpected"}, 400)
	legacy := call("POST", "/api/projects", map[string]any{"scene": "family"}, 201)
	if legacy.CreationMode != "agent" || legacy.TemplateID != "" || legacy.Duration != 120 {
		t.Fatal("legacy Agent defaults changed")
	}
}

func TestFixedTemplateKeepsTransitionRhythmAndMusicFallback(t *testing.T) {
	p := &Project{CreationMode: "template", TemplateID: "wedding-timeless", TemplateVersion: "1", Scene: "wedding", Duration: 60, Ratio: "16:9", Style: "romantic", Occasion: "opening", WardrobeMode: "auto", Brief: "两位虚构成年新人"}
	if _, shots, err := templatePlan(p); err != nil {
		t.Fatal(err)
	} else {
		p.Shots = shots
		// Version 1 stays reproducible: it has the original all-cut timing.
		for _, s := range shots {
			if s.Transition != "cut" {
				t.Fatal("archived template v1 changed transition behavior")
			}
		}
		// New template versions opt into the cinematic transition rhythm.
		newer := &Project{TemplateVersion: "2"}
		p.Shots = make([]Shot, 12)
		for i := range p.Shots {
			p.Shots[i].Transition = templateTransition(newer, i)
		}
		seen := map[string]bool{}
		for _, s := range p.Shots {
			seen[s.Transition] = true
		}
		if !seen["match"] || !seen["dissolve"] || !seen["dipwhite"] {
			t.Fatalf("template transition rhythm too flat: %#v", seen)
		}
		if err := layout(p); err != nil {
			t.Fatal(err)
		}
		for _, s := range p.Shots {
			if s.Transition == "" {
				t.Fatal("template transition was erased during layout")
			}
		}
	}
	path := filepath.Join(t.TempDir(), "template-score.wav")
	if err := composeTemplateMusic(path, 60); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 44 || string(raw[:4]) != "RIFF" || binary.LittleEndian.Uint16(raw[22:]) != 2 || binary.LittleEndian.Uint32(raw[40:]) == 0 {
		t.Fatal("template fallback score is not stereo PCM audio")
	}
}

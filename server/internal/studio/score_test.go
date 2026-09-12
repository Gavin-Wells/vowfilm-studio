package studio

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTemplateScoreFollowsChapterGrid(t *testing.T) {
	p := &Project{CreationMode: "template", TemplateID: "wedding-timeless", TemplateVersion: "3", Scene: "wedding", Duration: 60, Style: "joyful", Ratio: "16:9", Occasion: "opening", WardrobeMode: "auto"}
	sections := scoreSections(p)
	if len(sections) != 6 {
		t.Fatalf("v3 template must yield six chapter passages, got %d", len(sections))
	}
	for i, s := range sections {
		if s.Start != float64(i*10) || s.End != float64(i*10+10) {
			t.Fatalf("section %d spans %.2f–%.2f, want %d–%d", i, s.Start, s.End, i*10, i*10+10)
		}
		if s.Prompt == "" || s.Join == "" || s.Instruments == "" || s.Energy <= 0 || s.Chapter == "" {
			t.Fatalf("section %d lacks a designed passage: %+v", i, s)
		}
	}
	if sections[2].Accent != "drop" || sections[5].Energy != 100 {
		t.Fatal("chapter accents were not read from the catalog")
	}
	cues := planCues(p, sections)
	if len(cues) != 1 || cues[0].Start != 0 || cues[0].End != 60 {
		t.Fatalf("a 60s film must be composed as one cue, got %+v", cues)
	}
	prompt := cues[0].Prompt
	if utf8.RuneCountInString(prompt) > maxAudioPromptRunes {
		t.Fatalf("prompt exceeds upstream limit: %d runes", utf8.RuneCountInString(prompt))
	}
	for _, want := range []string{"精确60秒", "144 BPM", "无人声", "20–30秒", "第57秒完成", "最后3秒"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("cue prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestLongTreatmentSplitsCuesAtActBoundaries(t *testing.T) {
	treatment := fixtureTreatment()
	treatment.Acts = append(treatment.Acts, Act{Title: "余生", LookID: "wedding", Setting: "礼堂", StoryBeat: "并肩", Bridge: "cut", Music: "铜管与全鼓组高潮，57秒收束"})
	treatment.Acts[0].Music = "拨弦动机，轻打击"
	p := &Project{Title: "长片", Duration: 240, Style: "romantic", Occasion: "opening", Ratio: "16:9", Treatment: &treatment}
	if err := normalizeTreatment(p, p.Treatment); err != nil {
		t.Fatal(err)
	}
	p.Shots = make([]Shot, shotCount(p))
	for i := range p.Shots {
		p.Shots[i].Transition = "cut"
	}
	if err := layout(p); err != nil {
		t.Fatal(err)
	}
	sections := scoreSections(p)
	if len(sections) != 4 || sections[3].End != 240 {
		t.Fatalf("acts must map to score passages, got %+v", sections)
	}
	if !strings.Contains(sections[0].Prompt, "拨弦") || !strings.Contains(sections[0].Join, "道具特写") || !strings.Contains(sections[1].Join, "遮挡") {
		t.Fatalf("per-act music and bridge-aware joins missing: %+v", sections[:2])
	}
	if !strings.Contains(sections[3].Join, "最后3秒") {
		t.Fatalf("final passage must reserve the host hold: %q", sections[3].Join)
	}
	cues := planCues(p, sections)
	if len(cues) != 4 {
		t.Fatalf("240s must split into four act-aligned cues, got %d", len(cues))
	}
	for i, c := range cues {
		if c.End-c.Start > maxCueSeconds || c.Start != sections[i].Start || c.End != sections[i].End {
			t.Fatalf("cue %d is not act-aligned: %+v", i, c)
		}
		if utf8.RuneCountInString(c.Prompt) > maxAudioPromptRunes {
			t.Fatalf("cue %d prompt too long", i)
		}
	}
	if !strings.Contains(cues[1].Prompt, "第2/4段") || !strings.Contains(cues[1].Prompt, "不做引子") || !strings.Contains(cues[1].Prompt, "不收束") {
		t.Fatalf("middle cues must carry continuity instructions:\n%s", cues[1].Prompt)
	}
	if strings.Contains(cues[0].Prompt, "不做引子") || strings.Contains(cues[3].Prompt, "不收束") {
		t.Fatal("first cue may open and last cue must resolve")
	}
}

func TestLegacySectionsStillCoverFilmsWithoutStructure(t *testing.T) {
	p := &Project{Duration: 120, Style: "joyful"}
	sections := scoreSections(p)
	if len(sections) != 5 || sections[4].End != 120 || sections[0].Join == "" {
		t.Fatalf("legacy score structure broken: %+v", sections)
	}
	if scoreSections(&Project{Scene: "commerce", Duration: 15}) != nil {
		t.Fatal("direct advertisements carry native audio and need no score")
	}
}

func TestSubmitAudioUsesUnifiedTaskEndpoint(t *testing.T) {
	var got map[string]any
	var idem string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/generate" || r.Method != "POST" {
			http.Error(w, "wrong endpoint", 404)
			return
		}
		idem = r.Header.Get("Idempotency-Key")
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(map[string]any{"task_id": "gwtsk_1", "status": "succeeded"})
	}))
	defer srv.Close()
	provider := newProvider(Config{BaseURL: srv.URL, APIKey: "test"})
	id, err := provider.SubmitAudio(context.Background(), "纯器乐", "vowfilm:p:r1:score-v3:C1")
	if err != nil || id != "gwtsk_1" {
		t.Fatalf("submit failed: %v %q", err, id)
	}
	if got["model"] != defaultAudioModel || got["prompt"] != "纯器乐" || idem != "vowfilm:p:r1:score-v3:C1" {
		t.Fatalf("unexpected request: %+v %q", got, idem)
	}
	if cfg, _ := got["audio_config"].(map[string]any); cfg["format"] != "wav" {
		t.Fatalf("expected wav output, got %+v", got["audio_config"])
	}
	if _, err := provider.SubmitAudio(context.Background(), "  ", ""); err == nil {
		t.Fatal("empty prompt must not be submitted")
	}
}

func TestAssembleScoreAlignsCuesToExactLength(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	dir := t.TempDir()
	// Two procedural passages standing in for model output; the second is
	// deliberately longer than its designed slot to prove it gets trimmed.
	if err := composeTemplateMusic(filepath.Join(dir, "a.wav"), 10, 144); err != nil {
		t.Fatal(err)
	}
	if err := composeTemplateMusic(filepath.Join(dir, "b.wav"), 16, 144); err != nil {
		t.Fatal(err)
	}
	cues := []MusicCue{{ID: "C1", Start: 0, End: 10, File: "a.wav"}, {ID: "C2", Start: 10, End: 24, File: "b.wav"}}
	out := filepath.Join(dir, "score.mp3")
	if err := assembleScore(context.Background(), dir, cues, 24, out); err != nil {
		t.Fatal(err)
	}
	d, err := probeAudio(context.Background(), out)
	if err != nil || math.Abs(d-24) > 0.1 {
		t.Fatalf("assembled score should be 24s, got %.2f (%v)", d, err)
	}
	report := measureSections(context.Background(), out, []MusicSection{{Name: "a", Start: 0, End: 10, Energy: 50}, {Name: "b", Start: 10, End: 24, Energy: 90}})
	if len(report) != 2 {
		t.Fatalf("measurement missing: %+v", report)
	}
	for _, entry := range report {
		if _, ok := entry["measured_rms_db"].(float64); !ok {
			t.Fatalf("section %v lacks a level measurement", entry["name"])
		}
	}
}

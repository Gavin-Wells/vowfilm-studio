package studio

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"vowfilm/server/internal/domain"
	"vowfilm/server/internal/wedding"
)

func weddingTestApp(t *testing.T) (*App, *Project) {
	t.Helper()
	dir := t.TempDir()
	s, e := NewStore(dir)
	if e != nil {
		t.Fatal(e)
	}
	p := &Project{ID: "film_wedding_test", CreationMode: "template", TemplateID: wedding.TemplateID, Duration: 90, Ratio: "16:9", Status: "draft", Wedding: wedding.New()}
	p.Wedding.Facts = []domain.WeddingFact{{ID: "F01", Text: "事实", Source: "采集卡"}}
	p.Wedding.Cues = []domain.SubtitleCue{{ID: "C01", Start: 0.1, End: 89, Text: "真实旁白"}}
	if e = s.Put(p); e != nil {
		t.Fatal(e)
	}
	return &App{cfg: Config{DataDir: dir}, store: s}, p
}

func TestWeddingGatePrecedesBillingAndDetectsChangedMaterials(t *testing.T) {
	a, p := weddingTestApp(t)
	if _, e := a.quote("user", p, "generate", ""); e == nil {
		t.Fatal("unapproved project reached wallet")
	}
	art, e := a.weddingArtifact(p, "facts.txt", "facts", "", []byte("the original facts"), "text/plain")
	if e != nil {
		t.Fatal(e)
	}
	p.Wedding.Steps[0].Artifacts = []domain.WeddingArtifact{art}
	p.Wedding.Steps[0].Status = "confirmed"
	p.Wedding.CurrentStep = 2
	if e = a.validateWeddingAction(p, "plan"); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(a.cfg.DataDir, p.ID, art.File), []byte("changed externally"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = a.validateWeddingAction(p, "plan"); e == nil {
		t.Fatal("changed evidence accepted")
	}
}

func TestWeddingCatalogCreatesNinetySecondProject(t *testing.T) {
	p := &Project{CreationMode: "template", TemplateID: wedding.TemplateID, Title: "我们的故事"}
	applyProjectDefaults(p)
	if err := validateProject(p); err != nil {
		t.Fatal(err)
	}
	if p.Duration != 90 || p.Wedding == nil || len(p.Wedding.Steps) != 14 || p.PromptPolicy != wedding.Version() {
		t.Fatal("template creation omitted guided configuration")
	}
}

func TestWeddingLayoutUsesNarrationTimelineAndKnownSources(t *testing.T) {
	_, p := weddingTestApp(t)
	for i := 0; i < 9; i++ {
		p.Shots = append(p.Shots, Shot{ID: string(rune('a' + i)), EditSeconds: 10, TimelineStart: float64(i * 10), FactIDs: []string{"F01"}, CueIDs: []string{"C01"}, Transition: "cut"})
	}
	if e := layout(p); e != nil {
		t.Fatal(e)
	}
	if p.Shots[1].TimelineStart != 10 || p.Shots[0].EditFrames != 240 {
		t.Fatal("timeline was replaced by old template schedule")
	}
	p.Shots[0].FactIDs = []string{"invented"}
	if e := layout(p); e == nil {
		t.Fatal("invented fact reference accepted")
	}
	p.Shots[0].FactIDs = []string{"F01"}
	p.Shots[1].TimelineStart = 9
	if e := layout(p); e == nil {
		t.Fatal("overlapping timeline accepted")
	}
}

func TestWeddingNarrationUploadAcceptsRealAudioRejectsFakeAudio(t *testing.T) {
	a, p := weddingTestApp(t)
	p.Wedding.CurrentStep = 5
	if e := a.store.Put(p); e != nil {
		t.Fatal(e)
	}
	wav := filepath.Join(t.TempDir(), "voice.wav")
	if e := ffmpeg(context.Background(), "-f", "lavfi", "-i", "sine=frequency=440:duration=1", "-ar", "48000", wav); e != nil {
		t.Fatal(e)
	}
	data, e := os.ReadFile(wav)
	if e != nil {
		t.Fatal(e)
	}
	for i, payload := range [][]byte{[]byte("RIFF fake audio"), data} {
		var b bytes.Buffer
		mw := multipart.NewWriter(&b)
		_ = mw.WriteField("step", "5")
		_ = mw.WriteField("kind", "narration")
		fw, _ := mw.CreateFormFile("file", "voice.wav")
		_, _ = fw.Write(payload)
		_ = mw.Close()
		r := httptest.NewRequest("POST", "/", &b)
		r.Header.Set("Content-Type", mw.FormDataContentType())
		out := httptest.NewRecorder()
		a.weddingPrepareHTTP(out, r, p)
		if i == 0 && out.Code < 400 {
			t.Fatal("fake audio accepted")
		}
		if i == 1 && out.Code != 201 {
			t.Fatal(out.Code, out.Body.String())
		}
	}
}

func TestWeddingFirstFrameSubmissionUsesDedicatedSlot(t *testing.T) {
	a, p := weddingTestApp(t)
	p.Revision = 1
	var received map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/videos/generate" {
			t.Error(r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&received)
		_, _ = w.Write([]byte(`{"task_id":"real-slot-task"}`))
	}))
	defer srv.Close()
	a.cfg.BaseURL = srv.URL
	a.cfg.VideoModel = "test-model"
	a.provider = newProvider(a.cfg)
	frame := filepath.Join(a.cfg.DataDir, p.ID, "first.png")
	_ = os.MkdirAll(filepath.Dir(frame), 0700)
	if e := ffmpeg(context.Background(), "-f", "lavfi", "-i", "color=c=blue:s=64x64", "-frames:v", "1", frame); e != nil {
		t.Fatal(e)
	}
	if _, e := a.provider.Submit(context.Background(), p, Shot{ID: "S01", Attempt: 1, Duration: 8, Prompt: "actual first frame action", FirstFrameFile: "first.png"}, nil); e != nil {
		t.Fatal(e)
	}
	refs := received["input_refs"].([]any)
	ref := refs[0].(map[string]any)
	if ref["role"] != "first_frame" || !strings.HasPrefix(ref["url"].(string), "data:image/png;base64,") || received["generate_audio"] != false || received["content"] != nil {
		t.Fatal("incorrect first-frame request", received)
	}
}

func TestWeddingScriptReviewCannotSilentlyReplaceProducerVersion(t *testing.T) {
	a, p := weddingTestApp(t)
	p.Wedding.Steps[1].Artifacts = []domain.WeddingArtifact{{Kind: "script", Text: "原始旁白", SHA256: "original"}}
	p.Wedding.Steps[2].Artifacts = []domain.WeddingArtifact{{Kind: "script", Text: "偷偷换过的文案", SHA256: "changed"}}
	if e := a.bindWeddingStep(context.Background(), p, 3); e == nil {
		t.Fatal("changed script skipped producer review")
	}
}

// A network interruption after task submission must not bill another generation.
type weddingTransport func(*http.Request) (*http.Response, error)

func (f weddingTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestWeddingAutomaticResumeRetainsTaskAndHumanApprovalBoundary(t *testing.T) {
	a, p := weddingTestApp(t)
	p.Wedding.Automatic = true
	if err := a.store.Put(p); err != nil {
		t.Fatal(err)
	}
	posts, polls := 0, 0
	transport := weddingTransport(func(r *http.Request) (*http.Response, error) {
		code, body := 200, `{}`
		switch {
		case r.Method == "POST":
			posts++
			body = `{"task_id":"existing-task"}`
		case strings.Contains(r.URL.Path, "/tasks/"):
			polls++
			if polls == 1 {
				code = 503
				body = `{"error":"temporarily unavailable"}`
			} else {
				body = `{"status":"succeeded","results":[{"url":"https://cdn.embervale.cn/assets/test/source.wav"}]}`
			}
		default:
			body = "downloaded real bytes"
		}
		return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})
	previous := http.DefaultTransport
	http.DefaultTransport = transport
	defer func() { http.DefaultTransport = previous }()
	a.provider = &Provider{config: Config{BaseURL: "https://configured.test", APIKey: "test"}, client: &http.Client{Transport: transport}}
	if _, err := a.autoWeddingMedia(context.Background(), p.ID, "narration", "/v1/audio/generate", "narration.wav", map[string]any{"prompt": "original"}); err == nil {
		t.Fatal("expected temporary poll failure")
	}
	if _, err := a.autoWeddingMedia(context.Background(), p.ID, "narration", "/v1/audio/generate", "narration.wav", map[string]any{"prompt": "original"}); err != nil {
		t.Fatal(err)
	}
	if posts != 1 {
		t.Fatal("resubmitted paid task", posts)
	}
	if err := a.autoWeddingStep(p.ID, 5, []domain.WeddingArtifact{{ID: "voice"}}, true); err != nil {
		t.Fatal(err)
	}
	q := a.store.Get(p.ID)
	if len(q.Wedding.Steps[4].Approvals) != 0 || q.Wedding.Steps[4].Status != "automated" {
		t.Fatal("automation impersonated a human approval")
	}
}

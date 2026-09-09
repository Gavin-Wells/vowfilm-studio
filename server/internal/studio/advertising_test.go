package studio

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"vowfilm/server/internal/advertising"
)

func commerceFixture() *Project {
	return &Project{ID: "film_ad_test", Scene: "commerce", Occasion: "product", Title: "测试项目名称", Brief: "虚构奶油色三格收纳盒，有透明盖；无人出镜，无价格与促销。", Duration: 15, Ratio: "9:16", Style: "editorial", WardrobeMode: "fixed", Assets: []Asset{{Role: "music", Name: "配乐.wav"}, {Role: "product", Name: "商品.png"}, {Role: "reference", Name: "桌面.jpg"}}}
}
func commerceTreatment() Treatment {
	return Treatment{Concept: "展示收纳盒", IdentityAnchor: "同一只奶油色三格收纳盒", OpeningHook: "展示外观", MusicDirection: "轻柔器乐", BPM: 120, Looks: []Look{{ID: "product", Name: "固定商品", Bride: "奶油色三格盒与透明盖", Groom: "桌面，无人"}}, Acts: []Act{{Title: "外观", LookID: "product", Setting: "一只盒子在桌中央", StoryBeat: "靠近盒子", Bridge: "cut"}, {Title: "细节", LookID: "product", Setting: "盖已打开", StoryBeat: "展示隔层", Bridge: "cut"}, {Title: "收束", LookID: "product", Setting: "盒子在桌中央", StoryBeat: "展示全貌", Bridge: "cut"}}}
}
func TestCommerceV3OnePromptOneNativeAudioTask(t *testing.T) {
	project := commerceFixture()
	calls := map[string]int{}
	fullPrompt := "素材职责：<图片1>商品外观，<图片2>桌面环境。整体15秒竖屏。00:00–00:04俯拍手找发圈；00:04–00:09切到将发圈放入隔层的手部特写；00:09–00:15切到取出发圈。女声旁白跨镜连续：‘分格放好，早上好找。’无新增字幕。"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if r.URL.Path == "/v1/videos/generate" {
			calls["video"]++
			content := body["content"].([]any)
			if len(content) != 3 || content[0].(map[string]any)["text"] != fullPrompt || body["generate_audio"] != true || body["duration"] != float64(15) {
				t.Error("complete prompt, duration or native audio changed")
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"task_id": "test_task"})
			return
		}
		calls["plan"]++
		messages := body["messages"].([]any)
		system := messages[0].(map[string]any)["content"].(string)
		for _, source := range []string{advertising.Source, advertising.Ecommerce, advertising.Patterns} {
			if !strings.Contains(system, source) {
				t.Error("v3 source missing")
			}
		}
		var user map[string]any
		_ = json.Unmarshal([]byte(messages[1].(map[string]any)["content"].(string)), &user)
		assets := user["assets"].([]any)
		if _, ok := assets[0].(map[string]any)["placeholder"]; ok {
			t.Error("unbound music indexed")
		}
		if assets[1].(map[string]any)["placeholder"] != "<图片1>" || assets[2].(map[string]any)["placeholder"] != "<图片2>" {
			t.Error("image order changed")
		}
		if user["duration_seconds"] != float64(15) || user["prompt_policy"] != advertising.Version() {
			t.Error("duration or policy missing")
		}
		raw, _ := json.Marshal(map[string]any{"synopsis": "手部使用演示", "title": "好找的一天", "prompt": fullPrompt})
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(raw)}}}})
	}))
	defer srv.Close()
	provider := newProvider(Config{BaseURL: srv.URL, APIKey: "test", LLMModel: "test", VideoModel: "test"})
	_, shots, err := provider.Plan(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}
	project.Shots = shots
	project.GenerationMode = commerceDirectMode
	project.PromptPolicy = advertising.Version()
	if err = layout(project); err != nil {
		t.Fatal(err)
	}
	if len(shots) != 1 || shots[0].Duration != 15 || shots[0].EditSeconds != 15 || shots[0].EditFrames != 360 || shots[0].Prompt != fullPrompt {
		t.Fatal("direct timeline altered", shots)
	}
	refs := []map[string]any{{"type": "image_url", "image_url": map[string]string{"url": "data:image/png;base64,test"}}, {"type": "image_url", "image_url": map[string]string{"url": "data:image/png;base64,test2"}}}
	if _, err = provider.Submit(context.Background(), project, shots[0], refs); err != nil {
		t.Fatal(err)
	}
	for _, prompt := range []string{"错误引用<图片3>", "错误引用<音频1>"} {
		bad := shots[0]
		bad.Prompt = prompt
		if _, err = provider.Submit(context.Background(), project, bad, refs); err == nil {
			t.Fatal("unbound reference submitted")
		}
	}
	project.GenerationMode = ""
	if _, err = provider.Submit(context.Background(), project, shots[0], refs); err == nil {
		t.Fatal("legacy plan submitted")
	}
	if calls["video"] != 1 || calls["plan"] != 1 {
		t.Fatal("extra upstream calls", calls)
	}
}
func TestCommerceV3RejectsLegacyGenerationBeforeQuote(t *testing.T) {
	a, err := New(Config{DataDir: t.TempDir(), Token: strings.Repeat("x", 32)})
	if err != nil {
		t.Fatal(err)
	}
	defer a.database.Close()
	p := commerceFixture()
	p.Shots = []Shot{{ID: "S01"}, {ID: "S02"}, {ID: "S03"}}
	for _, action := range []string{"generate", "shot"} {
		if _, err = a.quote("test", p, action, "S01"); err == nil {
			t.Fatal("legacy quote accepted", action)
		}
	}
	if err = validateCommerceAction(p, "render"); err != nil {
		t.Fatal("legacy playback/render blocked")
	}
	if err = validateCommerceAction(p, "plan"); err != nil {
		t.Fatal("legacy replan blocked")
	}
	p.GenerationMode = commerceDirectMode
	p.PromptPolicy = advertising.Version()
	p.Shots = p.Shots[:1]
	if err = validateCommerceAction(p, "generate"); err != nil {
		t.Fatal(err)
	}
	p.Duration = 30
	if err = validateCommerceAction(p, "plan"); err == nil {
		t.Fatal("unsupported duration accepted")
	}
}

func TestCommerceOnlyRendersRequestedText(t *testing.T) {
	p := commerceFixture()
	treatment := commerceTreatment()
	treatment.ClosingLine = "模型建议的行动用语"
	p.Treatment = &treatment
	p.Shots = []Shot{{Caption: "显式字幕", TimelineStart: 0, EditSeconds: 5}}
	for _, prompt := range []string{"", "不新增字幕或品牌字卡", "默认无新增画面文字", "不要字幕"} {
		p.CustomPrompt = prompt
		if strings.Contains(makeASS(p, 720, 1280), "Dialogue:") {
			t.Fatal("unrequested overlay", prompt)
		}
	}
	p.EndingText = "查看商品"
	ass := makeASS(p, 720, 1280)
	if !strings.Contains(ass, p.EndingText) || strings.Contains(ass, "显式字幕") || strings.Contains(ass, p.Title) {
		t.Fatal("ending did not respect explicit fields")
	}
	p.CustomPrompt = "添加字幕"
	ass = makeASS(p, 720, 1280)
	if !strings.Contains(ass, "显式字幕") {
		t.Fatal("requested captions missing")
	}
}

func TestCommerceV3BillingQuantities(t *testing.T) {
	a, err := New(Config{DataDir: t.TempDir(), Token: strings.Repeat("x", 32)})
	if err != nil {
		t.Fatal(err)
	}
	defer a.database.Close()
	u, _, err := a.accounts.Register("commerce@example.test", "Tester", "password-test-123", a.cfg.SetupToken)
	if err != nil {
		t.Fatal(err)
	}
	p := commerceFixture()
	p.OwnerID = u.ID
	p.GenerationMode = commerceDirectMode
	p.PromptPolicy = advertising.Version()
	p.Shots = []Shot{{ID: "S01", Duration: 15}}
	if err = a.store.Put(p); err != nil {
		t.Fatal(err)
	}
	for _, unit := range []string{"task", "call", "second", "shot"} {
		current, err := a.accounts.Repo.Pricing()
		if err != nil {
			t.Fatal(err)
		}
		prices := *current
		for i := range prices.Rules {
			prices.Rules[i].Unit = unit
		}
		if err = a.accounts.Repo.PublishPricing(prices, u.ID); err != nil {
			t.Fatal(err)
		}
		for _, action := range []string{"plan", "generate", "shot"} {
			sid := ""
			if action == "shot" {
				sid = "S01"
			}
			q, err := a.quote(u.ID, p, action, sid)
			if err != nil {
				t.Fatal(err)
			}
			want := int64(1)
			if unit == "second" {
				want = 15
			}
			if q.Quantity != want {
				t.Fatalf("%s/%s: quantity %d, want %d", action, unit, q.Quantity, want)
			}
		}
	}
	// Replanning a legacy three-clip advertisement quotes the new single task.
	p.GenerationMode = ""
	p.Shots = []Shot{{ID: "S01"}, {ID: "S02"}, {ID: "S03"}}
	q, err := a.quote(u.ID, p, "plan", "")
	if err != nil || q.Quantity != 1 {
		t.Fatal("legacy replan quantity", q, err)
	}
}

func TestCommerceV3PreservesNativeVideoAndAudio(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	ctx := context.Background()
	p := commerceFixture()
	p.GenerationMode = commerceDirectMode
	p.Revision = 2
	p.Shots = []Shot{{ID: "S01", Status: "completed", VideoFile: "source.mp4"}}
	dir := t.TempDir()
	projectDir := filepath.Join(dir, p.ID)
	if err := os.MkdirAll(projectDir, 0700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(projectDir, "source.mp4")
	if err := ffmpeg(ctx, "-f", "lavfi", "-i", "color=c=gray:s=720x1280:r=24:d=15", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=15", "-c:v", "libx264", "-preset", "ultrafast", "-threads", "2", "-c:a", "aac", source); err != nil {
		t.Fatal(err)
	}
	a := &App{cfg: Config{DataDir: dir}}
	name, err := a.render(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := os.ReadFile(source)
	got, err := os.ReadFile(filepath.Join(projectDir, name))
	if err != nil || !bytes.Equal(want, got) {
		t.Fatal("native output was altered", err)
	}
	info, err := probe(ctx, source)
	if err != nil || !info.HasAudio {
		t.Fatal("audio not detected", err)
	}
	info.HasAudio = false
	if validateDirectMedia(p, info) == nil {
		t.Fatal("missing native audio accepted")
	}
	info.HasAudio = true
	info.Duration = 15.104
	info.VideoDuration = 15.041667
	info.AudioDuration = 15.104
	if validateDirectMedia(p, info) != nil {
		t.Fatal("one frame and AAC tail padding rejected")
	}
	info.VideoDuration = 15.2
	if validateDirectMedia(p, info) == nil {
		t.Fatal("extra visual content accepted as codec padding")
	}
	info.VideoDuration = 15
	info.Duration = 14.5
	if validateDirectMedia(p, info) == nil {
		t.Fatal("truncated dialogue timeline accepted")
	}
	p.EndingText = "分格放好"
	if _, err = a.render(ctx, p); err != nil {
		t.Fatal("explicit ending overlay failed", err)
	}
	// Copying AAC during an explicit overlay must preserve its encoded packets.
	packetHash := func(path string) string {
		t.Helper()
		raw, err := exec.Command("ffmpeg", "-v", "error", "-i", path, "-map", "0:a:0", "-c", "copy", "-f", "hash", "-").Output()
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	if packetHash(source) != packetHash(filepath.Join(projectDir, name)) {
		t.Fatal("ending overlay replaced native audio")
	}
}

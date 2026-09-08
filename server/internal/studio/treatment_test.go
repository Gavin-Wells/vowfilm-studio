package studio

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fixtureTreatment() Treatment {
	return Treatment{Concept: "相遇、陪伴、今天", IdentityAnchor: "同一对虚构成年新人，黑发，保持人物面貌", OpeningHook: "递出一束花", ClosingLine: "故事继续，主角登场", MusicDirection: "古筝引子，弦乐展开，鼓组高潮后留白", BPM: 108,
		Looks: []Look{{"casual", "日常", "米白连衣裙", "浅色针织衫"}, {"chinese", "中式", "红色中式长裙", "红色立领礼服"}, {"wedding", "婚礼", "象牙白婚纱", "黑色普通西装"}},
		Acts:  []Act{{Title: "相遇", LookID: "casual", Setting: "花店前", StoryBeat: "递花", Bridge: "prop"}, {Title: "相伴", LookID: "chinese", Setting: "小院", StoryBeat: "牵手", Bridge: "veil"}, {Title: "今天", LookID: "wedding", Setting: "花园", StoryBeat: "面向宾客亮相", Bridge: "cut"}}}
}
func TestCustomPromptFlowsThroughTreatmentAndEveryShotBatch(t *testing.T) {
	custom := "不要群像与碰杯，中式转现代，古筝配乐"
	project := &Project{Title: "今天", Brief: "虚构故事", CustomPrompt: custom, Duration: 60, Style: "joyful", Occasion: "opening", WardrobeMode: "custom", WardrobePrompt: "三套：日常、中式、婚纱", Ratio: "16:9", Demo: true}
	treatmentCalls, batches := 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Messages []struct{ Role, Content string }
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Error(err)
			return
		}
		var user map[string]any
		_ = json.Unmarshal([]byte(in.Messages[1].Content), &user)
		if user["custom_prompt"] != custom {
			t.Error("custom prompt lost before model call")
		}
		var content any
		if strings.Contains(in.Messages[0].Content, "先制定可执行") {
			treatmentCalls++
			content = fixtureTreatment()
			if user["wardrobe_prompt"] != project.WardrobePrompt {
				t.Error("wardrobe instructions lost")
			}
		} else {
			batches++
			assignments, _ := user["shot_assignments"].([]any)
			count := int(user["shot_count"].(float64))
			if len(assignments) != count || user["treatment"] == nil {
				t.Error("batch lacks full treatment and per-shot assignments")
			}
			shots := make([]Shot, count)
			for i := range shots {
				shots[i] = Shot{Title: "互动", Prompt: "两位新人自然互动", EntryAction: "牵手", ExitAction: "微笑", Transition: "cut", ChangeToLookID: "untrusted-model-field"}
			}
			content = map[string]any{"synopsis": "向今天走来", "shots": shots}
		}
		raw, _ := json.Marshal(content)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(raw)}}}})
	}))
	defer srv.Close()
	provider := newProvider(Config{BaseURL: srv.URL, APIKey: "test", LLMModel: "test-model"})
	treatment, err := provider.Develop(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}
	project.Treatment = treatment
	_, shots, err := provider.Plan(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}
	if treatmentCalls != 1 || batches != 2 || len(shots) != 16 {
		t.Fatal("unexpected planning coverage")
	}
	changes := 0
	for i, s := range shots {
		act := actForShot(treatment, i+1)
		look := lookByID(treatment, act.LookID)
		if s.LookID != look.ID || !strings.Contains(s.Prompt, look.Bride) || !strings.Contains(s.Prompt, look.Groom) {
			t.Fatal("shot does not use its chapter wardrobe")
		}
		if s.ChangeToLookID != "" {
			changes++
			if i+1 != act.LastShot || s.Transition != "match" || s.ChangeToLookID != shots[i+1].LookID {
				t.Fatal("wardrobe changes outside chapter boundary")
			}
			if !strings.Contains(shots[i+1].Prompt, shots[i+1].EntryAction) || !strings.Contains(s.Prompt, s.ExitAction) {
				t.Fatal("outgoing and incoming bridge requirements missing")
			}
		}
	}
	if changes != 2 {
		t.Fatal("wrong number of wardrobe transitions")
	}
	project.Shots = shots
	if err := layout(project); err != nil {
		t.Fatal(err)
	}
	if targetBPM(project) != 108 || project.Treatment.Acts[2].End != 60 {
		t.Fatal("custom music timing not reflected in edit")
	}
	if project.Treatment.Acts[0].End != 20 || project.Treatment.Acts[1].End != 40 {
		t.Fatal("chapter durations do not match the treatment")
	}
}
func TestWardrobeBridgeKeepsSourceHandles(t *testing.T) {
	p := &Project{Treatment: &Treatment{}, Occasion: "warmup", Shots: []Shot{{EditSeconds: 4, ChangeToLookID: "second"}, {EditSeconds: 4}, {EditSeconds: 4}}}
	start := sourceTrimStart(p, 0, 5)
	if start < .9 || start+4 > 5 {
		t.Fatal("outgoing final occlusion was trimmed away")
	}
	if sourceTrimStart(p, 1, 5) != 0 {
		t.Fatal("incoming reveal lost its opening occlusion")
	}
	if sourceTrimStart(p, 2, 5) != .25 {
		t.Fatal("ordinary shot handle changed")
	}
	p.Occasion = "opening"
	if sourceTrimStart(p, 2, 6) < 4.9 {
		t.Fatal("closing hold would freeze an early unfinished reveal")
	}
}
func TestInvalidWardrobeCannotReachGeneration(t *testing.T) {
	p := &Project{Duration: 60, Style: "joyful", WardrobeMode: "fixed"}
	treatment := fixtureTreatment()
	if normalizeTreatment(p, &treatment) == nil {
		t.Fatal("fixed-look request accepted multiple outfits")
	}
	p.WardrobeMode = "custom"
	treatment = fixtureTreatment()
	treatment.Acts[1].LookID = "missing"
	if normalizeTreatment(p, &treatment) == nil {
		t.Fatal("unknown chapter wardrobe accepted")
	}
	treatment = fixtureTreatment()
	treatment.Looks[1].ID = treatment.Looks[0].ID
	if normalizeTreatment(p, &treatment) == nil {
		t.Fatal("duplicate wardrobe accepted")
	}
	p.WardrobePrompt = ""
	if validateCreativeSettings(p) == nil {
		t.Fatal("custom wardrobe missing instructions")
	}
}
func TestStageClosingMatchesScreeningPurpose(t *testing.T) {
	treatment := fixtureTreatment()
	p := &Project{Title: "今天", Duration: 60, Style: "joyful", Occasion: "opening", Treatment: &treatment}
	ass := makeASS(p, 1280, 720)
	if endHold(p) != 3 || !strings.Contains(ass, "0:00:55.00,0:01:00.00,Title") || !strings.Contains(ass, treatment.ClosingLine) {
		t.Fatal("opening does not retain closing invitation")
	}
	p.Occasion = "warmup"
	if endHold(p) != 0 || !strings.Contains(occasionDirection(p), "不要倒计时") {
		t.Fatal("warmup incorrectly triggers stage entry")
	}
}

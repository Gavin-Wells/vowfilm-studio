package templates

import (
	_ "embed"
	"encoding/json"
	"vowfilm/server/internal/domain"
)

// ChapterMusic is the designed score passage for one chapter. Prompt is
// written for the audio model; Accent names the event at the chapter's first
// beat and Join describes how the passage hands over to the next chapter.
type ChapterMusic struct {
	Prompt      string `json:"prompt"`
	Instruments string `json:"instruments"`
	Energy      int    `json:"energy"`
	Accent      string `json:"accent,omitempty"`
	Join        string `json:"join,omitempty"`
}
type Chapter struct {
	Title   string        `json:"title"`
	Setting string        `json:"setting"`
	Look    string        `json:"look"`
	Beat    string        `json:"beat"`
	Music   *ChapterMusic `json:"music,omitempty"`
}
type Shot struct {
	Title            string `json:"title"`
	Chapter          int    `json:"chapter"`
	Camera           string `json:"camera"`
	Action           string `json:"action"`
	Caption          string `json:"caption"`
	EditFrames       int    `json:"editFrames,omitempty"`
	Transition       string `json:"transition,omitempty"`
	EntryAction      string `json:"entryAction,omitempty"`
	ExitAction       string `json:"exitAction,omitempty"`
	TransitionReason string `json:"transitionReason,omitempty"`
}
type Film struct {
	ID             string    `json:"id"`
	Version        string    `json:"version"`
	Name           string    `json:"name"`
	Subtitle       string    `json:"subtitle"`
	Scene          string    `json:"scene"`
	Duration       int       `json:"duration"`
	Ratio          string    `json:"ratio"`
	Style          string    `json:"style"`
	Occasion       string    `json:"occasion"`
	WardrobeMode   string    `json:"wardrobeMode"`
	BPM            int       `json:"bpm,omitempty"`
	MusicDirection string    `json:"musicDirection,omitempty"`
	Chapters       []Chapter `json:"chapters"`
	Shots          []Shot    `json:"shots"`
}

// Both the client catalog and server planner use the same versioned definition.
//
//go:embed catalog.json
var rawCatalog []byte

// Historical catalogs stay embedded so an existing project can still render
// with the template version it was created from after a newer version ships.
// New projects always use the current catalog above.
//
//go:embed archive.json
var rawArchive []byte

var catalog = func() []Film {
	var result []Film
	if err := json.Unmarshal(rawCatalog, &result); err != nil {
		panic(err)
	}
	return result
}()

var archiveCatalog = func() []Film {
	var result []Film
	if err := json.Unmarshal(rawArchive, &result); err != nil {
		panic(err)
	}
	return result
}()

func Find(id string) (Film, bool) {
	for _, item := range catalog {
		if item.ID == id {
			return item, true
		}
	}
	return Film{}, false
}

func FindVersion(id, version string) (Film, bool) {
	for _, item := range catalog {
		if item.ID == id && item.Version == version {
			return item, true
		}
	}
	for _, item := range archiveCatalog {
		if item.ID == id && item.Version == version {
			return item, true
		}
	}
	return Film{}, false
}

func Apply(p *domain.Project) {
	if p.CreationMode != "template" || p.TemplateID == "" {
		return
	}
	t, ok := Find(p.TemplateID)
	if p.TemplateVersion != "" {
		t, ok = FindVersion(p.TemplateID, p.TemplateVersion)
	}
	if ok {
		if p.TemplateVersion == "" {
			p.TemplateVersion = t.Version
		}
		p.Scene, p.Duration, p.Ratio, p.Style, p.Occasion, p.WardrobeMode = t.Scene, t.Duration, t.Ratio, t.Style, t.Occasion, t.WardrobeMode
	}
}

package templates

import (
	_ "embed"
	"encoding/json"
	"vowfilm/server/internal/domain"
)

type Chapter struct {
	Title   string `json:"title"`
	Setting string `json:"setting"`
	Look    string `json:"look"`
	Beat    string `json:"beat"`
}
type Shot struct {
	Title   string `json:"title"`
	Chapter int    `json:"chapter"`
	Camera  string `json:"camera"`
	Action  string `json:"action"`
	Caption string `json:"caption"`
}
type Film struct {
	ID           string    `json:"id"`
	Version      string    `json:"version"`
	Name         string    `json:"name"`
	Subtitle     string    `json:"subtitle"`
	Scene        string    `json:"scene"`
	Duration     int       `json:"duration"`
	Ratio        string    `json:"ratio"`
	Style        string    `json:"style"`
	Occasion     string    `json:"occasion"`
	WardrobeMode string    `json:"wardrobeMode"`
	Chapters     []Chapter `json:"chapters"`
	Shots        []Shot    `json:"shots"`
}

// Both the client catalog and server planner use the same versioned definition.
//
//go:embed catalog.json
var rawCatalog []byte

var catalog = func() []Film {
	var result []Film
	if err := json.Unmarshal(rawCatalog, &result); err != nil {
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

func Apply(p *domain.Project) {
	if p.CreationMode != "template" || p.TemplateID == "" {
		return
	}
	if t, ok := Find(p.TemplateID); ok {
		if p.TemplateVersion == "" {
			p.TemplateVersion = t.Version
		}
		p.Scene, p.Duration, p.Ratio, p.Style, p.Occasion, p.WardrobeMode = t.Scene, t.Duration, t.Ratio, t.Style, t.Occasion, t.WardrobeMode
	}
}

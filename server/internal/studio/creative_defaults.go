package studio

import (
	"strings"
	"unicode"
)

// Defaults apply only to omitted values at the input boundary. Explicit settings
// still pass through validation; neither creation nor naming calls a paid model.
func applyProjectDefaults(p *Project) {
	if p.Scene == "" {
		p.Scene = "wedding"
	}
	duration, style, ratio := 60, "joyful", "16:9"
	switch p.Scene {
	case "family":
		duration, style = 120, "vintage"
	case "anniversary":
		style = "romantic"
	case "commerce":
		duration, style, ratio = 15, "editorial", "9:16"
	}
	if p.Duration == 0 {
		p.Duration = duration
	}
	if p.Style == "" {
		p.Style = style
	}
	if p.Ratio == "" {
		p.Ratio = ratio
	}
	if p.Occasion == "" && len(sceneFor(p).Occasions) > 0 {
		p.Occasion = sceneFor(p).Occasions[0]
	}
	if p.WardrobeMode == "" {
		p.WardrobeMode = "auto"
		if p.Scene == "commerce" {
			p.WardrobeMode = "fixed"
		}
	}
	p.Title = strings.TrimSpace(p.Title)
	p.AutoTitle = p.Title == ""
	if p.AutoTitle {
		p.Title = automaticProjectTitle(p)
	}
}

func automaticProjectTitle(p *Project) string {
	for _, part := range strings.FieldsFunc(p.Brief, func(r rune) bool { return strings.ContainsRune("\n\r。！？!?；;", r) }) {
		part = strings.TrimFunc(part, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
		if part == "" {
			continue
		}
		runes := []rune(strings.Join(strings.Fields(part), " "))
		if len(runes) > 24 {
			return string(runes[:24]) + "…"
		}
		return string(runes)
	}
	return sceneFor(p).Name + "新作"
}

package domain

type SubtitleCue struct {
	ID    string  `json:"id"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}
type WeddingFact struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Source string `json:"source"`
}
type WeddingArtifact struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Name   string `json:"name"`
	File   string `json:"file"`
	URL    string `json:"url"`
	MIME   string `json:"mime"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
	ShotID string `json:"shotId,omitempty"`
	Text   string `json:"text,omitempty"`
}
type WeddingApproval struct {
	By       string `json:"by"`
	ActorID  string `json:"actorId"`
	Evidence string `json:"evidence"`
	At       string `json:"at"`
	Version  string `json:"version"`
}
type WeddingStep struct {
	Number    int               `json:"number"`
	Name      string            `json:"name"`
	Status    string            `json:"status"`
	Version   string            `json:"version"`
	Artifacts []WeddingArtifact `json:"artifacts"`
	Approvals []WeddingApproval `json:"approvals"`
}
type WeddingHistory struct {
	At     string      `json:"at"`
	Reason string      `json:"reason"`
	Step   WeddingStep `json:"step"`
}
type WeddingWorkflow struct {
	Automatic        bool              `json:"automatic,omitempty"`
	AutoTasks        map[string]string `json:"autoTasks,omitempty"`
	AutoFiles        map[string]string `json:"autoFiles,omitempty"`
	PolicyVersion    string            `json:"policyVersion"`
	CurrentStep      int               `json:"currentStep"`
	Completed        bool              `json:"completed"`
	Steps            []WeddingStep     `json:"steps"`
	History          []WeddingHistory  `json:"history,omitempty"`
	Facts            []WeddingFact     `json:"facts,omitempty"`
	NarrationFile    string            `json:"narrationFile,omitempty"`
	NarrationURL     string            `json:"narrationUrl,omitempty"`
	NarrationSeconds float64           `json:"narrationSeconds,omitempty"`
	Cues             []SubtitleCue     `json:"cues,omitempty"`
	MixFile          string            `json:"mixFile,omitempty"`
	SubtitleURL      string            `json:"subtitleUrl,omitempty"`
}

package domain

type Asset struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Role            string `json:"role"`
	URL             string `json:"url"`
	File            string `json:"file"`
	MIME            string `json:"mime"`
	ProviderAssetID string `json:"providerAssetId,omitempty"`
}
type Shot struct {
	ID               string  `json:"id"`
	Title            string  `json:"title"`
	Chapter          string  `json:"chapter"`
	Description      string  `json:"description"`
	Camera           string  `json:"camera"`
	Prompt           string  `json:"prompt"`
	Caption          string  `json:"caption"`
	Transition       string  `json:"transition"`
	EntryAction      string  `json:"entryAction,omitempty"`
	ExitAction       string  `json:"exitAction,omitempty"`
	TransitionReason string  `json:"transitionReason,omitempty"`
	Duration         int     `json:"duration"`
	EditSeconds      float64 `json:"editSeconds"`
	EditFrames       int     `json:"editFrames"`
	TimelineStart    float64 `json:"timelineStart"`
	Status           string  `json:"status"`
	TaskID           string  `json:"taskId,omitempty"`
	Attempt          int     `json:"attempt"`
	Reserved         bool    `json:"reserved"`
	VideoURL         string  `json:"videoUrl,omitempty"`
	VideoFile        string  `json:"videoFile,omitempty"`
	ThumbnailURL     string  `json:"thumbnailUrl,omitempty"`
	Error            string  `json:"error,omitempty"`
	ActID            string  `json:"actId,omitempty"`
	LookID           string  `json:"lookId,omitempty"`
	ChangeToLookID   string  `json:"changeToLookId,omitempty"`
}
type Event struct {
	At      string `json:"at"`
	Message string `json:"message"`
}
type MusicSection struct {
	Name        string  `json:"name"`
	Start       float64 `json:"start"`
	End         float64 `json:"end"`
	Instruments string  `json:"instruments"`
	Energy      int     `json:"energy"`
}
type Project struct {
	OwnerID          string         `json:"ownerId"`
	CreationMode     string         `json:"creationMode,omitempty"`
	TemplateID       string         `json:"templateId,omitempty"`
	TemplateVersion  string         `json:"templateVersion,omitempty"`
	Scene            string         `json:"scene"`
	ID               string         `json:"id"`
	Title            string         `json:"title"`
	AutoTitle        bool           `json:"autoTitle,omitempty"`
	Brief            string         `json:"brief"`
	Duration         int            `json:"duration"`
	Style            string         `json:"style"`
	Ratio            string         `json:"ratio"`
	Status           string         `json:"status"`
	Progress         int            `json:"progress"`
	Message          string         `json:"message"`
	Synopsis         string         `json:"synopsis"`
	Demo             bool           `json:"demo"`
	Shots            []Shot         `json:"shots"`
	Assets           []Asset        `json:"assets"`
	Events           []Event        `json:"events"`
	FilmURL          string         `json:"filmUrl,omitempty"`
	PosterURL        string         `json:"posterUrl,omitempty"`
	OutputResolution string         `json:"outputResolution"`
	CreatedAt        string         `json:"createdAt"`
	UpdatedAt        string         `json:"updatedAt"`
	Revision         int            `json:"revision"`
	GeneratedSeconds int            `json:"generatedSeconds"`
	GenerationBudget int            `json:"generationBudget"`
	LLMModel         string         `json:"llmModel"`
	VideoModel       string         `json:"videoModel"`
	GenerationMode   string         `json:"generationMode,omitempty"`
	PromptPolicy     string         `json:"promptPolicy,omitempty"`
	MusicSections    []MusicSection `json:"musicSections,omitempty"`
	MusicSource      string         `json:"musicSource,omitempty"`
	MusicTaskID      string         `json:"musicTaskId,omitempty"`
	MusicFile        string         `json:"musicFile,omitempty"`
	Occasion         string         `json:"occasion,omitempty"`
	CustomPrompt     string         `json:"customPrompt,omitempty"`
	WardrobeMode     string         `json:"wardrobeMode,omitempty"`
	WardrobePrompt   string         `json:"wardrobePrompt,omitempty"`
	EndingText       string         `json:"endingText,omitempty"`
	Treatment        *Treatment     `json:"treatment,omitempty"`
}
type Look struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Bride string `json:"bride"`
	Groom string `json:"groom"`
}
type Act struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	LookID    string  `json:"lookId"`
	Setting   string  `json:"setting"`
	StoryBeat string  `json:"storyBeat"`
	Bridge    string  `json:"bridge"`
	FirstShot int     `json:"firstShot"`
	LastShot  int     `json:"lastShot"`
	Start     float64 `json:"start"`
	End       float64 `json:"end"`
}
type Treatment struct {
	Concept        string   `json:"concept"`
	IdentityAnchor string   `json:"identityAnchor"`
	OpeningHook    string   `json:"openingHook"`
	ClosingLine    string   `json:"closingLine"`
	MusicDirection string   `json:"musicDirection"`
	BPM            int      `json:"bpm"`
	MustHave       []string `json:"mustHave"`
	Avoid          []string `json:"avoid"`
	Notes          []string `json:"notes"`
	Looks          []Look   `json:"looks"`
	Acts           []Act    `json:"acts"`
}

type ProjectRepository interface {
	LoadProjects() (map[string]*Project, error)
	SaveProjects(map[string]*Project) error
}

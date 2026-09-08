package studio

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Config struct {
	DataDir, BaseURL, APIKey, LLMModel, VideoModel, Token, Addr string
	Concurrency                                                 int
}
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
	ID            string  `json:"id"`
	Title         string  `json:"title"`
	Chapter       string  `json:"chapter"`
	Description   string  `json:"description"`
	Camera        string  `json:"camera"`
	Prompt        string  `json:"prompt"`
	Caption       string  `json:"caption"`
	Transition    string  `json:"transition"`
	Duration      int     `json:"duration"`
	EditSeconds   float64 `json:"editSeconds"`
	EditFrames    int     `json:"editFrames"`
	TimelineStart float64 `json:"timelineStart"`
	Status        string  `json:"status"`
	TaskID        string  `json:"taskId,omitempty"`
	Attempt       int     `json:"attempt"`
	Reserved      bool    `json:"reserved"`
	VideoURL      string  `json:"videoUrl,omitempty"`
	VideoFile     string  `json:"videoFile,omitempty"`
	ThumbnailURL  string  `json:"thumbnailUrl,omitempty"`
	Error         string  `json:"error,omitempty"`
}
type Event struct {
	At      string `json:"at"`
	Message string `json:"message"`
}
type Project struct {
	ID               string  `json:"id"`
	Title            string  `json:"title"`
	Brief            string  `json:"brief"`
	Duration         int     `json:"duration"`
	Style            string  `json:"style"`
	Ratio            string  `json:"ratio"`
	Status           string  `json:"status"`
	Progress         int     `json:"progress"`
	Message          string  `json:"message"`
	Synopsis         string  `json:"synopsis"`
	Demo             bool    `json:"demo"`
	Shots            []Shot  `json:"shots"`
	Assets           []Asset `json:"assets"`
	Events           []Event `json:"events"`
	FilmURL          string  `json:"filmUrl,omitempty"`
	PosterURL        string  `json:"posterUrl,omitempty"`
	OutputResolution string  `json:"outputResolution"`
	CreatedAt        string  `json:"createdAt"`
	UpdatedAt        string  `json:"updatedAt"`
	Revision         int     `json:"revision"`
	GeneratedSeconds int     `json:"generatedSeconds"`
	GenerationBudget int     `json:"generationBudget"`
	LLMModel         string  `json:"llmModel"`
	VideoModel       string  `json:"videoModel"`
}
type Store struct {
	mu       sync.RWMutex
	dir      string
	projects map[string]*Project
}

func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, projects: map[string]*Project{}}
	raw, err := os.ReadFile(filepath.Join(dir, "projects.json"))
	if err == nil {
		err = json.Unmarshal(raw, &s.projects)
	}
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return s, nil
}
func copyProject(p *Project) *Project {
	if p == nil {
		return nil
	}
	b, _ := json.Marshal(p)
	var out Project
	_ = json.Unmarshal(b, &out)
	return &out
}
func (s *Store) Get(id string) *Project {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return copyProject(s.projects[id])
}
func (s *Store) List() []*Project {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a := make([]*Project, 0, len(s.projects))
	for _, p := range s.projects {
		a = append(a, copyProject(p))
	}
	sort.Slice(a, func(i, j int) bool { return a[i].CreatedAt > a[j].CreatedAt })
	return a
}
func (s *Store) persist() error {
	raw, err := json.MarshalIndent(s.projects, "", "  ")
	if err != nil {
		return err
	}
	f := filepath.Join(s.dir, "projects.json.tmp")
	if err = os.WriteFile(f, raw, 0600); err != nil {
		return err
	}
	return os.Rename(f, filepath.Join(s.dir, "projects.json"))
}
func (s *Store) Put(p *Project) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.projects[p.ID]
	s.projects[p.ID] = copyProject(p)
	if err := s.persist(); err != nil {
		if old == nil {
			delete(s.projects, p.ID)
		} else {
			s.projects[p.ID] = old
		}
		return err
	}
	return nil
}
func (s *Store) Update(id string, fn func(*Project) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.projects[id]
	if old == nil {
		return errors.New("项目不存在")
	}
	next := copyProject(old)
	if err := fn(next); err != nil {
		return err
	}
	next.UpdatedAt = now()
	s.projects[id] = next
	if err := s.persist(); err != nil {
		s.projects[id] = old
		return err
	}
	return nil
}
func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func newID(prefix string) string {
	b := make([]byte, 10)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}
func event(p *Project, msg string) {
	p.Message = msg
	p.Events = append(p.Events, Event{now(), msg})
	if len(p.Events) > 180 {
		p.Events = p.Events[len(p.Events)-180:]
	}
}
func validateProject(p *Project) error {
	if len([]rune(p.Title)) < 1 || len([]rune(p.Title)) > 80 {
		return errors.New("片名需为 1–80 个字符")
	}
	if p.Duration != 60 && p.Duration != 120 && p.Duration != 180 && p.Duration != 240 {
		return errors.New("时长请选择 60、120、180 或 240 秒")
	}
	if p.Ratio != "16:9" && p.Ratio != "9:16" {
		return errors.New("画幅请选择 16:9 或 9:16")
	}
	if p.Style != "garden" && p.Style != "seaside" && p.Style != "vintage" {
		return errors.New("不支持的影片风格")
	}
	if len([]rune(p.Brief)) > 4000 {
		return errors.New("故事描述不能超过 4000 字")
	}
	return nil
}
func isRunning(status string) bool {
	return status == "planning" || status == "generating" || status == "rendering"
}
func layout(p *Project) error {
	n := len(p.Shots)
	if n < 1 {
		return errors.New("没有可剪辑的镜头")
	}
	overlap := 0
	for i := 0; i < n-1; i++ {
		if p.Shots[i].Transition != "cut" {
			overlap += 12
		}
	}
	total := p.Duration*24 + overlap
	base, extra := total/n, total%n
	cursor := 0
	for i := range p.Shots {
		s := &p.Shots[i]
		s.EditFrames = base
		if i < extra {
			s.EditFrames++
		}
		s.EditSeconds = float64(s.EditFrames) / 24
		s.TimelineStart = float64(cursor) / 24
		need := (s.EditFrames + 18 + 23) / 24
		if need < 4 {
			need = 4
		}
		if need > 15 {
			return fmt.Errorf("镜头 %s 超出模型时长限制", s.ID)
		}
		s.Duration = need
		cursor += s.EditFrames
		if i < n-1 && s.Transition != "cut" {
			cursor -= 12
		}
	}
	if cursor != p.Duration*24 {
		return errors.New("时间线时长不匹配")
	}
	return nil
}

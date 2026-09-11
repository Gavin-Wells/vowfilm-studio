package studio

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
	"vowfilm/server/internal/domain"
)

type Config struct {
	DataDir, BaseURL, APIKey, LLMModel, VideoModel, Token, Addr, PublicMediaURL string
	Concurrency                                                                 int
	SetupToken, DatabaseDriver, DatabaseURL                                     string
}
type Asset = domain.Asset
type Shot = domain.Shot
type Event = domain.Event
type MusicSection = domain.MusicSection
type Project = domain.Project
type Store struct {
	repo     domain.ProjectRepository
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
func NewRepositoryStore(repo domain.ProjectRepository) (*Store, error) {
	projects, err := repo.LoadProjects()
	if err != nil {
		return nil, err
	}
	return &Store{repo: repo, projects: projects}, nil
}
func (s *Store) persist() error {
	if s.repo != nil {
		return s.repo.SaveProjects(s.projects)
	}
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
	if err := validateTemplateProject(p); err != nil {
		return err
	}
	if len([]rune(p.Title)) < 1 || len([]rune(p.Title)) > 80 {
		return errors.New("片名需为 1–80 个字符")
	}
	if sceneID(p) == "commerce" {
		if p.Duration != 15 {
			return errors.New("电商 v3 采用15秒整条直出")
		}
	} else if p.Duration != 60 && p.Duration != 120 && p.Duration != 180 && p.Duration != 240 {
		return errors.New("此场景支持60–240秒整分钟")
	}

	if p.Ratio != "16:9" && p.Ratio != "9:16" {
		return errors.New("画幅请选择 16:9 或 9:16")
	}
	if !validStyle(p.Style) {
		return errors.New("不支持的影片风格")
	}
	if len([]rune(p.Brief)) > 4000 {
		return errors.New("故事描述不能超过 4000 字")
	}
	if err := validateCreativeSettings(p); err != nil {
		return err
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
	if isDirectCommerce(p) {
		if n != 1 || p.Duration != 15 {
			return errors.New("电商直出必须是一条15秒广告")
		}
		s := &p.Shots[0]
		s.Duration = 15
		s.EditSeconds = 15
		s.EditFrames = 360
		s.TimelineStart = 0
		s.Transition = "cut"
		return nil
	}
	overlap := 0
	if p.CreationMode == "template" {
		if p.Duration*24%n != 0 {
			return errors.New("模板时长不能平均分配到固定镜头")
		}
		frames := p.Duration * 24 / n
		for i := range p.Shots {
			s := &p.Shots[i]
			s.EditFrames, s.EditSeconds, s.TimelineStart = frames, float64(frames)/24, float64(i*frames)/24
			s.Duration = int(math.Ceil(s.EditSeconds))
			s.Transition = "cut"
		}
		return nil
	}
	for i := 0; i < n-1; i++ {
		overlap += overlapFrames(p.Shots[i].Transition)
	}
	total := p.Duration*24 + overlap
	base, extra := total/n, total%n
	cursor := 0
	// Cuts land on a half-second beat grid for the 120 BPM styles. Vary shot
	// lengths to create an opening, build, bridge, celebration and coda.
	weights := []int{6, 6, 4, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8}
	weightSum := 0
	for i := range p.Shots {
		weightSum += weights[i%len(weights)]
	}
	weightCursor := 0
	for i := range p.Shots {
		s := &p.Shots[i]
		s.EditFrames = base
		if i < extra {
			s.EditFrames++
		}
		if rhythmicStyle(p.Style) && sceneID(p) != "commerce" {
			weightCursor += weights[i%len(weights)]
			beatFrames := 1440.0 / float64(targetBPM(p))
			ideal := float64(weightCursor*p.Duration*24) / float64(weightSum)
			if p.Treatment != nil {
				for ai, act := range p.Treatment.Acts {
					if i+1 >= act.FirstShot && i+1 <= act.LastShot {
						localTotal, localWeight := 0, 0
						for j := act.FirstShot - 1; j < act.LastShot; j++ {
							w := weights[(j-act.FirstShot+1)%len(weights)]
							localTotal += w
							if j <= i {
								localWeight += w
							}
						}
						ideal = (float64(ai) + float64(localWeight)/float64(localTotal)) * float64(p.Duration*24) / float64(len(p.Treatment.Acts))
						break
					}
				}
			}
			next := int(math.Round(math.Round(ideal/beatFrames) * beatFrames))
			if i == n-1 {
				next = p.Duration * 24
			}
			s.EditFrames = next - cursor
			if i < n-1 {
				s.EditFrames += overlapFrames(s.Transition)
			}
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
		if i < n-1 {
			cursor -= overlapFrames(s.Transition)
		}
	}
	if cursor != p.Duration*24 {
		return errors.New("时间线时长不匹配")
	}
	if p.Treatment != nil {
		for i := range p.Treatment.Acts {
			act := &p.Treatment.Acts[i]
			if act.FirstShot < 1 || act.LastShot > n || act.FirstShot > act.LastShot {
				return errors.New("章节镜头范围无效")
			}
			act.Start = p.Shots[act.FirstShot-1].TimelineStart
			act.End = float64(p.Duration)
			if act.LastShot < n {
				act.End = p.Shots[act.LastShot].TimelineStart
			}
		}
	}
	return nil
}

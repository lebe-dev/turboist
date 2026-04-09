package troiki

import (
	"context"
	"fmt"
	"strings"

	synctodoist "github.com/CnTeng/todoist-api-go/sync"
	"github.com/charmbracelet/log"
	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/todoist"
	"golang.org/x/text/unicode/norm"
)

type SectionClass string

const (
	Important SectionClass = "important"
	Medium    SectionClass = "medium"
	Rest      SectionClass = "rest"
)

var sectionOrder = []SectionClass{Important, Medium, Rest}

type SectionState struct {
	Class     SectionClass    `json:"class"`
	SectionID string          `json:"section_id"`
	Name      string          `json:"name"`
	Tasks     []*todoist.Task `json:"tasks"`
	RootCount int             `json:"root_count"`
	MaxTasks  int             `json:"max_tasks"`
	Capacity  int             `json:"capacity"`
	CanAdd    bool            `json:"can_add"`
}

type State struct {
	ProjectID string         `json:"project_id"`
	Sections  []SectionState `json:"sections"`
}

type capacityStore interface {
	GetAllTroikiCapacity() (map[string]int, error)
	IncrementTroikiCapacity(sectionClass string) error
	EnsureMinTroikiCapacity(sectionClass string, min int) error
}

type cache interface {
	Projects() []*todoist.Project
	Sections() []*todoist.Section
	Tasks() []*todoist.Task
	AddTask(ctx context.Context, args *synctodoist.TaskAddArgs) (string, error)
	AddSection(ctx context.Context, name string, projectID string) (string, error)
}

type Service struct {
	cache cache
	cfg   config.TroikiConfig
	store capacityStore

	projectID  string
	sectionIDs map[SectionClass]string
}

func NewService(cache cache, cfg config.TroikiConfig, store capacityStore) *Service {
	return &Service{
		cache:      cache,
		cfg:        cfg,
		store:      store,
		sectionIDs: make(map[SectionClass]string),
	}
}

func normName(s string) string {
	return norm.NFC.String(strings.TrimSpace(s))
}

// Init resolves the troiki project by name and finds or creates the 3 sections.
func (s *Service) Init(ctx context.Context) error {
	want := normName(s.cfg.ProjectName)
	projects := s.cache.Projects()

	for _, p := range projects {
		log.Debug("troiki: todoist project", "name", "'"+p.Name+"'")
	}

	for _, p := range projects {
		if normName(p.Name) == want {
			s.projectID = p.ID
			break
		}
	}
	if s.projectID == "" {
		log.Warn("troiki project not found", "want", "'"+want+"'")
		return fmt.Errorf("troiki project %q not found", s.cfg.ProjectName)
	}

	sectionNames := map[SectionClass]string{
		Important: s.cfg.Sections.Important,
		Medium:    s.cfg.Sections.Medium,
		Rest:      s.cfg.Sections.Rest,
	}

	existing := s.cache.Sections()
	existingByName := make(map[string]*todoist.Section)
	for _, sec := range existing {
		if sec.ProjectID == s.projectID {
			existingByName[sec.Name] = sec
		}
	}

	for _, class := range sectionOrder {
		name := sectionNames[class]
		if sec, ok := existingByName[name]; ok {
			s.sectionIDs[class] = sec.ID
			continue
		}
		id, err := s.cache.AddSection(ctx, name, s.projectID)
		if err != nil {
			return fmt.Errorf("create section %q: %w", name, err)
		}
		s.sectionIDs[class] = id
	}

	for _, class := range []SectionClass{Medium, Rest} {
		if err := s.store.EnsureMinTroikiCapacity(string(class), s.cfg.InitialCapacity); err != nil {
			return fmt.Errorf("init capacity for %s: %w", class, err)
		}
	}

	log.Info("troiki initialized",
		"project_id", s.projectID,
		"important_section", s.sectionIDs[Important],
		"medium_section", s.sectionIDs[Medium],
		"rest_section", s.sectionIDs[Rest],
	)
	return nil
}

// ProjectID returns the resolved troiki project ID.
func (s *Service) ProjectID() string {
	return s.projectID
}

// ComputeState builds the current troiki state from cache and storage.
func (s *Service) ComputeState() (State, error) {
	tasks := s.cache.Tasks()

	tasksBySection := make(map[SectionClass][]*todoist.Task)
	rootCounts := make(map[SectionClass]int)
	for _, t := range tasks {
		if t.ProjectID != s.projectID {
			continue
		}
		class, ok := s.classForSection(t.SectionID)
		if !ok {
			continue
		}
		tasksBySection[class] = append(tasksBySection[class], t)
		if t.ParentID == nil {
			rootCounts[class]++
		}
	}

	caps, err := s.store.GetAllTroikiCapacity()
	if err != nil {
		return State{}, fmt.Errorf("get capacity: %w", err)
	}

	sections := make([]SectionState, 0, 3)
	for _, class := range sectionOrder {
		capacity := caps[string(class)]
		if class == Important {
			capacity = s.cfg.MaxTasksPerSection
		}

		rootCount := rootCounts[class]
		canAdd := s.canAdd(class, rootCount, capacity)

		sectionTasks := tasksBySection[class]
		if sectionTasks == nil {
			sectionTasks = []*todoist.Task{}
		}

		sections = append(sections, SectionState{
			Class:     class,
			SectionID: s.sectionIDs[class],
			Name:      s.sectionName(class),
			Tasks:     sectionTasks,
			RootCount: rootCount,
			MaxTasks:  s.cfg.MaxTasksPerSection,
			Capacity:  capacity,
			CanAdd:    canAdd,
		})
	}

	return State{
		ProjectID: s.projectID,
		Sections:  sections,
	}, nil
}

// CanAddTask checks whether a new root task can be added to the given section class.
func (s *Service) CanAddTask(class SectionClass) (bool, error) {
	tasks := s.cache.Tasks()
	rootCount := 0
	for _, t := range tasks {
		if t.ProjectID != s.projectID || t.ParentID != nil {
			continue
		}
		c, ok := s.classForSection(t.SectionID)
		if !ok || c != class {
			continue
		}
		rootCount++
	}

	if class == Important {
		return rootCount < s.cfg.MaxTasksPerSection, nil
	}

	caps, err := s.store.GetAllTroikiCapacity()
	if err != nil {
		return false, fmt.Errorf("get capacity: %w", err)
	}
	capacity := caps[string(class)]
	return s.canAdd(class, rootCount, capacity), nil
}

// OnTaskCompleted handles capacity unlock when a troiki task is completed.
// Important task -> unlocks Medium. Medium task -> unlocks Rest. Rest -> no-op.
func (s *Service) OnTaskCompleted(task *todoist.Task) {
	if task.ProjectID != s.projectID {
		return
	}

	class, ok := s.classForSection(task.SectionID)
	if !ok {
		return
	}

	// Only root tasks unlock downstream capacity
	if task.ParentID != nil {
		return
	}

	var downstream SectionClass
	switch class {
	case Important:
		downstream = Medium
	case Medium:
		downstream = Rest
	case Rest:
		return
	}

	if err := s.store.IncrementTroikiCapacity(string(downstream)); err != nil {
		log.Error("troiki: failed to increment capacity", "class", downstream, "err", err)
	}
}

// AddTask creates a new task in the specified troiki section with capacity validation.
func (s *Service) AddTask(ctx context.Context, class SectionClass, content, description string) (string, error) {
	canAdd, err := s.CanAddTask(class)
	if err != nil {
		return "", fmt.Errorf("check capacity: %w", err)
	}
	if !canAdd {
		return "", ErrNoCapacity
	}

	sectionID := s.sectionIDs[class]
	projectID := s.projectID
	priority := priorityForClass(class)
	args := &synctodoist.TaskAddArgs{
		Content:   content,
		ProjectID: &projectID,
		SectionID: &sectionID,
		Priority:  &priority,
	}
	if description != "" {
		args.Description = &description
	}

	id, err := s.cache.AddTask(ctx, args)
	if err != nil {
		return "", fmt.Errorf("add task: %w", err)
	}
	return id, nil
}

// SectionIDForClass returns the resolved section ID for a given class.
func (s *Service) SectionIDForClass(class SectionClass) string {
	return s.sectionIDs[class]
}

func (s *Service) canAdd(class SectionClass, rootCount, capacity int) bool {
	if class == Important {
		return rootCount < s.cfg.MaxTasksPerSection
	}
	return rootCount < min(capacity, s.cfg.MaxTasksPerSection)
}

func (s *Service) classForSection(sectionID *string) (SectionClass, bool) {
	if sectionID == nil {
		return "", false
	}
	for class, id := range s.sectionIDs {
		if id == *sectionID {
			return class, true
		}
	}
	return "", false
}

func (s *Service) sectionName(class SectionClass) string {
	switch class {
	case Important:
		return s.cfg.Sections.Important
	case Medium:
		return s.cfg.Sections.Medium
	case Rest:
		return s.cfg.Sections.Rest
	}
	return ""
}

// SetTestState sets the internal resolved state for testing from outside the package.
func (s *Service) SetTestState(projectID string, sectionIDs map[SectionClass]string) {
	s.projectID = projectID
	for class, id := range sectionIDs {
		s.sectionIDs[class] = id
	}
}

// priorityForClass returns Todoist priority value for a troiki section.
// 4 = red (urgent), 3 = yellow (high), 2 = blue (medium), 1 = no priority.
func priorityForClass(class SectionClass) int {
	switch class {
	case Important:
		return 4
	case Medium:
		return 3
	case Rest:
		return 2
	default:
		return 1
	}
}

var ErrNoCapacity = fmt.Errorf("no capacity available")

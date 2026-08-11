// Package store keeps every project the user tracks in one JSON file.
//
// A JSON document is the right shape here: the working set is a few hundred
// rows at most, it stays readable/diffable/backup-able by hand, and it keeps
// the binary free of a database driver.
package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/abekci-dev/kisaf-project-manager/internal/apperr"
)

// Priority is how urgent a task is. Kept as a small closed set rather than a
// free-form field so the UI can offer a fixed choice and sort by it.
type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityNormal Priority = "normal"
	PriorityHigh   Priority = "high"
)

func (p Priority) valid() bool {
	switch p {
	case PriorityLow, PriorityNormal, PriorityHigh:
		return true
	}
	return false
}

// rank orders priorities high-to-low for display.
func (p Priority) rank() int {
	switch p {
	case PriorityHigh:
		return 0
	case PriorityNormal:
		return 1
	default:
		return 2
	}
}

// Todo is one task attached to a project — the "where did I leave off?"
// problem, written as something you can tick off instead of prose in a note.
type Todo struct {
	ID        string     `json:"id"`
	Text      string     `json:"text"`
	Done      bool       `json:"done"`
	Priority  Priority   `json:"priority"`
	CreatedAt time.Time  `json:"createdAt"`
	DoneAt    *time.Time `json:"doneAt,omitempty"`
}

// TodoStats is the summary the project card shows.
type TodoStats struct {
	Total int `json:"total"`
	Done  int `json:"done"`
	High  int `json:"high"` // open tasks marked high priority
}

// Project is one tracked folder on disk.
type Project struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Path         string     `json:"path"`
	Description  string     `json:"description"`
	Notes        string     `json:"notes"`
	Todos        []Todo     `json:"todos"`
	Tags         []string   `json:"tags"`
	Color        string     `json:"color"`
	Favorite     bool       `json:"favorite"`
	Archived     bool       `json:"archived"`
	Editor       string     `json:"editor"` // per-project override of the default editor
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	LastOpenedAt *time.Time `json:"lastOpenedAt,omitempty"`
	OpenCount    int        `json:"openCount"`
}

// TodoStats summarises the task list.
func (p Project) TodoStats() TodoStats {
	stats := TodoStats{Total: len(p.Todos)}
	for _, t := range p.Todos {
		if t.Done {
			stats.Done++
			continue
		}
		if t.Priority == PriorityHigh {
			stats.High++
		}
	}
	return stats
}

// CustomEditor lets the user register a launcher we failed to auto-detect.
type CustomEditor struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Exec string `json:"exec"`
	Args string `json:"args"` // optional, {path} is substituted
}

// Settings are the preferences that belong to the UI rather than the process.
type Settings struct {
	DefaultEditor  string         `json:"defaultEditor"`
	ScanRoots      []string       `json:"scanRoots"`
	ScanDepth      int            `json:"scanDepth"`
	Theme          string         `json:"theme"`
	Sort           string         `json:"sort"`
	View           string         `json:"view"`     // "grid" (cards) or "list" (compact)
	Language       string         `json:"language"` // "auto" | "en" | "tr"
	CustomEditors  []CustomEditor `json:"customEditors"`
	ShowArchived   bool           `json:"showArchived"`
	CommitCount    int            `json:"commitCount"`
	TerminalPrefer string         `json:"terminalPrefer"`
}

type document struct {
	Version  int       `json:"version"`
	Projects []Project `json:"projects"`
	Settings Settings  `json:"settings"`
}

// Store is a goroutine-safe handle to the on-disk document.
type Store struct {
	mu   sync.RWMutex
	path string
	doc  document
}

// ErrNotFound is returned when an ID does not match any project.
var ErrNotFound = apperr.New(apperr.CodeProjectNotFound, "project not found")

// ErrTodoNotFound is returned when a task ID does not match any task.
var ErrTodoNotFound = apperr.New(apperr.CodeTodoNotFound, "task not found")

func defaultSettings() Settings {
	return Settings{
		DefaultEditor: "",
		ScanDepth:     3,
		Theme:         "dark",
		Sort:          "recent",
		View:          "grid",
		Language:      "auto",
		CommitCount:   25,
	}
}

// Open loads data.json from dir, creating an empty document on first run.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{path: filepath.Join(dir, "data.json")}
	s.doc = document{Version: 1, Projects: []Project{}, Settings: defaultSettings()}

	raw, err := os.ReadFile(s.path)
	switch {
	case err == nil:
		if err := json.Unmarshal(raw, &s.doc); err != nil {
			// Never destroy user data on a parse error: park the broken file
			// next to the original so it can be recovered by hand.
			backup := s.path + ".corrupt-" + time.Now().Format("20060102-150405")
			_ = os.WriteFile(backup, raw, 0o600)
			s.doc = document{Version: 1, Projects: []Project{}, Settings: defaultSettings()}
			return s, fmt.Errorf("could not read data.json, backed up to %s: %w", backup, err)
		}
	case os.IsNotExist(err):
		if err := s.save(); err != nil {
			return nil, err
		}
	default:
		return nil, err
	}

	if s.doc.Projects == nil {
		s.doc.Projects = []Project{}
	}
	// Normalise rows written by an older version, so the API never hands the
	// UI a null where it expects a list.
	for i := range s.doc.Projects {
		if s.doc.Projects[i].Todos == nil {
			s.doc.Projects[i].Todos = []Todo{}
		}
		for j := range s.doc.Projects[i].Todos {
			if !s.doc.Projects[i].Todos[j].Priority.valid() {
				s.doc.Projects[i].Todos[j].Priority = PriorityNormal
			}
		}
	}
	if s.doc.Settings.ScanDepth <= 0 {
		s.doc.Settings.ScanDepth = 3
	}
	if s.doc.Settings.CommitCount <= 0 {
		s.doc.Settings.CommitCount = 25
	}
	return s, nil
}

// Path is the location of the backing file, surfaced in the UI so the user
// always knows where their data lives.
func (s *Store) Path() string { return s.path }

// save assumes the caller holds the write lock. The temp-file + rename dance
// means a crash mid-write can never leave a half-written data.json behind.
func (s *Store) save() error {
	raw, err := json.MarshalIndent(s.doc, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Projects returns a copy of every project, ordered for display.
func (s *Store) Projects() []Project {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Project, len(s.doc.Projects))
	copy(out, s.doc.Projects)
	sortProjects(out, s.doc.Settings.Sort)
	return out
}

// Get returns a single project by ID.
func (s *Store) Get(id string) (Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.doc.Projects {
		if p.ID == id {
			return p, nil
		}
	}
	return Project{}, ErrNotFound
}

// Settings returns the current UI preferences.
func (s *Store) Settings() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.doc.Settings
}

// SaveSettings replaces the UI preferences wholesale.
func (s *Store) SaveSettings(in Settings) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if in.ScanDepth <= 0 {
		in.ScanDepth = 3
	}
	if in.ScanDepth > 8 {
		in.ScanDepth = 8
	}
	if in.CommitCount <= 0 || in.CommitCount > 200 {
		in.CommitCount = 25
	}
	if in.ScanRoots == nil {
		in.ScanRoots = []string{}
	}
	if in.CustomEditors == nil {
		in.CustomEditors = []CustomEditor{}
	}
	if in.View != "grid" && in.View != "list" {
		in.View = "grid"
	}
	switch in.Language {
	case "en", "tr", "auto":
	default:
		in.Language = "auto"
	}
	for i := range in.CustomEditors {
		if in.CustomEditors[i].ID == "" {
			in.CustomEditors[i].ID = "custom-" + newID()
		}
	}
	s.doc.Settings = in
	return s.doc.Settings, s.save()
}

// Add registers a new project. The path is normalised first so the same folder
// reached through different spellings is still recognised as a duplicate.
func (s *Store) Add(p Project) (Project, error) {
	abs, err := NormalizePath(p.Path)
	if err != nil {
		return Project{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return Project{}, apperr.Wrap(err, apperr.CodeProjectUnopened, "cannot open folder: %v", err)
	}
	if !info.IsDir() {
		return Project{}, apperr.New(apperr.CodeProjectNotDir, "that path is not a folder")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, existing := range s.doc.Projects {
		if strings.EqualFold(existing.Path, abs) {
			return existing, apperr.New(apperr.CodeProjectDuplicate, "this folder is already tracked: %s", existing.Name)
		}
	}

	now := time.Now()
	p.ID = newID()
	p.Path = abs
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		p.Name = filepath.Base(abs)
	}
	p.Tags = cleanTags(p.Tags)
	if p.Todos == nil {
		p.Todos = []Todo{}
	}
	p.CreatedAt = now
	p.UpdatedAt = now
	s.doc.Projects = append(s.doc.Projects, p)
	return p, s.save()
}

// Patch applies a partial update. Only non-nil fields are touched, which keeps
// the UI free to send just the field the user edited.
type Patch struct {
	Name        *string   `json:"name"`
	Path        *string   `json:"path"`
	Description *string   `json:"description"`
	Notes       *string   `json:"notes"`
	Tags        *[]string `json:"tags"`
	Color       *string   `json:"color"`
	Favorite    *bool     `json:"favorite"`
	Archived    *bool     `json:"archived"`
	Editor      *string   `json:"editor"`
}

// Update applies a Patch to the project with the given ID.
func (s *Store) Update(id string, patch Patch) (Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := s.indexOf(id)
	if idx < 0 {
		return Project{}, ErrNotFound
	}
	p := &s.doc.Projects[idx]

	if patch.Name != nil {
		if name := strings.TrimSpace(*patch.Name); name != "" {
			p.Name = name
		}
	}
	if patch.Path != nil {
		abs, err := NormalizePath(*patch.Path)
		if err != nil {
			return Project{}, err
		}
		for i, existing := range s.doc.Projects {
			if i != idx && strings.EqualFold(existing.Path, abs) {
				return Project{}, apperr.New(apperr.CodeProjectDuplicate, "this folder is already tracked: %s", existing.Name)
			}
		}
		p.Path = abs
	}
	if patch.Description != nil {
		p.Description = *patch.Description
	}
	if patch.Notes != nil {
		p.Notes = *patch.Notes
	}
	if patch.Tags != nil {
		p.Tags = cleanTags(*patch.Tags)
	}
	if patch.Color != nil {
		p.Color = *patch.Color
	}
	if patch.Favorite != nil {
		p.Favorite = *patch.Favorite
	}
	if patch.Archived != nil {
		p.Archived = *patch.Archived
	}
	if patch.Editor != nil {
		p.Editor = *patch.Editor
	}
	p.UpdatedAt = time.Now()

	out := *p
	return out, s.save()
}

// Delete removes a project from the list. The folder on disk is never touched —
// this tool only ever manages references.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.indexOf(id)
	if idx < 0 {
		return ErrNotFound
	}
	s.doc.Projects = append(s.doc.Projects[:idx], s.doc.Projects[idx+1:]...)
	return s.save()
}

// ---------------------------------------------------------------------------
// Tasks
// ---------------------------------------------------------------------------

// maxTodos keeps one runaway project from bloating data.json. It is far above
// any sane checklist, so hitting it means something has gone wrong.
const maxTodos = 500

// AddTodo appends a task and returns the whole project, so the caller can
// replace its copy in one step instead of patching a nested list by hand.
func (s *Store) AddTodo(projectID, text string, priority Priority) (Project, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Project{}, apperr.New(apperr.CodeTodoEmpty, "task text cannot be empty")
	}
	if len(text) > 500 {
		text = text[:500]
	}
	if !priority.valid() {
		priority = PriorityNormal
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	idx := s.indexOf(projectID)
	if idx < 0 {
		return Project{}, ErrNotFound
	}
	p := &s.doc.Projects[idx]
	if len(p.Todos) >= maxTodos {
		return Project{}, apperr.New(apperr.CodeTodoLimit, "a project can hold at most %d tasks", maxTodos)
	}

	p.Todos = append(p.Todos, Todo{
		ID:        newID(),
		Text:      text,
		Priority:  priority,
		CreatedAt: time.Now(),
	})
	p.UpdatedAt = time.Now()

	return *p, s.save()
}

// TodoPatch is a partial task update; nil fields are left alone.
type TodoPatch struct {
	Text     *string   `json:"text"`
	Done     *bool     `json:"done"`
	Priority *Priority `json:"priority"`
}

// UpdateTodo edits one task.
func (s *Store) UpdateTodo(projectID, todoID string, patch TodoPatch) (Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, todo, err := s.locateTodo(projectID, todoID)
	if err != nil {
		return Project{}, err
	}

	if patch.Text != nil {
		text := strings.TrimSpace(*patch.Text)
		if text == "" {
			return Project{}, apperr.New(apperr.CodeTodoEmpty, "task text cannot be empty")
		}
		if len(text) > 500 {
			text = text[:500]
		}
		todo.Text = text
	}
	if patch.Priority != nil && patch.Priority.valid() {
		todo.Priority = *patch.Priority
	}
	if patch.Done != nil && *patch.Done != todo.Done {
		todo.Done = *patch.Done
		if todo.Done {
			now := time.Now()
			todo.DoneAt = &now
		} else {
			todo.DoneAt = nil
		}
	}
	p.UpdatedAt = time.Now()

	return *p, s.save()
}

// DeleteTodo removes one task.
func (s *Store) DeleteTodo(projectID, todoID string) (Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := s.indexOf(projectID)
	if idx < 0 {
		return Project{}, ErrNotFound
	}
	p := &s.doc.Projects[idx]

	for i := range p.Todos {
		if p.Todos[i].ID == todoID {
			p.Todos = append(p.Todos[:i], p.Todos[i+1:]...)
			p.UpdatedAt = time.Now()
			return *p, s.save()
		}
	}
	return Project{}, ErrTodoNotFound
}

// ClearDoneTodos drops every completed task in one go.
func (s *Store) ClearDoneTodos(projectID string) (Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := s.indexOf(projectID)
	if idx < 0 {
		return Project{}, ErrNotFound
	}
	p := &s.doc.Projects[idx]

	kept := p.Todos[:0]
	for _, t := range p.Todos {
		if !t.Done {
			kept = append(kept, t)
		}
	}
	p.Todos = kept
	p.UpdatedAt = time.Now()

	return *p, s.save()
}

// ReorderTodos applies a new order given as a list of task IDs. Any task the
// caller left out keeps its place at the end, so a stale client cannot silently
// delete tasks by sending an incomplete list.
func (s *Store) ReorderTodos(projectID string, ids []string) (Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := s.indexOf(projectID)
	if idx < 0 {
		return Project{}, ErrNotFound
	}
	p := &s.doc.Projects[idx]

	byID := make(map[string]Todo, len(p.Todos))
	for _, t := range p.Todos {
		byID[t.ID] = t
	}

	ordered := make([]Todo, 0, len(p.Todos))
	seen := map[string]bool{}
	for _, id := range ids {
		if t, ok := byID[id]; ok && !seen[id] {
			seen[id] = true
			ordered = append(ordered, t)
		}
	}
	for _, t := range p.Todos {
		if !seen[t.ID] {
			ordered = append(ordered, t)
		}
	}

	p.Todos = ordered
	p.UpdatedAt = time.Now()
	return *p, s.save()
}

// locateTodo assumes the write lock is held.
func (s *Store) locateTodo(projectID, todoID string) (*Project, *Todo, error) {
	idx := s.indexOf(projectID)
	if idx < 0 {
		return nil, nil, ErrNotFound
	}
	p := &s.doc.Projects[idx]
	for i := range p.Todos {
		if p.Todos[i].ID == todoID {
			return p, &p.Todos[i], nil
		}
	}
	return nil, nil, ErrTodoNotFound
}

// MarkOpened records that the project was launched, which feeds the
// "recently used" ordering.
func (s *Store) MarkOpened(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.indexOf(id)
	if idx < 0 {
		return
	}
	now := time.Now()
	s.doc.Projects[idx].LastOpenedAt = &now
	s.doc.Projects[idx].OpenCount++
	_ = s.save()
}

// Tags returns every tag in use together with how many projects carry it.
func (s *Store) Tags() []TagCount {
	s.mu.RLock()
	defer s.mu.RUnlock()
	counts := map[string]int{}
	for _, p := range s.doc.Projects {
		for _, t := range p.Tags {
			counts[t]++
		}
	}
	out := make([]TagCount, 0, len(counts))
	for name, n := range counts {
		out = append(out, TagCount{Name: name, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// TagCount is a tag plus its usage count.
type TagCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// KnownPaths reports the normalised path of every tracked project, so a folder
// scan can mark rows that are already imported.
func (s *Store) KnownPaths() map[string]bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]bool, len(s.doc.Projects))
	for _, p := range s.doc.Projects {
		out[strings.ToLower(p.Path)] = true
	}
	return out
}

func (s *Store) indexOf(id string) int {
	for i := range s.doc.Projects {
		if s.doc.Projects[i].ID == id {
			return i
		}
	}
	return -1
}

func sortProjects(list []Project, mode string) {
	switch mode {
	case "name":
		sort.SliceStable(list, func(i, j int) bool {
			return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name)
		})
	case "created":
		sort.SliceStable(list, func(i, j int) bool { return list[i].CreatedAt.After(list[j].CreatedAt) })
	default: // "recent": most recently opened first, never-opened fall back to creation time
		sort.SliceStable(list, func(i, j int) bool {
			return lastTouch(list[i]).After(lastTouch(list[j]))
		})
	}
	// Favourites always float to the top, whatever the sort mode is.
	sort.SliceStable(list, func(i, j int) bool { return list[i].Favorite && !list[j].Favorite })
}

func lastTouch(p Project) time.Time {
	if p.LastOpenedAt != nil {
		return *p.LastOpenedAt
	}
	return p.CreatedAt
}

func cleanTags(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, t := range in {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		key := strings.ToLower(t)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out
}

// NormalizePath turns user input into an absolute, cleaned path.
func NormalizePath(p string) (string, error) {
	p = strings.TrimSpace(p)
	p = strings.Trim(p, "\"'")
	if p == "" {
		return "", apperr.New(apperr.CodePathEmpty, "folder path cannot be empty")
	}
	if strings.HasPrefix(p, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	abs, err := filepath.Abs(filepath.Clean(p))
	if err != nil {
		return "", err
	}
	return abs, nil
}

func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// A time-based fallback keeps IDs unique enough for a single-user tool.
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

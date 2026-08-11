package store

import (
	"errors"
	"testing"
)

func projectWithTodos(t *testing.T, texts ...string) (*Store, Project) {
	t.Helper()
	s := newStore(t)
	p, err := s.Add(Project{Path: t.TempDir(), Name: "project"})
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range texts {
		if p, err = s.AddTodo(p.ID, text, PriorityNormal); err != nil {
			t.Fatal(err)
		}
	}
	return s, p
}

func TestAddTodo(t *testing.T) {
	s, p := projectWithTodos(t)

	p, err := s.AddTodo(p.ID, "  finish the checkout screen  ", PriorityHigh)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Todos) != 1 {
		t.Fatalf("%d tasks", len(p.Todos))
	}
	todo := p.Todos[0]
	if todo.Text != "finish the checkout screen" {
		t.Errorf("whitespace was not trimmed: %q", todo.Text)
	}
	if todo.Priority != PriorityHigh {
		t.Errorf("priority = %q", todo.Priority)
	}
	if todo.Done || todo.DoneAt != nil {
		t.Error("a new task arrived already done")
	}
	if todo.ID == "" {
		t.Error("the task has no id")
	}
	_ = s
}

func TestAddTodoRejectsEmpty(t *testing.T) {
	s, p := projectWithTodos(t)
	if _, err := s.AddTodo(p.ID, "   ", PriorityNormal); err == nil {
		t.Error("an empty task was accepted")
	}
}

// TestAddTodoDefaultsPriority: the UI may omit the field entirely, and an
// invalid value must not end up stored and then break sorting later.
func TestAddTodoDefaultsPriority(t *testing.T) {
	s, p := projectWithTodos(t)

	p, err := s.AddTodo(p.ID, "task", Priority("nonsense"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Todos[0].Priority != PriorityNormal {
		t.Errorf("an invalid priority was not corrected: %q", p.Todos[0].Priority)
	}
}

func TestAddTodoUnknownProject(t *testing.T) {
	s, _ := projectWithTodos(t)
	if _, err := s.AddTodo("no-such-id", "task", PriorityNormal); !errors.Is(err, ErrNotFound) {
		t.Errorf("error = %v, wanted ErrNotFound", err)
	}
}

func TestToggleTodoSetsAndClearsDoneAt(t *testing.T) {
	s, p := projectWithTodos(t, "task")
	id := p.Todos[0].ID

	done := true
	p, err := s.UpdateTodo(p.ID, id, TodoPatch{Done: &done})
	if err != nil {
		t.Fatal(err)
	}
	if !p.Todos[0].Done || p.Todos[0].DoneAt == nil {
		t.Fatal("the done flag or its timestamp was not written")
	}

	notDone := false
	p, err = s.UpdateTodo(p.ID, id, TodoPatch{Done: &notDone})
	if err != nil {
		t.Fatal(err)
	}
	if p.Todos[0].Done || p.Todos[0].DoneAt != nil {
		t.Error("the completion time was not cleared when the task was reopened")
	}
}

func TestUpdateTodoTouchesOnlyGivenFields(t *testing.T) {
	s, p := projectWithTodos(t, "original text")
	id := p.Todos[0].ID

	high := PriorityHigh
	p, err := s.UpdateTodo(p.ID, id, TodoPatch{Priority: &high})
	if err != nil {
		t.Fatal(err)
	}
	if p.Todos[0].Text != "original text" {
		t.Errorf("text that was not touched changed: %q", p.Todos[0].Text)
	}
	if p.Todos[0].Priority != PriorityHigh {
		t.Error("the priority was not updated")
	}
}

func TestUpdateTodoRejectsEmptyText(t *testing.T) {
	s, p := projectWithTodos(t, "task")
	empty := "  "
	if _, err := s.UpdateTodo(p.ID, p.Todos[0].ID, TodoPatch{Text: &empty}); err == nil {
		t.Error("empty text was accepted")
	}
}

func TestDeleteTodo(t *testing.T) {
	s, p := projectWithTodos(t, "a", "b", "c")
	id := p.Todos[1].ID

	p, err := s.DeleteTodo(p.ID, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Todos) != 2 {
		t.Fatalf("%d tasks left", len(p.Todos))
	}
	if p.Todos[0].Text != "a" || p.Todos[1].Text != "c" {
		t.Errorf("the wrong task was deleted: %q, %q", p.Todos[0].Text, p.Todos[1].Text)
	}

	if _, err := s.DeleteTodo(p.ID, id); !errors.Is(err, ErrTodoNotFound) {
		t.Errorf("error = %v, wanted ErrTodoNotFound", err)
	}
}

func TestClearDoneTodos(t *testing.T) {
	s, p := projectWithTodos(t, "a", "b", "c")
	done := true
	for _, i := range []int{0, 2} {
		var err error
		if p, err = s.UpdateTodo(p.ID, p.Todos[i].ID, TodoPatch{Done: &done}); err != nil {
			t.Fatal(err)
		}
	}

	p, err := s.ClearDoneTodos(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Todos) != 1 || p.Todos[0].Text != "b" {
		t.Errorf("the wrong tasks were removed: %+v", p.Todos)
	}
}

func TestReorderTodos(t *testing.T) {
	s, p := projectWithTodos(t, "a", "b", "c")
	ids := []string{p.Todos[2].ID, p.Todos[0].ID, p.Todos[1].ID}

	p, err := s.ReorderTodos(p.ID, ids)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{p.Todos[0].Text, p.Todos[1].Text, p.Todos[2].Text}
	want := []string{"c", "a", "b"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, wanted %v", got, want)
		}
	}
}

// TestReorderKeepsOmittedTodos: a stale browser tab must not be able to delete
// tasks it has never seen just by sending a short list.
func TestReorderKeepsOmittedTodos(t *testing.T) {
	s, p := projectWithTodos(t, "a", "b", "c")

	p, err := s.ReorderTodos(p.ID, []string{p.Todos[1].ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Todos) != 3 {
		t.Fatalf("%d tasks left, wanted 3", len(p.Todos))
	}
	if p.Todos[0].Text != "b" {
		t.Errorf("the id that was sent did not move to the front: %q", p.Todos[0].Text)
	}
}

func TestReorderIgnoresUnknownAndDuplicateIDs(t *testing.T) {
	s, p := projectWithTodos(t, "a", "b")
	first := p.Todos[0].ID

	p, err := s.ReorderTodos(p.ID, []string{first, first, "imaginary-id"})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Todos) != 2 {
		t.Errorf("%d tasks — a duplicate id cloned a task", len(p.Todos))
	}
}

func TestTodoStats(t *testing.T) {
	s, p := projectWithTodos(t, "a", "b")
	high := PriorityHigh
	done := true

	p, _ = s.UpdateTodo(p.ID, p.Todos[0].ID, TodoPatch{Done: &done})
	p, _ = s.UpdateTodo(p.ID, p.Todos[1].ID, TodoPatch{Priority: &high})

	stats := p.TodoStats()
	if stats.Total != 2 || stats.Done != 1 || stats.High != 1 {
		t.Errorf("summary = %+v", stats)
	}

	// A high priority task that is already done is no longer urgent.
	p, _ = s.UpdateTodo(p.ID, p.Todos[1].ID, TodoPatch{Done: &done})
	if got := p.TodoStats().High; got != 0 {
		t.Errorf("a completed high priority task is still counted: %d", got)
	}
}

func TestTodosSurviveReopen(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Add(Project{Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddTodo(p.ID, "persistent task", PriorityHigh); err != nil {
		t.Fatal(err)
	}

	again, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := again.Get(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Todos) != 1 || reloaded.Todos[0].Text != "persistent task" {
		t.Errorf("the tasks were never written to disk: %+v", reloaded.Todos)
	}
	if reloaded.Todos[0].Priority != PriorityHigh {
		t.Errorf("the priority was lost: %q", reloaded.Todos[0].Priority)
	}
}

// TestTodosNeverNil: the UI iterates this list directly, so a null would be a
// runtime error in the browser rather than an empty section.
func TestTodosNeverNil(t *testing.T) {
	s := newStore(t)
	p, err := s.Add(Project{Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if p.Todos == nil {
		t.Error("the task list is nil on a new project")
	}
	if got, _ := s.Get(p.ID); got.Todos == nil {
		t.Error("the task list is nil on a project read back from disk")
	}
}

package tools

import (
	"context"
	"sync"
)

// BackgroundTask represents a task running in the background (e.g., a sub-agent).
type BackgroundTask struct {
	ID         string
	Ctx        context.Context
	Cancel     context.CancelFunc
	Done       chan struct{}
	Result     string
	Err        error
	OutputFile string
}

// BackgroundTaskStore manages background tasks shared by Agent, TaskOutput, and TaskStop tools.
type BackgroundTaskStore struct {
	mu    sync.Mutex
	tasks map[string]*BackgroundTask
}

// NewBackgroundTaskStore creates a new background task store.
func NewBackgroundTaskStore() *BackgroundTaskStore {
	return &BackgroundTaskStore{
		tasks: make(map[string]*BackgroundTask),
	}
}

// Add registers a background task.
func (s *BackgroundTaskStore) Add(task *BackgroundTask) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[task.ID] = task
}

// Get retrieves a background task by ID.
func (s *BackgroundTaskStore) Get(id string) (*BackgroundTask, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tasks[id]
	return t, ok
}

// Remove deletes a background task from the store.
func (s *BackgroundTaskStore) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tasks, id)
}

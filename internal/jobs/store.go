package jobs

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bee/java-process-mapper/internal/analysis"
	"github.com/bee/java-process-mapper/internal/output"
	"github.com/bee/java-process-mapper/internal/pipeline"
)

type Status string

const (
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

type Job struct {
	ID        string           `json:"jobId"`
	Status    Status           `json:"status"`
	Phase     string           `json:"phase"`
	Counts    map[string]int   `json:"counts,omitempty"`
	Error     string           `json:"error,omitempty"`
	OutputDir string           `json:"outputDir"`
	Artifacts output.Artifacts `json:"artifacts,omitempty"`
	Summary   any              `json:"summary,omitempty"`
	StartedAt time.Time        `json:"startedAt"`
	EndedAt   time.Time        `json:"endedAt,omitempty"`
}

type Store struct {
	mu   sync.RWMutex
	seq  int
	jobs map[string]*Job
}

func NewStore() *Store {
	return &Store{jobs: map[string]*Job{}}
}

func (s *Store) Start(ctx context.Context, opts analysis.Options) *Job {
	s.mu.Lock()
	s.seq++
	id := fmt.Sprintf("job-%d", s.seq)
	job := &Job{
		ID:        id,
		Status:    StatusRunning,
		Phase:     "queued",
		OutputDir: opts.OutputDir,
		StartedAt: time.Now().UTC(),
	}
	s.jobs[id] = job
	s.mu.Unlock()

	go func() {
		result, err := pipeline.Run(opts, func(phase string, counts map[string]int) {
			s.update(id, func(job *Job) {
				job.Phase = phase
				job.Counts = counts
			})
		})
		s.update(id, func(job *Job) {
			job.EndedAt = time.Now().UTC()
			if err != nil {
				job.Status = StatusFailed
				job.Phase = "failed"
				job.Error = err.Error()
				return
			}
			job.Status = StatusCompleted
			job.Phase = "completed"
			job.OutputDir = result.Artifacts.OutputDir
			job.Artifacts = result.Artifacts
			job.Summary = result.Summary
		})
	}()
	return s.Get(id)
}

func (s *Store) Get(id string) *Job {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[id]
	if !ok {
		return nil
	}
	copy := *job
	if job.Counts != nil {
		copy.Counts = map[string]int{}
		for k, v := range job.Counts {
			copy.Counts[k] = v
		}
	}
	return &copy
}

func (s *Store) update(id string, fn func(job *Job)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job, ok := s.jobs[id]; ok {
		fn(job)
	}
}

package config

import "sync"

type Config struct {
	PrinterName string
	IPPPort     int
	WebPort     int
	PaperWidth  string
	OutputDir   string
	Version     string
	SaveFormat  string // all, txt, html, raw
}

// Job representa um trabalho de impressão recebido
type Job struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	User       string `json:"user"`
	State      string `json:"state"` // pending, processing, completed, aborted
	Format     string `json:"format"`
	Pages      int    `json:"pages"`
	Data       []byte `json:"-"`
	ReceivedAt string `json:"received_at"`
	Size       int    `json:"size"`
	FilePath   string `json:"file_path"`
}

// JobStore armazena jobs em memória com mutex
type JobStore struct {
	mu      sync.RWMutex
	jobs    []*Job
	counter int
}

func NewJobStore() *JobStore {
	return &JobStore{jobs: make([]*Job, 0)}
}

func (s *JobStore) Add(j *Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counter++
	j.ID = s.counter
	s.jobs = append(s.jobs, j)
}

func (s *JobStore) All() []*Job {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := make([]*Job, len(s.jobs))
	copy(cp, s.jobs)
	return cp
}

func (s *JobStore) Get(id int) *Job {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, j := range s.jobs {
		if j.ID == id {
			return j
		}
	}
	return nil
}

func (s *JobStore) UpdateState(id int, state string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, j := range s.jobs {
		if j.ID == id {
			j.State = state
			return
		}
	}
}

func (s *JobStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs = make([]*Job, 0)
}

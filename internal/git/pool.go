package git

import (
	"context"
	"sync"
)

// Job represents a git operation to perform
type Job struct {
	ID      string // unique identifier (e.g., repo full name)
	Type    JobType
	URL     string // for clone
	Path    string // local path
	Options CloneOptions
	Reset   bool // for pull: reset on conflict
}

// JobType identifies the type of git operation
type JobType int

const (
	JobClone JobType = iota
	JobPull
	JobFetch
)

// JobResult contains the outcome of a git job
type JobResult struct {
	Job Job
	Err error
}

// Pool manages concurrent git operations
type Pool struct {
	workers int
	jobs    chan Job
	results chan JobResult
	wg      sync.WaitGroup
	ctx     context.Context
	cancel  context.CancelFunc
}

// PoolConfig configures the worker pool
type PoolConfig struct {
	Workers int // number of concurrent workers (default: 4, max: 10)
}

// DefaultPoolConfig returns sensible defaults
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		Workers: 4,
	}
}

// NewPool creates a new git operation pool
func NewPool(cfg PoolConfig) *Pool {
	if cfg.Workers <= 0 {
		cfg.Workers = 4
	}
	if cfg.Workers > 10 {
		cfg.Workers = 10
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Pool{
		workers: cfg.Workers,
		jobs:    make(chan Job, 100),
		results: make(chan JobResult, 100),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start begins processing jobs with the configured number of workers
func (p *Pool) Start(ctx context.Context) {
	// Use provided context for cancellation
	p.ctx, p.cancel = context.WithCancel(ctx)

	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.worker()
	}
}

// Submit adds a job to the queue
func (p *Pool) Submit(job Job) {
	select {
	case p.jobs <- job:
	case <-p.ctx.Done():
	}
}

// Results returns the results channel
func (p *Pool) Results() <-chan JobResult {
	return p.results
}

// Close signals no more jobs and waits for completion
func (p *Pool) Close() {
	close(p.jobs)
	p.wg.Wait()
	close(p.results)
}

// Cancel aborts all pending jobs
func (p *Pool) Cancel() {
	p.cancel()
	p.Close()
}

func (p *Pool) worker() {
	defer p.wg.Done()

	for {
		select {
		case job, ok := <-p.jobs:
			if !ok {
				return
			}
			result := p.execute(job)
			select {
			case p.results <- result:
			case <-p.ctx.Done():
				return
			}
		case <-p.ctx.Done():
			return
		}
	}
}

func (p *Pool) execute(job Job) JobResult {
	var err error

	switch job.Type {
	case JobClone:
		err = Clone(p.ctx, job.URL, job.Path, job.Options)
	case JobPull:
		err = Pull(p.ctx, job.Path, job.Reset)
	case JobFetch:
		err = Fetch(p.ctx, job.Path)
	}

	return JobResult{
		Job: job,
		Err: err,
	}
}

// RunAll executes all jobs and collects results
// This is a convenience method for batch operations
func RunAll(ctx context.Context, jobs []Job, workers int) []JobResult {
	if len(jobs) == 0 {
		return nil
	}

	results := make([]JobResult, 0, len(jobs))
	for result := range RunStream(ctx, jobs, workers) {
		results = append(results, result)
	}
	return results
}

// RunStream executes jobs and returns results as they complete.
// The returned channel is closed when all jobs finish.
func RunStream(ctx context.Context, jobs []Job, workers int) <-chan JobResult {
	out := make(chan JobResult)

	if len(jobs) == 0 {
		close(out)
		return out
	}

	pool := NewPool(PoolConfig{Workers: workers})
	pool.Start(ctx)

	// Submit all jobs then close input
	go func() {
		for _, job := range jobs {
			pool.Submit(job)
		}
		pool.Close()
	}()

	// Forward results until pool closes
	go func() {
		defer close(out)
		for r := range pool.Results() {
			select {
			case out <- r:
			case <-ctx.Done():
				return
			}
		}
	}()

	return out
}

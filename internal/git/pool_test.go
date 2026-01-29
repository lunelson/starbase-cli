package git

import (
	"context"
	"testing"
	"time"
)

func TestPoolConfig(t *testing.T) {
	tests := []struct {
		name     string
		workers  int
		expected int
	}{
		{"default when zero", 0, 4},
		{"respects positive", 3, 3},
		{"caps at max", 20, 10},
		{"negative becomes default", -1, 4},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pool := NewPool(PoolConfig{Workers: tc.workers})
			if pool.workers != tc.expected {
				t.Errorf("got %d workers, want %d", pool.workers, tc.expected)
			}
		})
	}
}

func TestPoolConcurrency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping concurrency test in short mode")
	}

	// Create test jobs
	numJobs := 10
	jobs := make([]Job, numJobs)
	for i := 0; i < numJobs; i++ {
		jobs[i] = Job{
			ID:   string(rune('A' + i)),
			Type: JobFetch, // Will fail but we just want to test concurrency
			Path: "/nonexistent",
		}
	}

	// Use RunAll which handles collection properly
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	results := RunAll(ctx, jobs, 4)

	if len(results) != numJobs {
		t.Errorf("got %d results, want %d", len(results), numJobs)
	}
}

func TestPoolCancel(t *testing.T) {
	pool := NewPool(PoolConfig{Workers: 2})

	ctx, cancel := context.WithCancel(context.Background())
	pool.Start(ctx)

	// Submit some jobs
	for i := 0; i < 5; i++ {
		pool.Submit(Job{
			ID:   string(rune('A' + i)),
			Type: JobFetch,
			Path: "/nonexistent",
		})
	}

	// Cancel immediately
	cancel()
	pool.Cancel()

	// Should complete without hanging
	// The test passing without timeout is the verification
}

func TestPoolContextCancellation(t *testing.T) {
	pool := NewPool(PoolConfig{Workers: 2})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	pool.Start(ctx)

	// Submit jobs (they'll fail anyway due to invalid paths)
	for i := 0; i < 100; i++ {
		pool.Submit(Job{
			ID:   string(rune(i)),
			Type: JobClone,
			URL:  "https://example.com/repo.git",
			Path: "/nonexistent/path",
		})
	}

	// Wait for context to cancel
	<-ctx.Done()
	pool.Cancel()

	// Test passes if it doesn't hang
}

func TestRunAllEmpty(t *testing.T) {
	results := RunAll(context.Background(), nil, 4)
	if results != nil {
		t.Errorf("expected nil results for empty jobs, got %v", results)
	}

	results = RunAll(context.Background(), []Job{}, 4)
	if results != nil {
		t.Errorf("expected nil results for empty slice, got %v", results)
	}
}

func TestRunAllCollectsAllResults(t *testing.T) {
	jobs := []Job{
		{ID: "a", Type: JobFetch, Path: "/nonexistent1"},
		{ID: "b", Type: JobFetch, Path: "/nonexistent2"},
		{ID: "c", Type: JobFetch, Path: "/nonexistent3"},
	}

	results := RunAll(context.Background(), jobs, 2)

	if len(results) != len(jobs) {
		t.Errorf("got %d results, want %d", len(results), len(jobs))
	}

	// All should have errors (invalid paths)
	for _, r := range results {
		if r.Err == nil {
			t.Errorf("expected error for job %s", r.Job.ID)
		}
	}
}

func TestJobTypes(t *testing.T) {
	// Verify job type constants are distinct
	if JobClone == JobPull || JobPull == JobFetch || JobClone == JobFetch {
		t.Error("job types should be distinct")
	}
}

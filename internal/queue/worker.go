package queue

import (
	"context"
	"log"
	"sync"

	"soroban-studio-backend/internal/executor"
	"soroban-studio-backend/internal/model"
)

// WorkerPool manages a pool of workers that process compilation jobs.
// It uses a buffered Go channel as a simple, efficient job queue.
// Concurrency is limited by the number of workers (default: 3).
type WorkerPool struct {
	jobs       chan model.Job
	wg         sync.WaitGroup
	executor   *executor.Executor
	workers    int
	activeJobs sync.Map // Map[string]context.CancelFunc (key: JobID)
}

// NewWorkerPool creates a new worker pool with the specified concurrency limit.
func NewWorkerPool(workers int, exec *executor.Executor) *WorkerPool {
	return &WorkerPool{
		jobs:       make(chan model.Job, 100), // buffered to prevent blocking callers
		executor:   exec,
		workers:    workers,
		activeJobs: sync.Map{},
	}
}

// Start launches the worker goroutines. Each worker continuously pulls
// jobs from the channel and processes them sequentially.
func (wp *WorkerPool) Start() {
	for i := 0; i < wp.workers; i++ {
		wp.wg.Add(1)
		go wp.worker(i)
	}
}

// Enqueue adds a job to the queue. Non-blocking as long as the buffer isn't full.
func (wp *WorkerPool) Enqueue(job model.Job) {
	wp.jobs <- job
}

// Kill stops a running job by its JobID.
func (wp *WorkerPool) Kill(jobID string) {
	if cancel, ok := wp.activeJobs.Load(jobID); ok {
		cancel.(context.CancelFunc)()
		wp.activeJobs.Delete(jobID)
	}
}

// Stop gracefully shuts down the worker pool by closing the channel
// and waiting for all in-progress jobs to complete.
func (wp *WorkerPool) Stop() {
	close(wp.jobs)
	wp.wg.Wait()
}

// worker is the main loop for each worker goroutine.
// It processes jobs until the channel is closed.
func (wp *WorkerPool) worker(id int) {
	defer wp.wg.Done()

	for job := range wp.jobs {
		// Create a cancellable context for this specific job
		ctx, cancel := context.WithCancel(context.Background())
		wp.activeJobs.Store(job.JobID, cancel)

		// Execute the job with the context
		wp.executor.Execute(ctx, job)

		// Cleanup after job completion
		cancel()
		wp.activeJobs.Delete(job.JobID)
	}
}

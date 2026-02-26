package services

import (
	"context"
	"log"
	"sync"
	"time"
)

// ProxyJob represents a proxy checking job
type ProxyJob struct {
	ID      int64
	Address string
	Timeout time.Duration
}

// ProxyResult represents the result of a proxy check
type ProxyResult struct {
	ID      int64
	Address string
	Alive   bool
	Type    string // http, https, socks4, socks5
	Latency int    // milliseconds
	Error   string
}

// WorkerPool manages concurrent proxy checking
type WorkerPool struct {
	workerCount int
	jobChan     chan ProxyJob
	resultChan  chan ProxyResult
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	checker     *ProxyChecker
	mu          sync.RWMutex
	running     bool
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(workerCount int, checker *ProxyChecker) *WorkerPool {
	return &WorkerPool{
		workerCount: workerCount,
		checker:     checker,
	}
}

// Start begins the worker pool (creates new channels each time)
func (p *WorkerPool) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Create fresh context and channels for each run
	p.ctx, p.cancel = context.WithCancel(context.Background())
	// Very large buffers to handle all jobs without blocking
	p.jobChan = make(chan ProxyJob, p.workerCount*100)
	p.resultChan = make(chan ProxyResult, p.workerCount*100)
	p.running = true

	log.Printf("WorkerPool: Starting %d workers", p.workerCount)

	for i := 0; i < p.workerCount; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}

	log.Printf("WorkerPool: %d workers started", p.workerCount)
}

// worker processes jobs from the job channel
func (p *WorkerPool) worker(id int) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Worker %d: recovered from panic: %v", id, r)
		}
		p.wg.Done()
	}()
	processed := 0
	// Process jobs until jobChan is closed
	for job := range p.jobChan {
		result := p.checker.CheckProxy(job)
		processed++

		// Send result - this will block until resultChan has space
		// DO NOT check ctx.Done() here - let workers finish all jobs
		p.resultChan <- result
	}
	log.Printf("Worker %d: finished (processed %d jobs)", id, processed)
}

// Submit adds a job to the pool with timeout
func (p *WorkerPool) Submit(job ProxyJob) bool {
	p.mu.RLock()
	running := p.running
	p.mu.RUnlock()

	if !running {
		return false
	}

	select {
	case p.jobChan <- job:
		return true
	case <-time.After(10 * time.Second):
		return false // Timeout
	}
}

// SubmitWait adds a job to the pool, waiting with timeout
func (p *WorkerPool) SubmitWait(job ProxyJob) bool {
	return p.Submit(job) // Same as Submit now
}

// SubmitBatch adds multiple jobs and returns count submitted
func (p *WorkerPool) SubmitBatch(jobs []ProxyJob) int {
	count := 0
	for _, job := range jobs {
		if p.Submit(job) {
			count++
		}
	}
	return count
}

// Results returns the result channel
func (p *WorkerPool) Results() <-chan ProxyResult {
	return p.resultChan
}

// Stop gracefully shuts down the worker pool
func (p *WorkerPool) Stop() {
	p.mu.Lock()
	p.running = false
	p.mu.Unlock()

	log.Println("WorkerPool: Stopping...")

	// Close jobChan first - signals workers to finish all jobs and exit naturally
	if p.jobChan != nil {
		close(p.jobChan)
	}

	// Wait for ALL workers to finish (they exit when jobChan closes AND all jobs processed)
	// Each job can take up to 40s (4 protocols × 10s)
	// With 500 workers and buffer of 5000, worst case: 500 jobs in progress + 4500 in queue
	// Give ample time: 5 minutes
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("WorkerPool: All workers finished")
	case <-time.After(5 * time.Minute):
		log.Println("WorkerPool: Timeout waiting for workers - forcing shutdown")
		// Force shutdown: close resultChan to unblock anyone waiting
		// cancel context if needed (but hopefully workers already done)
		if p.cancel != nil {
			p.cancel()
		}
	}

	// Now safe to close resultChan - all workers are done
	if p.resultChan != nil {
		close(p.resultChan)
	}
	log.Println("WorkerPool: Stopped")
}

// JobCount returns pending job count
func (p *WorkerPool) JobCount() int {
	if p.jobChan == nil {
		return 0
	}
	return len(p.jobChan)
}

// IsRunning returns whether the pool is running
func (p *WorkerPool) IsRunning() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.running
}

// GetPendingCount returns the number of pending jobs (thread-safe)
func (p *WorkerPool) GetPendingCount() int32 {
	if p.jobChan == nil {
		return 0
	}
	return int32(len(p.jobChan))
}

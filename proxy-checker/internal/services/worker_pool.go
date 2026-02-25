package services

import (
	"context"
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
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(workerCount int, checker *ProxyChecker) *WorkerPool {
	ctx, cancel := context.WithCancel(context.Background())
	return &WorkerPool{
		workerCount: workerCount,
		jobChan:     make(chan ProxyJob, 1000), // Bounded queue
		resultChan:  make(chan ProxyResult, 1000),
		ctx:         ctx,
		cancel:      cancel,
		checker:     checker,
	}
}

// Start begins the worker pool
func (p *WorkerPool) Start() {
	for i := 0; i < p.workerCount; i++ {
		p.wg.Add(1)
		go p.worker()
	}
}

// worker processes jobs from the job channel
func (p *WorkerPool) worker() {
	defer p.wg.Done()
	for {
		select {
		case <-p.ctx.Done():
			return
		case job, ok := <-p.jobChan:
			if !ok {
				return
			}
			result := p.checker.CheckProxy(job)
			select {
			case p.resultChan <- result:
			case <-p.ctx.Done():
				return
			}
		}
	}
}

// Submit adds a job to the pool (non-blocking)
func (p *WorkerPool) Submit(job ProxyJob) bool {
	select {
	case p.jobChan <- job:
		return true
	default:
		return false // Queue full
	}
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
	p.cancel()
	close(p.jobChan)
	p.wg.Wait()
	close(p.resultChan)
}

// JobCount returns pending job count
func (p *WorkerPool) JobCount() int {
	return len(p.jobChan)
}

package workerpool

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
)

// Buffer size for the job queue
const JobQueueBufferSize = 10000

// Job represents a unit of work to be done
type Job func() error

// Worker represents a worker that can execute jobs
type Worker struct {
	id         int
	jobQueue   <-chan Job
	quit       chan struct{}
	workerPool chan<- chan Job
	jobChannel chan Job
	wg         *sync.WaitGroup

	pool *Pool
}

// NewWorker creates a new worker
func NewWorker(id int, workerPool chan<- chan Job, wg *sync.WaitGroup, pool *Pool) *Worker {
	return &Worker{
		id:         id,
		workerPool: workerPool,
		jobChannel: make(chan Job),
		quit:       make(chan struct{}),
		wg:         wg,

		pool: pool,
	}
}

// Start method starts the run loop for the worker
func (w *Worker) Start() {
	go func() {
		for {
			w.workerPool <- w.jobChannel
			select {
			case job := <-w.jobChannel:
				if err := job(); err != nil {
					log.Printf("Worker %d encountered an error: %v", w.id, err)
				}
				w.pool.wg.Done()
			case <-w.quit:
				return
			}
		}
	}()
}

// Start helper function
func (w *Worker) run() {
	for {
		select {
		case <-w.quit:
			log.Printf("Worker %d is stopping", w.id)
			return
		default:
			w.waitForJob()
		}
	}
}

// Start helper function
func (w *Worker) waitForJob() {
	w.workerPool <- w.jobChannel
	log.Printf("Worker %d is ready to receive a job", w.id)

	select {
	case job := <-w.jobChannel:
		w.processJob(job)
	case <-w.quit:
		log.Printf("Worker %d is stopping", w.id)
	}
}

// Start helper function
func (w *Worker) processJob(job func() error) {
	log.Printf("Worker %d received a job", w.id)
	if err := job(); err != nil {
		log.Printf("Worker %d encountered an error: %v", w.id, err)
	}
	log.Printf("Worker %d completed the job", w.id)
	w.wg.Done()
}

// Stop tells the worker to stop listening for work requests.
func (w *Worker) Stop() {
	close(w.quit)
	// FIX: is this really needed?
	// Wait for the worker to fully stop
	go func() {
		<-w.jobChannel
		log.Printf("Worker %d is fully stopped", w.id)
	}()
}

// Pool represents a worker pool
type Pool struct {
	workers    []*Worker
	workerPool chan chan Job
	//jobQueue     chan Job
	boundedQueue   chan Job
	unboundedQueue chan Job
	quit           chan struct{}
	maxWorkers     int
	wg             sync.WaitGroup
	//queueTracker   int
	mu sync.Mutex

	// Experimental
	stopping int32
}

// NewPool creates a new worker pool
func NewPool(maxWorkers int) *Pool {
	return &Pool{
		workerPool: make(chan chan Job, maxWorkers),
		//jobQueue:   make(chan Job, JobQueueBufferSize),
		boundedQueue:   make(chan Job, JobQueueBufferSize),
		unboundedQueue: make(chan Job, 0),
		quit:           make(chan struct{}),
		maxWorkers:     maxWorkers,
	}
}

// Start starts the pool
func (p *Pool) Start() {
	log.Printf("Starting pool with %d workers", p.maxWorkers)
	for i := 0; i < p.maxWorkers; i++ {
		worker := NewWorker(i+1, p.workerPool, &p.wg, p)
		p.workers = append(p.workers, worker)
		worker.Start()
	}

	go p.dispatch()

	// Background goroutine to move jobs from unbounded to bounded queue when possible
	go func() {
		for {
			select {
			case job := <-p.unboundedQueue:
				select {
				case p.boundedQueue <- job:
					// Successfully moved a job to bounded queue
				case <-p.quit:
					return
				}
			case <-p.quit:
				return
			}
		}
	}()
}

// AddWorker
func (p *Pool) AddWorker() {
	p.mu.Lock()
	defer p.mu.Unlock()

	workerID := len(p.workers) + 1
	worker := NewWorker(workerID, p.workerPool, &p.wg, p)
	p.workers = append(p.workers, worker)
	worker.Start()
	log.Printf("Added worker %d, total workers: %d", workerID, len(p.workers))
}

// RemoveWorker removes the last worker from the pool
func (p *Pool) RemoveWorker() {
	p.mu.Lock()
	if len(p.workers) == 0 {
		log.Println("No workers to remove")
		p.mu.Unlock()
		return
	}

	worker := p.workers[len(p.workers)-1]
	p.workers = p.workers[:len(p.workers)-1]
	p.mu.Unlock()

	// Ensure the worker is fully stopped before proceeding
	worker.Stop()

	log.Printf("Removed worker %d, total workers: %d", worker.id, len(p.workers))
}

// Revised: The dispatch method runs in its own goroutine
func (p *Pool) dispatch() {
	for {
		select {
		//case job, ok := <-p.jobQueue:
		case job, ok := <-p.boundedQueue:
			if !ok {
				return // jobQueue has been closed
			}
			//p.incrementQueue(-1)
			select {
			case jobChannel := <-p.workerPool:
				jobChannel <- job
			case <-p.quit:
				p.wg.Done() // Ensure we don't leave the job unaccounted for
				return
			}
		case <-p.quit:
			return
		}
	}
}

// Submit adds a job to the job queue
func (p *Pool) Submit(job Job) {
	if atomic.LoadInt32(&p.stopping) == 1 {
		log.Println("Pool is stopping, job not submitted")
		return
	}

	p.wg.Add(1)
	select {
	/*case p.jobQueue <- job:
	  p.incrementQueue(1)
	  log.Println("Submitting a job to the queue")
	*/
	case p.boundedQueue <- job:
		//p.incrementQueue(1)
		log.Println("Submitting a job to the queue")
	case p.unboundedQueue <- job:
		log.Println("Submitting a job to the unbounded Queue")
	default:
		p.wg.Done()
		log.Println("Job queue is full, job not submitted")
	}
}

// Stop stops all workers, the dispatcher and waits for all jobs to complete
func (p *Pool) Stop() {
	if atomic.CompareAndSwapInt32(&p.stopping, 0, 1) {
		return // Pool is already stopping
	}

	close(p.quit)

	p.wg.Wait()
	//close(p.jobQueue)
	close(p.boundedQueue)
	close(p.unboundedQueue)
	close(p.workerPool)

	log.Println("All workers have finished")

}

// Wait waits for all submitted jobs to complete
func (p *Pool) Wait() {
	log.Println("Waiting for all jobs to complete")
	p.wg.Wait()
	log.Println("All jobs have completed")
}

// WithContext returns a new Pool that will be canceled when the context is done
func (p *Pool) WithContext(ctx context.Context) *Pool {
	newPool := NewPool(p.maxWorkers)
	newPool.Start()
	go func() {
		<-ctx.Done()
		newPool.Stop()
	}()
	return newPool
}

// IMPORTANT: To be tested (important)
// Experimental LoadBalancer
type LoadBalancer struct {
	pools    []*Pool
	jobQueue chan Job
	stopChan chan struct{}
	wg       sync.WaitGroup
	poolSize int
	numPools int
	counter  uint64 // Use a counter to track the pool index
}

// NewLoadBalancer creates a new load balancer
func NewLoadBalancer(poolSize, numPools int) *LoadBalancer {
	lb := &LoadBalancer{
		pools:    make([]*Pool, numPools),
		jobQueue: make(chan Job, JobQueueBufferSize*numPools*100),
		stopChan: make(chan struct{}),
		poolSize: poolSize,
		numPools: numPools,
		counter:  0, // Initialize the counter
	}
	for i := 0; i < numPools; i++ {
		lb.pools[i] = NewPool(poolSize)
	}

	return lb
}

// Start starts the load balancer and all pools
func (lb *LoadBalancer) Start() {
	for _, pool := range lb.pools {
		pool.Start()
	}
	go lb.balance()
}

// balance distributes jobs among pools using round-robin
func (lb *LoadBalancer) balance() {
	for {
		select {
		case job := <-lb.jobQueue:
			poolIndex := atomic.AddUint64(&lb.counter, 1) % uint64(lb.numPools)
			lb.pools[poolIndex].Submit(job)
		case <-lb.stopChan:
			return
		}
	}
}

// Submit adds a job to the load balancer's queue
func (lb *LoadBalancer) Submit(job Job) {
	select {
	case lb.jobQueue <- job:
		// Job submitted successfully
	default:
		log.Println("Load balancer queue is full, job not submitted")
		go job()
	}
}

// Stop stops the load balancer and all pools
func (lb *LoadBalancer) Stop() {
	close(lb.stopChan)
	for _, pool := range lb.pools {
		pool.Stop()
	}
	//lb.wg.Wait()
}

// Wait waits for all submitted jobs to complete
func (lb *LoadBalancer) Wait() {
	//lb.wg.Wait()
	for _, pool := range lb.pools {
		pool.Wait()
	}
}

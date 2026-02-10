package workerpool

import (
	"math/rand"
	"sync"
    "sync/atomic"
	"testing"
	"time"
)

func TestPoolMultipleJobs(t *testing.T) {
	pool := NewPool(3) // 3 workers
	pool.Start()

	var wg sync.WaitGroup
	jobExecuted := make([]bool, 5) // Tracking execution of each job

	// Define 5 jobs
	for i := 0; i < 5; i++ {
		wg.Add(1)
		jobIndex := i
		job := func() error {
			defer wg.Done()
			jobExecuted[jobIndex] = true
			return nil
		}
		pool.Submit(job)
	}

	wg.Wait()
	pool.Stop()

	// Assert all jobs have been executed
	for i, executed := range jobExecuted {
		if !executed {
			t.Errorf("Job %d was not executed", i)
		}
	}
}

func TestPoolJobParallelism(t *testing.T) {
	pool := NewPool(3) // 3 workers
	pool.Start()

	var wg sync.WaitGroup
	startTime := time.Now()

	// Define 3 jobs that take 1 second each
	for i := 0; i < 3; i++ {
		wg.Add(1)
		job := func() error {
			defer wg.Done()
			time.Sleep(1 * time.Second)
			return nil
		}
		pool.Submit(job)
	}

	wg.Wait()
	elapsedTime := time.Since(startTime)
	pool.Stop()

	// Assert that the jobs were processed in parallel (should take around 1 second)
	if elapsedTime >= 2*time.Second {
		t.Errorf("Jobs were not processed in parallel, took too long: %v", elapsedTime)
	}
}

func TestPoolStop(t *testing.T) {
	pool := NewPool(3)
	pool.Start()

	var wg sync.WaitGroup
	jobExecuted := false

	// Define a job that takes time to execute
	job := func() error {
		defer wg.Done()
		time.Sleep(500 * time.Millisecond)
		jobExecuted = true
		return nil
	}

	wg.Add(1)
	pool.Submit(job)

	time.Sleep(100 * time.Millisecond)
	pool.Stop()

	wg.Wait()

	// Assert that the job was executed before stopping
	if !jobExecuted {
		t.Errorf("Job was not executed before pool stopped")
	}
}

func TestPoolWorkerComparison(t *testing.T) {
	jobCount := 50

	// Define a job that takes 100ms to execute
	job := func() error {
		time.Sleep(100 * time.Millisecond)
		return nil
	}

	testWorkerPool := func(workerCount int) time.Duration {
		t.Logf("Starting test with %d workers...", workerCount)
		startTime := time.Now()
		pool := NewPool(workerCount)
		pool.Start()

		var wg sync.WaitGroup
		for i := 0; i < jobCount; i++ {
			wg.Add(1)
			pool.Submit(func() error {
				defer wg.Done()
				return job()
			})
		}

		wg.Wait()
		pool.Stop()
		elapsedTime := time.Since(startTime)

		t.Logf("%d workers finished all jobs in %v", workerCount, elapsedTime)
		return elapsedTime
	}

	// Test with different worker pool sizes
	elapsedTimeSingleWorker := testWorkerPool(1)
	elapsedTimeFiveWorkers := testWorkerPool(5)
	elapsedTimeTenWorkers := testWorkerPool(10)
	elapsedTimeFiftyWorkers := testWorkerPool(50)

	// Ensure that increasing workers reduces execution time
	if elapsedTimeFiveWorkers >= elapsedTimeSingleWorker {
		t.Errorf("5 workers did not finish faster than 1 worker; 5 workers took %v, 1 worker took %v", elapsedTimeFiveWorkers, elapsedTimeSingleWorker)
	}

	if elapsedTimeTenWorkers >= elapsedTimeFiveWorkers {
		t.Errorf("10 workers did not finish faster than 5 workers; 10 workers took %v, 5 workers took %v", elapsedTimeTenWorkers, elapsedTimeFiveWorkers)
	}

	if elapsedTimeFiftyWorkers >= elapsedTimeTenWorkers {
		t.Errorf("50 workers did not finish faster than 10 workers; 50 workers took %v, 10 workers took %v", elapsedTimeFiftyWorkers, elapsedTimeTenWorkers)
	}
}

func TestRealWorldWorkerPoolScenario(t *testing.T) {
	// Create a pool with 3 workers initially
	pool := NewPool(3)
	pool.Start()

	// Job function to simulate work
	job := func() error {
		time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond) // Simulate some work
		return nil
	}

	// Step 1: Submit initial jobs
	for i := 0; i < 5; i++ {
		pool.Submit(job)
	}
	t.Log("Submitted initial jobs")

	// Step 2: Randomly add and remove workers, and submit jobs in between
	for i := 0; i < 10; i++ { // Adjust iteration count to increase complexity
		action := rand.Intn(3)
		switch action {
		case 0:
			// Add a worker
			pool.AddWorker()
			t.Logf("Added a worker, total workers: %d", len(pool.workers))
		case 1:
			// Remove a worker
			pool.RemoveWorker()
			t.Logf("Removed a worker, total workers: %d", len(pool.workers))
		case 2:
			// Submit more jobs
			for j := 0; j < rand.Intn(5)+1; j++ {
				pool.Submit(job)
			}
			t.Logf("Submitted additional jobs")
		}

		// Random short sleep to simulate real-time operations
		time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)
	}

	// Step 3: Final job submissions after all worker adjustments
	for i := 0; i < 10; i++ {
		pool.Submit(job)
		time.Sleep(100 * time.Millisecond)
	}
	t.Log("Submitted final jobs")

	// Stop the pool after all operations
	pool.Stop()
	t.Log("Pool stopped successfully")
}

func TestPoolParallelExecution(t *testing.T) {
	// Create a new random source and generator
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	pool := NewPool(5) // 5 workers
	pool.Start()

	var wg sync.WaitGroup
	jobCount := 10
	executionTimes := make([]time.Duration, jobCount)

	// Define jobs with random execution times
	for i := 0; i < jobCount; i++ {
		wg.Add(1)
		jobIndex := i
		job := func() error {
			defer wg.Done()
			sleepTime := time.Duration(rng.Intn(1000)) * time.Millisecond
			time.Sleep(sleepTime)
			executionTimes[jobIndex] = sleepTime
			return nil
		}
		pool.Submit(job)
	}

	startTime := time.Now()
	wg.Wait()
	elapsedTime := time.Since(startTime)
	pool.Stop()

	// Calculate the total execution time of all jobs if they were run sequentially
	totalSequentialTime := time.Duration(0)
	for _, execTime := range executionTimes {
		totalSequentialTime += execTime
	}

	// Assert that the jobs were processed in parallel (total execution time should be less than the sum of all execution times)
	if elapsedTime >= totalSequentialTime {
		t.Errorf("Jobs did not run in parallel; expected less than %v, got %v", totalSequentialTime, elapsedTime)
	} else {
		t.Logf("Jobs ran in parallel; expected less than %v, got %v", totalSequentialTime, elapsedTime)
	}
}

func TestLoadBalancer(t *testing.T) {
	poolSize := 3 
	numPools := 5 
	numJobs := 50 

	// Create a new LoadBalancer with 4 pools, each having 3 workers
	lb := NewLoadBalancer(poolSize, numPools)
	lb.Start()

	var processedJobs int32

	// Job function to simulate work and count processed jobs
	job := func() error {
		atomic.AddInt32(&processedJobs, 1)
		time.Sleep(100 * time.Millisecond) // Simulate some work
		return nil
	}

	// Submit multiple jobs to the LoadBalancer
    for i := 0; i < numJobs; i++ {
	   lb.Submit(job)
       time.Sleep(1 * time.Millisecond) // Needed for synchronization
	}

	// Wait for all jobs to complete
	lb.Wait()

	// Stop the LoadBalancer
	lb.Stop()

	// Verify that all jobs were processed
	if processedJobs != int32(numJobs) {
		t.Errorf("Expected %d jobs to be processed, but got %d", numJobs, processedJobs)
	} else {
		t.Logf("All %d jobs were processed successfully", numJobs)
	}

	// Verify that all pools have stopped
	for _, pool := range lb.pools {
		if len(pool.workers) != poolSize {
			t.Errorf("Expected %d workers per pool, but got %d", poolSize, len(pool.workers))
		}
	}
}

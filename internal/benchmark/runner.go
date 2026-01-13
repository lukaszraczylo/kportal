package benchmark

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// ProgressCallback is called periodically with benchmark progress
type ProgressCallback func(completed, total int)

// Config holds the benchmark configuration
type Config struct {
	Headers          map[string]string
	ProgressCallback ProgressCallback
	URL              string
	Method           string
	Body             []byte
	Concurrency      int
	Requests         int
	Duration         time.Duration
	Timeout          time.Duration
}

// Runner executes HTTP benchmarks
type Runner struct {
	client *http.Client
}

// NewRunner creates a new benchmark runner
func NewRunner() *Runner {
	return &Runner{
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 100,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// Run executes the benchmark and returns results
func (r *Runner) Run(ctx context.Context, forwardID string, cfg Config) (*Results, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("URL is required")
	}

	if cfg.Concurrency < 1 {
		cfg.Concurrency = 1
	}

	// Ensure concurrency doesn't exceed number of requests (for request-based mode)
	if cfg.Duration == 0 && cfg.Requests > 0 && cfg.Concurrency > cfg.Requests {
		cfg.Concurrency = cfg.Requests
	}

	if cfg.Timeout > 0 {
		r.client.Timeout = cfg.Timeout
	}

	results := NewResults(forwardID, cfg.URL, cfg.Method)

	// Create work channel
	workCh := make(chan struct{}, cfg.Concurrency*2)

	// Create context for cancellation
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Start workers
	var wg sync.WaitGroup
	var completed int64
	var resultsMu sync.Mutex // Shared mutex for results access

	for i := 0; i < cfg.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.worker(runCtx, cfg, results, &resultsMu, workCh, &completed)
		}()
	}

	// Start progress reporter if callback is provided
	if cfg.ProgressCallback != nil {
		go func() {
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-runCtx.Done():
					return
				case <-ticker.C:
					cfg.ProgressCallback(int(atomic.LoadInt64(&completed)), cfg.Requests)
				}
			}
		}()
	}

	// Determine how to dispatch work
	if cfg.Duration > 0 {
		// Duration-based: keep sending work until duration expires
		timer := time.NewTimer(cfg.Duration)
		defer timer.Stop()

	dispatchLoop:
		for {
			select {
			case <-timer.C:
				cancel()
				break dispatchLoop
			case <-ctx.Done():
				cancel()
				break dispatchLoop
			case workCh <- struct{}{}:
				// Work dispatched
			}
		}
	} else {
		// Request-based: send exactly N requests
	requestLoop:
		for i := 0; i < cfg.Requests; i++ {
			select {
			case <-ctx.Done():
				cancel()
				break requestLoop
			case workCh <- struct{}{}:
				// Work dispatched
			}
		}
	}

	// Close work channel and wait for workers
	close(workCh)
	wg.Wait()

	results.Finalize()
	return results, nil
}

// worker processes requests from the work channel
func (r *Runner) worker(ctx context.Context, cfg Config, results *Results, resultsMu *sync.Mutex, workCh <-chan struct{}, completed *int64) {
	for range workCh {
		select {
		case <-ctx.Done():
			return
		default:
		}

		start := time.Now()
		statusCode, bytesRead, bytesWritten, err := r.makeRequestSafe(ctx, cfg)
		latency := time.Since(start)

		resultsMu.Lock()
		if err != nil {
			results.RecordFailure(err, latency)
		} else {
			results.RecordSuccess(statusCode, latency, bytesRead, bytesWritten)
		}
		resultsMu.Unlock()

		atomic.AddInt64(completed, 1)
	}
}

// makeRequestSafe wraps makeRequest with panic recovery
func (r *Runner) makeRequestSafe(ctx context.Context, cfg Config) (statusCode int, bytesRead, bytesWritten int64, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("request panic: %v", rec)
		}
	}()
	return r.makeRequest(ctx, cfg)
}

// makeRequest makes a single HTTP request
func (r *Runner) makeRequest(ctx context.Context, cfg Config) (statusCode int, bytesRead, bytesWritten int64, err error) {
	var body io.Reader
	if len(cfg.Body) > 0 {
		body = bytes.NewReader(cfg.Body)
		bytesWritten = int64(len(cfg.Body))
	}

	req, err := http.NewRequestWithContext(ctx, cfg.Method, cfg.URL, body)
	if err != nil {
		return 0, 0, 0, err
	}

	// Set headers
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return 0, 0, bytesWritten, err
	}
	defer resp.Body.Close()

	// Read response body to measure bytes
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, 0, bytesWritten, err
	}

	return resp.StatusCode, int64(len(respBody)), bytesWritten, nil
}

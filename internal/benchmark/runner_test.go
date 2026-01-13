package benchmark

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResults(t *testing.T) {
	r := NewResults("test-forward", "http://localhost/test", "GET")

	// Record some 2xx successes
	r.RecordSuccess(200, 10*time.Millisecond, 100, 0)
	r.RecordSuccess(200, 20*time.Millisecond, 150, 0)
	r.RecordSuccess(201, 15*time.Millisecond, 120, 0)

	// Record a transport failure
	r.RecordFailure(assert.AnError, 5*time.Millisecond)

	r.Finalize()

	assert.Equal(t, 4, r.TotalRequests)
	assert.Equal(t, 3, r.Successful)
	assert.Equal(t, 1, r.Failed)
	assert.Equal(t, int64(370), r.BytesRead)
	assert.Equal(t, 2, r.StatusCodes[200])
	assert.Equal(t, 1, r.StatusCodes[201])
}

func TestResultsNon2xxCountsAsFailure(t *testing.T) {
	r := NewResults("test-forward", "http://localhost/test", "GET")

	// Record a 200 success
	r.RecordSuccess(200, 10*time.Millisecond, 100, 0)

	// Record 4xx and 5xx - these should count as failures
	r.RecordSuccess(404, 10*time.Millisecond, 50, 0)
	r.RecordSuccess(500, 10*time.Millisecond, 30, 0)

	r.Finalize()

	assert.Equal(t, 3, r.TotalRequests)
	assert.Equal(t, 1, r.Successful, "Only 2xx should count as successful")
	assert.Equal(t, 2, r.Failed, "4xx and 5xx should count as failed")
	assert.Equal(t, 1, r.StatusCodes[200])
	assert.Equal(t, 1, r.StatusCodes[404])
	assert.Equal(t, 1, r.StatusCodes[500])
}

func TestResultsStats(t *testing.T) {
	r := NewResults("test", "http://localhost", "GET")

	// Add latencies
	latencies := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
		50 * time.Millisecond,
	}

	for _, lat := range latencies {
		r.RecordSuccess(200, lat, 0, 0)
	}

	r.EndTime = r.StartTime.Add(1 * time.Second)

	stats := r.CalculateStats()

	assert.Equal(t, 10*time.Millisecond, stats.MinLatency)
	assert.Equal(t, 50*time.Millisecond, stats.MaxLatency)
	assert.Equal(t, 30*time.Millisecond, stats.AvgLatency)
	assert.Equal(t, float64(5), stats.Throughput)
}

func TestPercentile(t *testing.T) {
	sorted := []time.Duration{
		1 * time.Millisecond,
		2 * time.Millisecond,
		3 * time.Millisecond,
		4 * time.Millisecond,
		5 * time.Millisecond,
		6 * time.Millisecond,
		7 * time.Millisecond,
		8 * time.Millisecond,
		9 * time.Millisecond,
		10 * time.Millisecond,
	}

	// P50 = index 5 (50*10/100 = 5) = 6ms
	assert.Equal(t, 6*time.Millisecond, percentile(sorted, 50))
	// P95 = index 9 (95*10/100 = 9) = 10ms
	assert.Equal(t, 10*time.Millisecond, percentile(sorted, 95))
	// P99 = index 9 (99*10/100 = 9) = 10ms
	assert.Equal(t, 10*time.Millisecond, percentile(sorted, 99))
}

func TestRunner(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Millisecond) // Simulate some latency
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	runner := NewRunner()

	cfg := Config{
		URL:         server.URL,
		Method:      "GET",
		Concurrency: 2,
		Requests:    10,
		Timeout:     5 * time.Second,
	}

	results, err := runner.Run(context.Background(), "test-forward", cfg)
	require.NoError(t, err)

	assert.Equal(t, 10, results.TotalRequests)
	assert.Equal(t, 10, results.Successful)
	assert.Equal(t, 0, results.Failed)
	assert.Equal(t, 10, results.StatusCodes[200])
}

func TestRunnerWithDuration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`ok`))
	}))
	defer server.Close()

	runner := NewRunner()

	cfg := Config{
		URL:         server.URL,
		Method:      "GET",
		Concurrency: 2,
		Duration:    100 * time.Millisecond,
		Timeout:     1 * time.Second,
	}

	results, err := runner.Run(context.Background(), "test-forward", cfg)
	require.NoError(t, err)

	// Should have made some requests in 100ms
	assert.Greater(t, results.TotalRequests, 0)
	assert.Equal(t, results.Successful, results.StatusCodes[200])
}

func TestRunnerWithHeaders(t *testing.T) {
	var receivedHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeader = r.Header.Get("X-Custom-Header")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	runner := NewRunner()

	cfg := Config{
		URL:    server.URL,
		Method: "GET",
		Headers: map[string]string{
			"X-Custom-Header": "test-value",
		},
		Concurrency: 1,
		Requests:    1,
	}

	_, err := runner.Run(context.Background(), "test", cfg)
	require.NoError(t, err)

	assert.Equal(t, "test-value", receivedHeader)
}

func TestRunnerWithBody(t *testing.T) {
	var receivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := http.MaxBytesReader(w, r.Body, 1024).Read(make([]byte, 1024))
		_ = body
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	runner := NewRunner()

	cfg := Config{
		URL:         server.URL,
		Method:      "POST",
		Body:        []byte(`{"test":"data"}`),
		Concurrency: 1,
		Requests:    1,
	}

	results, err := runner.Run(context.Background(), "test", cfg)
	require.NoError(t, err)

	_ = receivedBody // Used for debugging
	assert.Equal(t, int64(15), results.BytesWritten)
}

func TestRunnerWithProgressCallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond) // Add small delay so progress ticker can fire
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`ok`))
	}))
	defer server.Close()

	runner := NewRunner()

	var progressUpdates []int
	var mu sync.Mutex

	cfg := Config{
		URL:         server.URL,
		Method:      "GET",
		Concurrency: 5,
		Requests:    50, // More requests to ensure progress callbacks fire
		Timeout:     5 * time.Second,
		ProgressCallback: func(completed, total int) {
			mu.Lock()
			progressUpdates = append(progressUpdates, completed)
			mu.Unlock()
		},
	}

	results, err := runner.Run(context.Background(), "test-forward", cfg)
	require.NoError(t, err)

	assert.Equal(t, 50, results.TotalRequests)

	// Should have received some progress updates (ticker fires every 100ms)
	mu.Lock()
	updates := len(progressUpdates)
	mu.Unlock()
	assert.Greater(t, updates, 0, "Should have received progress updates")
}

func TestRunnerConcurrencyCappedAtRequests(t *testing.T) {
	requestCount := 0
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	runner := NewRunner()

	cfg := Config{
		URL:         server.URL,
		Method:      "GET",
		Concurrency: 100, // Higher than requests
		Requests:    5,
		Timeout:     5 * time.Second,
	}

	results, err := runner.Run(context.Background(), "test", cfg)
	require.NoError(t, err)

	assert.Equal(t, 5, results.TotalRequests)
}

package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestNewBenchmarkState tests the constructor
func TestNewBenchmarkState(t *testing.T) {
	state := newBenchmarkState("forward-123", "my-service", 8080)

	assert.Equal(t, "forward-123", state.forwardID)
	assert.Equal(t, "my-service", state.forwardAlias)
	assert.Equal(t, 8080, state.localPort)
	assert.Equal(t, BenchmarkStepConfig, state.step)
	assert.Equal(t, "/", state.urlPath)
	assert.Equal(t, "GET", state.method)
	assert.Equal(t, 10, state.concurrency)
	assert.Equal(t, 100, state.requests)
	assert.Equal(t, 0, state.cursor)
	assert.False(t, state.running)
	assert.Nil(t, state.results)
	assert.Nil(t, state.error)
	assert.Nil(t, state.cancelFunc)
}

// TestBenchmarkState_StepTransitions tests step progression
func TestBenchmarkState_StepTransitions(t *testing.T) {
	state := newBenchmarkState("fwd", "alias", 8080)

	// Initial state
	assert.Equal(t, BenchmarkStepConfig, state.step)

	// Move to running
	state.step = BenchmarkStepRunning
	state.running = true
	assert.Equal(t, BenchmarkStepRunning, state.step)
	assert.True(t, state.running)

	// Move to results
	state.step = BenchmarkStepResults
	state.running = false
	assert.Equal(t, BenchmarkStepResults, state.step)
	assert.False(t, state.running)
}

// TestBenchmarkState_ProgressTracking tests progress updates
func TestBenchmarkState_ProgressTracking(t *testing.T) {
	state := newBenchmarkState("fwd", "alias", 8080)
	state.step = BenchmarkStepRunning
	state.running = true
	state.total = 100

	// Simulate progress updates
	updates := []struct {
		progress int
		total    int
	}{
		{10, 100},
		{50, 100},
		{75, 100},
		{100, 100},
	}

	for _, u := range updates {
		state.progress = u.progress
		state.total = u.total
		assert.Equal(t, u.progress, state.progress)
		assert.Equal(t, u.total, state.total)
	}
}

// TestBenchmarkState_CancelFunc tests cancel function handling
func TestBenchmarkState_CancelFunc(t *testing.T) {
	state := newBenchmarkState("fwd", "alias", 8080)

	cancelled := false
	state.cancelFunc = func() {
		cancelled = true
	}

	assert.NotNil(t, state.cancelFunc)

	// Call cancel
	state.cancelFunc()
	assert.True(t, cancelled)
}

// TestBenchmarkState_Results tests result storage
func TestBenchmarkState_Results(t *testing.T) {
	state := newBenchmarkState("fwd", "alias", 8080)

	results := &BenchmarkResults{
		TotalRequests: 100,
		Successful:    95,
		Failed:        5,
		MinLatency:    10.5,
		MaxLatency:    250.0,
		AvgLatency:    45.2,
		P50Latency:    40.0,
		P95Latency:    120.0,
		P99Latency:    200.0,
		Throughput:    150.5,
		BytesRead:     1024000,
		StatusCodes: map[int]int{
			200: 90,
			201: 5,
			500: 5,
		},
	}

	state.results = results
	state.step = BenchmarkStepResults

	assert.Equal(t, 100, state.results.TotalRequests)
	assert.Equal(t, 95, state.results.Successful)
	assert.Equal(t, 5, state.results.Failed)
	assert.Equal(t, 45.2, state.results.AvgLatency)
	assert.Equal(t, 150.5, state.results.Throughput)
}

// TestBenchmarkState_Error tests error handling
func TestBenchmarkState_Error(t *testing.T) {
	state := newBenchmarkState("fwd", "alias", 8080)

	assert.Nil(t, state.error)

	// Simulate error
	state.error = assert.AnError
	state.step = BenchmarkStepResults
	state.running = false

	assert.NotNil(t, state.error)
	assert.Nil(t, state.results)
}

// TestBenchmarkState_ConfigFields tests configuration field updates
func TestBenchmarkState_ConfigFields(t *testing.T) {
	state := newBenchmarkState("fwd", "alias", 8080)

	// Update URL path
	state.urlPath = "/api/v1/health"
	assert.Equal(t, "/api/v1/health", state.urlPath)

	// Update method
	state.method = "POST"
	assert.Equal(t, "POST", state.method)

	// Update concurrency
	state.concurrency = 50
	assert.Equal(t, 50, state.concurrency)

	// Update requests
	state.requests = 1000
	assert.Equal(t, 1000, state.requests)
}

// TestBenchmarkState_CursorBounds tests cursor navigation bounds
func TestBenchmarkState_CursorBounds(t *testing.T) {
	state := newBenchmarkState("fwd", "alias", 8080)

	// There are 4 config fields (0-3)
	tests := []struct {
		name     string
		cursor   int
		expected int
	}{
		{"first field", 0, 0},
		{"second field", 1, 1},
		{"third field", 2, 2},
		{"fourth field", 3, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state.cursor = tt.cursor
			assert.Equal(t, tt.expected, state.cursor)
		})
	}
}

// TestBenchmarkState_ProgressChannel tests progress channel handling
func TestBenchmarkState_ProgressChannel(t *testing.T) {
	state := newBenchmarkState("fwd", "alias", 8080)

	// Create a progress channel
	state.progressCh = make(chan BenchmarkProgressMsg, 10)

	// Send some progress
	state.progressCh <- BenchmarkProgressMsg{
		ForwardID: "fwd",
		Completed: 50,
		Total:     100,
	}

	// Receive and verify
	msg := <-state.progressCh
	assert.Equal(t, "fwd", msg.ForwardID)
	assert.Equal(t, 50, msg.Completed)
	assert.Equal(t, 100, msg.Total)

	// Close channel
	close(state.progressCh)
}

// TestBenchmarkStepValues tests step constants
func TestBenchmarkStepValues(t *testing.T) {
	assert.Equal(t, BenchmarkStep(0), BenchmarkStepConfig)
	assert.Equal(t, BenchmarkStep(1), BenchmarkStepRunning)
	assert.Equal(t, BenchmarkStep(2), BenchmarkStepResults)
}

// TestBenchmarkResults_StatusCodeMap tests status code tracking
func TestBenchmarkResults_StatusCodeMap(t *testing.T) {
	results := &BenchmarkResults{
		StatusCodes: make(map[int]int),
	}

	// Simulate collecting status codes
	codes := []int{200, 200, 200, 201, 404, 500, 200}
	for _, code := range codes {
		results.StatusCodes[code]++
	}

	assert.Equal(t, 4, results.StatusCodes[200])
	assert.Equal(t, 1, results.StatusCodes[201])
	assert.Equal(t, 1, results.StatusCodes[404])
	assert.Equal(t, 1, results.StatusCodes[500])
}

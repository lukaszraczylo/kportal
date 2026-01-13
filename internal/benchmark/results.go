// Package benchmark provides HTTP benchmarking capabilities for port forwards.
// It measures latency, throughput, and reliability of forwarded connections.
//
// The benchmark runner sends configurable numbers of concurrent requests
// and collects statistics including:
//   - Latency percentiles (P50, P95, P99)
//   - Request success/failure rates
//   - Throughput (requests/second)
//   - Status code distribution
//
// Results can be displayed in the UI or exported for analysis.
package benchmark

import (
	"sort"
	"time"
)

// Results holds the aggregated results of a benchmark run
type Results struct {
	StartTime     time.Time       `json:"start_time"`
	EndTime       time.Time       `json:"end_time"`
	StatusCodes   map[int]int     `json:"status_codes"`
	Errors        map[string]int  `json:"errors,omitempty"`
	Method        string          `json:"method"`
	URL           string          `json:"url"`
	ForwardID     string          `json:"forward_id"`
	Latencies     []time.Duration `json:"-"`
	TotalRequests int             `json:"total_requests"`
	Successful    int             `json:"successful"`
	Failed        int             `json:"failed"`
	BytesRead     int64           `json:"bytes_read"`
	BytesWritten  int64           `json:"bytes_written"`
}

// Stats holds calculated statistics
type Stats struct {
	MinLatency time.Duration `json:"min_latency_ms"`
	MaxLatency time.Duration `json:"max_latency_ms"`
	AvgLatency time.Duration `json:"avg_latency_ms"`
	P50Latency time.Duration `json:"p50_latency_ms"`
	P95Latency time.Duration `json:"p95_latency_ms"`
	P99Latency time.Duration `json:"p99_latency_ms"`
	Throughput float64       `json:"throughput_rps"`
	Duration   time.Duration `json:"duration"`
}

// NewResults creates a new Results instance
func NewResults(forwardID, url, method string) *Results {
	return &Results{
		ForwardID:   forwardID,
		URL:         url,
		Method:      method,
		StartTime:   time.Now(),
		Latencies:   make([]time.Duration, 0),
		StatusCodes: make(map[int]int),
		Errors:      make(map[string]int),
	}
}

// RecordSuccess records a successful HTTP request (transport succeeded)
// Note: only 2xx status codes are counted as successful for statistics
func (r *Results) RecordSuccess(statusCode int, latency time.Duration, bytesRead, bytesWritten int64) {
	r.TotalRequests++
	// Only count 2xx as successful
	if statusCode >= 200 && statusCode < 300 {
		r.Successful++
	} else {
		r.Failed++
	}
	r.Latencies = append(r.Latencies, latency)
	r.StatusCodes[statusCode]++
	r.BytesRead += bytesRead
	r.BytesWritten += bytesWritten
}

// RecordFailure records a failed request
func (r *Results) RecordFailure(err error, latency time.Duration) {
	r.TotalRequests++
	r.Failed++
	r.Latencies = append(r.Latencies, latency)
	r.Errors[err.Error()]++
}

// Finalize marks the benchmark as complete
func (r *Results) Finalize() {
	r.EndTime = time.Now()
}

// CalculateStats calculates statistics from the results
func (r *Results) CalculateStats() Stats {
	stats := Stats{
		Duration: r.EndTime.Sub(r.StartTime),
	}

	if len(r.Latencies) == 0 {
		return stats
	}

	// Sort latencies for percentile calculation
	sorted := make([]time.Duration, len(r.Latencies))
	copy(sorted, r.Latencies)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	// Calculate min, max, avg
	var total time.Duration
	stats.MinLatency = sorted[0]
	stats.MaxLatency = sorted[len(sorted)-1]

	for _, lat := range sorted {
		total += lat
	}
	stats.AvgLatency = total / time.Duration(len(sorted))

	// Calculate percentiles
	stats.P50Latency = percentile(sorted, 50)
	stats.P95Latency = percentile(sorted, 95)
	stats.P99Latency = percentile(sorted, 99)

	// Calculate throughput
	if stats.Duration > 0 {
		stats.Throughput = float64(r.TotalRequests) / stats.Duration.Seconds()
	}

	return stats
}

// percentile calculates the p-th percentile of sorted durations
func percentile(sorted []time.Duration, p int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}

	idx := (p * len(sorted)) / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

package httplog

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// BenchmarkLoggerLog benchmarks the Log function with sync.Pool
func BenchmarkLoggerLog(b *testing.B) {
	l := &Logger{
		forwardID:  "benchmark",
		maxBodyLen: 1024,
		output:     io.Discard,
	}

	entry := Entry{
		Direction:  "request",
		RequestID:  "req-123",
		Method:     "POST",
		Path:       "/api/users",
		BodySize:   256,
		Body:       `{"name":"test user","email":"test@example.com","data":"some payload data here"}`,
		StatusCode: 200,
		LatencyMs:  42,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = l.Log(entry)
	}
}

// BenchmarkLoggerLogNoPool simulates logging without sync.Pool
func BenchmarkLoggerLogNoPool(b *testing.B) {
	l := &Logger{
		forwardID:  "benchmark",
		maxBodyLen: 1024,
		output:     io.Discard,
	}

	entry := Entry{
		Direction:  "request",
		RequestID:  "req-123",
		Method:     "POST",
		Path:       "/api/users",
		BodySize:   256,
		Body:       `{"name":"test user","email":"test@example.com","data":"some payload data here"}`,
		StatusCode: 200,
		LatencyMs:  42,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate old behavior: allocate new buffer each time
		data, _ := json.Marshal(entry)
		_, _ = l.output.Write(append(data, '\n'))
	}
}

// BenchmarkReadBodyLimited benchmarks reading body with sync.Pool
func BenchmarkReadBodyLimited(b *testing.B) {
	bodyData := bytes.Repeat([]byte("a"), 1024)
	transport := &loggingTransport{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Create a new ReadCloser for each iteration
		body := io.NopCloser(bytes.NewReader(bodyData))
		_, _ = transport.readBodyLimited(body, 2048)
	}
}

// BenchmarkReadBodyLimitedSmall benchmarks with small bodies (typical API requests)
func BenchmarkReadBodyLimitedSmall(b *testing.B) {
	bodyData := []byte(`{"id":123,"name":"test","active":true}`)
	transport := &loggingTransport{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		body := io.NopCloser(bytes.NewReader(bodyData))
		_, _ = transport.readBodyLimited(body, 1024)
	}
}

// BenchmarkReadBodyLimitedLarge benchmarks with large bodies
func BenchmarkReadBodyLimitedLarge(b *testing.B) {
	bodyData := bytes.Repeat([]byte("x"), 65536) // 64KB
	transport := &loggingTransport{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		body := io.NopCloser(bytes.NewReader(bodyData))
		_, _ = transport.readBodyLimited(body, 65536)
	}
}

// BenchmarkBufferPoolGetPut benchmarks the buffer pool itself
func BenchmarkBufferPoolGetPut(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			bufPtr := bufferPool.Get().(*[]byte)
			// Reset and use the buffer to simulate real usage
			*bufPtr = (*bufPtr)[:0]
			*bufPtr = append(*bufPtr, "test data..."...)
			bufferPool.Put(bufPtr)
		}
	})
}

// BenchmarkLogBufferPoolGetPut benchmarks the log buffer pool
func BenchmarkLogBufferPoolGetPut(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buf := logBufferPool.Get().(*bytes.Buffer)
			buf.Reset()
			buf.WriteString("test log entry")
			logBufferPool.Put(buf)
		}
	})
}

// BenchmarkFlattenHeaders benchmarks header flattening with pooling
func BenchmarkFlattenHeaders(b *testing.B) {
	headers := http.Header{
		"Content-Type": []string{"application/json"},
		"Accept":       []string{"text/html", "application/json"},
		"User-Agent":   []string{"test-client/1.0"},
		"X-Request-ID": []string{"abc-123-def"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = flattenHeaders(headers)
	}
}

// BenchmarkTruncateBody benchmarks body truncation with pooled buffers
func BenchmarkTruncateBody(b *testing.B) {
	body := "this is a very long body that should be truncated for logging purposes"
	maxLen := 20

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = truncateBody(body, maxLen)
	}
}

// BenchmarkTruncateBodyNoPool simulates truncation without pooling
func BenchmarkTruncateBodyNoPool(b *testing.B) {
	body := "this is a very long body that should be truncated for logging purposes"
	maxLen := 20

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if len(body) > maxLen {
			_ = body[:maxLen] + "...(truncated)"
		}
	}
}

// BenchmarkLoggerLogWithTruncation benchmarks logging with body truncation
func BenchmarkLoggerLogWithTruncation(b *testing.B) {
	l := &Logger{
		forwardID:  "benchmark",
		maxBodyLen: 50,
		output:     io.Discard,
	}

	entry := Entry{
		Direction: "request",
		RequestID: "req-123",
		Method:    "POST",
		Path:      "/api/users",
		Body:      `{"name":"test user","email":"test@example.com","data":"some payload data here for truncation"}`,
		BodySize:  100,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = l.Log(entry)
	}
}

// BenchmarkReadBufferPool benchmarks the read buffer pool
func BenchmarkReadBufferPool(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			bufPtr := readBufferPool.Get().(*[]byte)
			buf := *bufPtr
			_ = len(buf) // Use the buffer
			readBufferPool.Put(bufPtr)
		}
	})
}

// BenchmarkReadBodyLimitedParallel benchmarks body reading under concurrent load
func BenchmarkReadBodyLimitedParallel(b *testing.B) {
	bodyData := bytes.Repeat([]byte("x"), 4096)
	transport := &loggingTransport{}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			body := io.NopCloser(bytes.NewReader(bodyData))
			_, _ = transport.readBodyLimited(body, 8192)
		}
	})
}

// BenchmarkLoggerLogParallel benchmarks logging under concurrent load
func BenchmarkLoggerLogParallel(b *testing.B) {
	l := &Logger{
		forwardID:  "benchmark",
		maxBodyLen: 1024,
		output:     io.Discard,
	}

	entry := Entry{
		Direction: "request",
		RequestID: "req-123",
		Method:    "POST",
		Path:      "/api/users",
		Body:      `{"name":"test user"}`,
		BodySize:  100,
	}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = l.Log(entry)
		}
	})
}

// BenchmarkCompleteFlow benchmarks the complete logging flow
func BenchmarkCompleteFlow(b *testing.B) {
	l := &Logger{
		forwardID:  "benchmark",
		maxBodyLen: 1024,
		output:     io.Discard,
	}

	headers := http.Header{
		"Content-Type": []string{"application/json"},
		"Accept":       []string{"application/json"},
	}

	bodyData := []byte(`{"id":123,"name":"test"}`)
	transport := &loggingTransport{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate full request logging flow
		entry := Entry{
			Direction: "request",
			RequestID: "req-123",
			Method:    "POST",
			Path:      "/api/users",
			Headers:   flattenHeaders(headers),
			BodySize:  len(bodyData),
			Body:      string(bodyData),
		}
		_ = l.Log(entry)

		// Simulate body reading
		body := io.NopCloser(bytes.NewReader(bodyData))
		_, _ = transport.readBodyLimited(body, 2048)
	}
}

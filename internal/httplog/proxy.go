package httplog

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nvm/kportal/internal/config"
	"github.com/nvm/kportal/internal/logger"
)

// Proxy is an HTTP reverse proxy with logging capabilities
type Proxy struct {
	listener     net.Listener
	logger       *Logger
	server       *http.Server
	forwardID    string
	filterPath   string
	localPort    int
	targetPort   int
	requestCount uint64
	mu           sync.Mutex
	includeHdrs  bool
	running      bool
}

// NewProxy creates a new HTTP logging proxy
func NewProxy(fwd *config.Forward, targetPort int) (*Proxy, error) {
	httpCfg := fwd.HTTPLog
	if httpCfg == nil {
		return nil, fmt.Errorf("HTTP log config is nil")
	}

	logger, err := NewLogger(fwd.ID(), httpCfg.LogFile, fwd.GetHTTPLogMaxBodySize())
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}

	return &Proxy{
		localPort:   fwd.LocalPort,
		targetPort:  targetPort,
		logger:      logger,
		forwardID:   fwd.ID(),
		filterPath:  httpCfg.FilterPath,
		includeHdrs: httpCfg.IncludeHeaders,
	}, nil
}

// Start starts the HTTP proxy server
func (p *Proxy) Start() error {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return fmt.Errorf("proxy already running")
	}

	// Create listener
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p.localPort))
	if err != nil {
		p.mu.Unlock()
		return fmt.Errorf("failed to listen on port %d: %w", p.localPort, err)
	}
	p.listener = ln

	// Create reverse proxy
	director := func(req *http.Request) {
		req.URL.Scheme = "http"
		req.URL.Host = fmt.Sprintf("127.0.0.1:%d", p.targetPort)
	}

	proxy := &httputil.ReverseProxy{
		Director: director,
		Transport: &loggingTransport{
			proxy:     p,
			transport: http.DefaultTransport,
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			p.logError(r, err)
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("Proxy error: " + err.Error()))
		},
	}

	p.server = &http.Server{
		Handler:           proxy,
		ReadHeaderTimeout: 10 * time.Second,
	}

	p.running = true
	p.mu.Unlock()

	// Start serving (blocking)
	go func() {
		if err := p.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			logger.Debug("HTTP proxy serve error (will be replaced on reconnect)", map[string]any{"error": err.Error()})
		}
	}()

	return nil
}

// Stop stops the HTTP proxy server
func (p *Proxy) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return nil
	}

	p.running = false

	// Shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := p.server.Shutdown(ctx); err != nil {
		// Force close - error ignored as we're already shutting down
		_ = p.server.Close()
	}

	if err := p.logger.Close(); err != nil {
		return err
	}

	return nil
}

// loggingTransport wraps http.RoundTripper to log requests and responses
type loggingTransport struct {
	proxy     *Proxy
	transport http.RoundTripper
}

func (t *loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Generate request ID
	reqID := fmt.Sprintf("%d", atomic.AddUint64(&t.proxy.requestCount, 1))

	// Check if we should log this request based on path filter
	if !t.proxy.shouldLog(req.URL.Path) {
		return t.transport.RoundTrip(req)
	}

	startTime := time.Now()
	maxBodySize := t.proxy.logger.GetMaxBodyLen()

	// Read request body with size limit to prevent memory exhaustion
	var reqBody []byte
	var reqBodySize int
	if req.Body != nil {
		reqBody, reqBodySize = t.readBodyLimited(req.Body, maxBodySize)
		req.Body = io.NopCloser(bytes.NewBuffer(reqBody))
	}

	// Log request
	reqEntry := Entry{
		RequestID: reqID,
		Direction: "request",
		Method:    req.Method,
		Path:      req.URL.Path,
		BodySize:  reqBodySize,
		Body:      string(reqBody),
	}

	if t.proxy.includeHdrs {
		reqEntry.Headers = flattenHeaders(req.Header)
	}

	_ = t.proxy.logger.Log(reqEntry)

	// Make the request
	resp, err := t.transport.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	// Read response body with size limit to prevent memory exhaustion
	var respBody []byte
	var respBodySize int
	if resp.Body != nil {
		respBody, respBodySize = t.readBodyLimited(resp.Body, maxBodySize)
		resp.Body = io.NopCloser(bytes.NewBuffer(respBody))
	}

	latency := time.Since(startTime)

	// Log response
	respEntry := Entry{
		RequestID:  reqID,
		Direction:  "response",
		Method:     req.Method,
		Path:       req.URL.Path,
		StatusCode: resp.StatusCode,
		BodySize:   respBodySize,
		Body:       string(respBody),
		LatencyMs:  latency.Milliseconds(),
	}

	if t.proxy.includeHdrs {
		respEntry.Headers = flattenHeaders(resp.Header)
	}

	_ = t.proxy.logger.Log(respEntry)

	return resp, nil
}

// readBodyLimited reads a body with a size limit to prevent memory exhaustion.
// Returns the body content (up to maxSize bytes) and the actual content length.
// If the body exceeds maxSize, it reads only maxSize bytes for logging but
// consumes the entire body to get the true size for BodySize reporting.
func (t *loggingTransport) readBodyLimited(body io.ReadCloser, maxSize int) ([]byte, int) {
	// Read up to maxSize+1 to detect if there's more
	limitedReader := io.LimitReader(body, int64(maxSize+1))
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, 0
	}

	actualSize := len(data)
	wasTruncated := actualSize > maxSize

	// If we read exactly maxSize+1, there might be more data
	// Discard the rest but count the bytes for accurate BodySize
	if wasTruncated {
		data = data[:maxSize] // Keep only maxSize bytes for logging
		// Count remaining bytes without storing them
		remaining, _ := io.Copy(io.Discard, body)
		actualSize = maxSize + int(remaining)
	}

	return data, actualSize
}

// shouldLog checks if the request path matches the filter
func (p *Proxy) shouldLog(path string) bool {
	if p.filterPath == "" {
		return true
	}

	matched, err := filepath.Match(p.filterPath, path)
	if err != nil {
		// Invalid pattern, log everything
		return true
	}

	// Also try matching with ** for prefix patterns like /api/*
	if !matched && strings.HasSuffix(p.filterPath, "/*") {
		prefix := strings.TrimSuffix(p.filterPath, "/*")
		matched = strings.HasPrefix(path, prefix)
	}

	return matched
}

// logError logs an error entry
func (p *Proxy) logError(req *http.Request, err error) {
	entry := Entry{
		RequestID: fmt.Sprintf("%d", atomic.AddUint64(&p.requestCount, 1)),
		Direction: "error",
		Method:    req.Method,
		Path:      req.URL.Path,
		Error:     err.Error(),
	}
	_ = p.logger.Log(entry)
}

// flattenHeaders converts http.Header to map[string]string
func flattenHeaders(h http.Header) map[string]string {
	result := make(map[string]string, len(h))
	for k, v := range h {
		result[k] = strings.Join(v, ", ")
	}
	return result
}

// GetTargetPort returns the target port for the k8s tunnel
func (p *Proxy) GetTargetPort() int {
	return p.targetPort
}

// GetLogger returns the HTTP logger for subscribing to log entries
func (p *Proxy) GetLogger() *Logger {
	return p.logger
}

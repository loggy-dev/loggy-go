package loggy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// MetricsConfig configures the metrics tracker
type MetricsConfig struct {
	Token         string
	Endpoint      string
	FlushInterval time.Duration
	Disabled      bool
}

// RequestEndOptions contains options for ending a request measurement
type RequestEndOptions struct {
	StatusCode int
	BytesIn    int64
	BytesOut   int64
	Path       string
	Method     string
}

// MetricBucket holds aggregated metrics for a time period
type MetricBucket struct {
	Timestamp       time.Time
	Path            string
	Method          string
	RequestCount    int
	TotalDurationMs int64
	MinDurationMs   *int64
	MaxDurationMs   *int64
	TotalBytesIn    int64
	TotalBytesOut   int64
	Status2xx       int
	Status3xx       int
	Status4xx       int
	Status5xx       int
}

// Metrics is the performance metrics tracker
type Metrics struct {
	config      MetricsConfig
	buckets     map[string]*MetricBucket
	bucketMutex sync.Mutex
	flushTicker *time.Ticker
	stopChan    chan struct{}
	wg          sync.WaitGroup
}

// NewMetrics creates a new metrics tracker
func NewMetrics(config MetricsConfig) *Metrics {
	if config.Endpoint == "" {
		config.Endpoint = "https://loggy.dev/api/metrics/ingest"
	}
	if config.FlushInterval == 0 {
		config.FlushInterval = 60 * time.Second
	}

	m := &Metrics{
		config:   config,
		buckets:  make(map[string]*MetricBucket),
		stopChan: make(chan struct{}),
	}

	if !config.Disabled && config.Token != "" {
		m.flushTicker = time.NewTicker(config.FlushInterval)
		m.wg.Add(1)
		go m.flushLoop()
	}

	return m
}

func (m *Metrics) flushLoop() {
	defer m.wg.Done()
	for {
		select {
		case <-m.flushTicker.C:
			m.Flush()
		case <-m.stopChan:
			return
		}
	}
}

func roundToMinute(t time.Time) time.Time {
	return t.Truncate(time.Minute)
}

func getBucketKey(t time.Time, path, method string) string {
	return fmt.Sprintf("%s|%s|%s", roundToMinute(t).Format(time.RFC3339), path, method)
}

func getStatusCategory(statusCode int) string {
	switch {
	case statusCode >= 200 && statusCode < 300:
		return "2xx"
	case statusCode >= 300 && statusCode < 400:
		return "3xx"
	case statusCode >= 400 && statusCode < 500:
		return "4xx"
	case statusCode >= 500 && statusCode < 600:
		return "5xx"
	default:
		return ""
	}
}

func (m *Metrics) getOrCreateBucket(t time.Time, path, method string) *MetricBucket {
	key := getBucketKey(t, path, method)
	bucket, exists := m.buckets[key]
	if !exists {
		bucket = &MetricBucket{
			Timestamp: roundToMinute(t),
			Path:      path,
			Method:    method,
		}
		m.buckets[key] = bucket
	}
	return bucket
}

func (m *Metrics) recordRequest(startTime time.Time, durationMs int64, opts RequestEndOptions) {
	m.bucketMutex.Lock()
	defer m.bucketMutex.Unlock()

	bucket := m.getOrCreateBucket(startTime, opts.Path, opts.Method)

	bucket.RequestCount++
	bucket.TotalDurationMs += durationMs

	if bucket.MinDurationMs == nil || durationMs < *bucket.MinDurationMs {
		bucket.MinDurationMs = &durationMs
	}
	if bucket.MaxDurationMs == nil || durationMs > *bucket.MaxDurationMs {
		bucket.MaxDurationMs = &durationMs
	}

	bucket.TotalBytesIn += opts.BytesIn
	bucket.TotalBytesOut += opts.BytesOut

	switch getStatusCategory(opts.StatusCode) {
	case "2xx":
		bucket.Status2xx++
	case "3xx":
		bucket.Status3xx++
	case "4xx":
		bucket.Status4xx++
	case "5xx":
		bucket.Status5xx++
	}
}

// RequestTimer is returned by StartRequest and used to end the measurement
type RequestTimer struct {
	metrics   *Metrics
	startTime time.Time
	startNano int64
}

// End ends the request measurement and records the metrics
func (rt *RequestTimer) End(opts RequestEndOptions) {
	durationMs := (time.Now().UnixNano() - rt.startNano) / int64(time.Millisecond)
	rt.metrics.recordRequest(rt.startTime, durationMs, opts)
}

// StartRequest starts tracking a request and returns a timer
func (m *Metrics) StartRequest() *RequestTimer {
	return &RequestTimer{
		metrics:   m,
		startTime: time.Now(),
		startNano: time.Now().UnixNano(),
	}
}

// Record records a pre-measured request
func (m *Metrics) Record(durationMs int64, opts RequestEndOptions) {
	m.recordRequest(time.Now(), durationMs, opts)
}

// Flush sends all collected metrics to the server
func (m *Metrics) Flush() error {
	if m.config.Disabled || m.config.Token == "" {
		return nil
	}

	m.bucketMutex.Lock()
	if len(m.buckets) == 0 {
		m.bucketMutex.Unlock()
		return nil
	}

	currentTimeKey := roundToMinute(time.Now()).Format(time.RFC3339)
	bucketsToSend := make([]*MetricBucket, 0)
	keysToDelete := make([]string, 0)

	for key, bucket := range m.buckets {
		// Don't send current minute's bucket (still collecting)
		if key[:len(currentTimeKey)] != currentTimeKey {
			bucketsToSend = append(bucketsToSend, bucket)
			keysToDelete = append(keysToDelete, key)
		}
	}

	for _, key := range keysToDelete {
		delete(m.buckets, key)
	}
	m.bucketMutex.Unlock()

	if len(bucketsToSend) == 0 {
		return nil
	}

	// Transform to API format
	metrics := make([]map[string]interface{}, len(bucketsToSend))
	for i, b := range bucketsToSend {
		m := map[string]interface{}{
			"timestamp":       b.Timestamp.Format(time.RFC3339),
			"requestCount":    b.RequestCount,
			"totalDurationMs": b.TotalDurationMs,
			"totalBytesIn":    b.TotalBytesIn,
			"totalBytesOut":   b.TotalBytesOut,
			"status2xx":       b.Status2xx,
			"status3xx":       b.Status3xx,
			"status4xx":       b.Status4xx,
			"status5xx":       b.Status5xx,
		}
		if b.Path != "" {
			m["path"] = b.Path
		}
		if b.Method != "" {
			m["method"] = b.Method
		}
		if b.MinDurationMs != nil {
			m["minDurationMs"] = *b.MinDurationMs
		}
		if b.MaxDurationMs != nil {
			m["maxDurationMs"] = *b.MaxDurationMs
		}
		metrics[i] = m
	}

	payload := map[string]interface{}{
		"metrics": metrics,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", m.config.Endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-loggy-token", m.config.Token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("metrics flush failed with status %d", resp.StatusCode)
	}

	return nil
}

// GetPendingCount returns the number of pending metrics
func (m *Metrics) GetPendingCount() int {
	m.bucketMutex.Lock()
	defer m.bucketMutex.Unlock()

	count := 0
	for _, bucket := range m.buckets {
		count += bucket.RequestCount
	}
	return count
}

// Destroy stops the metrics tracker and flushes remaining data
func (m *Metrics) Destroy() error {
	if m.flushTicker != nil {
		m.flushTicker.Stop()
		close(m.stopChan)
		m.wg.Wait()
	}
	return m.Flush()
}

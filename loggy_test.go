package loggy

import (
	"fmt"
	"testing"
	"time"
)

func TestNewLogger(t *testing.T) {
	logger := New(Config{
		Identifier: "test-app",
		Color:      true,
		Compact:    false,
		Timestamp:  true,
	})

	if logger == nil {
		t.Fatal("Expected logger to be created")
	}

	// Test basic logging (should not panic)
	logger.Log("Test log message")
	logger.Info("Test info message")
	logger.Warn("Test warn message")
	logger.Error("Test error message")

	// Test with metadata
	logger.Info("Test with metadata", map[string]interface{}{
		"key": "value",
		"num": 123,
	})

	// Test with tags
	logger.InfoWithTags("Test with tags", []string{"tag1", "tag2"}, map[string]interface{}{
		"key": "value",
	})

	logger.Blank(1)
}

func TestNewMetrics(t *testing.T) {
	metrics := NewMetrics(MetricsConfig{
		Token:         "test-token",
		FlushInterval: 1 * time.Second,
		Disabled:      true, // Disable for testing
	})

	if metrics == nil {
		t.Fatal("Expected metrics to be created")
	}

	// Test request tracking
	timer := metrics.StartRequest()
	time.Sleep(10 * time.Millisecond)
	timer.End(RequestEndOptions{
		StatusCode: 200,
		BytesIn:    100,
		BytesOut:   200,
		Path:       "/api/test",
		Method:     "GET",
	})

	// Test record
	metrics.Record(50, RequestEndOptions{
		StatusCode: 201,
		Path:       "/api/create",
		Method:     "POST",
	})

	count := metrics.GetPendingCount()
	if count != 2 {
		t.Errorf("Expected 2 pending requests, got %d", count)
	}

	metrics.Destroy()
}

func TestNewTracer(t *testing.T) {
	tracer := NewTracer(TracerConfig{
		ServiceName:    "test-service",
		ServiceVersion: "1.0.0",
		Environment:    "test",
	})

	if tracer == nil {
		t.Fatal("Expected tracer to be created")
	}

	// Test span creation
	span := tracer.StartSpan("test-operation", &SpanOptions{
		Kind: SpanKindInternal,
		Attributes: SpanAttributes{
			"test.key": "test-value",
		},
	})

	if span == nil {
		t.Fatal("Expected span to be created")
	}

	if span.OperationName() != "test-operation" {
		t.Errorf("Expected operation name 'test-operation', got '%s'", span.OperationName())
	}

	if span.ServiceName() != "test-service" {
		t.Errorf("Expected service name 'test-service', got '%s'", span.ServiceName())
	}

	span.SetAttribute("another.key", "another-value")
	span.AddEvent("test-event", SpanAttributes{"event.key": "event-value"})
	span.SetStatus(SpanStatusOK, "")
	span.End()

	if span.IsRecording() {
		t.Error("Expected span to not be recording after End()")
	}

	tracer.Destroy()
}

func TestGenerateIDs(t *testing.T) {
	traceID := GenerateTraceID()
	if len(traceID) != 32 {
		t.Errorf("Expected trace ID length 32, got %d", len(traceID))
	}

	spanID := GenerateSpanID()
	if len(spanID) != 16 {
		t.Errorf("Expected span ID length 16, got %d", len(spanID))
	}
}

func TestTraceparent(t *testing.T) {
	// Test parsing
	header := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	ctx := ParseTraceparent(header)

	if ctx == nil {
		t.Fatal("Expected context to be parsed")
	}

	if ctx.TraceID != "0af7651916cd43dd8448eb211c80319c" {
		t.Errorf("Expected trace ID '0af7651916cd43dd8448eb211c80319c', got '%s'", ctx.TraceID)
	}

	if ctx.SpanID != "b7ad6b7169203331" {
		t.Errorf("Expected span ID 'b7ad6b7169203331', got '%s'", ctx.SpanID)
	}

	if ctx.TraceFlags != 1 {
		t.Errorf("Expected trace flags 1, got %d", ctx.TraceFlags)
	}

	// Test formatting
	formatted := FormatTraceparent(*ctx)
	if formatted != header {
		t.Errorf("Expected '%s', got '%s'", header, formatted)
	}

	// Test invalid headers
	invalidHeaders := []string{
		"",
		"invalid",
		"01-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01", // wrong version
		"00-00000000000000000000000000000000-b7ad6b7169203331-01", // all zeros trace ID
		"00-0af7651916cd43dd8448eb211c80319c-0000000000000000-01", // all zeros span ID
	}

	for _, h := range invalidHeaders {
		if ParseTraceparent(h) != nil {
			t.Errorf("Expected nil for invalid header '%s'", h)
		}
	}
}

func TestContextPropagation(t *testing.T) {
	ctx := SpanContext{
		TraceID:    "0af7651916cd43dd8448eb211c80319c",
		SpanID:     "b7ad6b7169203331",
		TraceFlags: 1,
	}

	carrier := make(map[string]string)
	InjectContext(ctx, carrier)

	if carrier["traceparent"] == "" {
		t.Error("Expected traceparent header to be set")
	}

	extracted := ExtractContext(carrier)
	if extracted == nil {
		t.Fatal("Expected context to be extracted")
	}

	if extracted.TraceID != ctx.TraceID {
		t.Errorf("Expected trace ID '%s', got '%s'", ctx.TraceID, extracted.TraceID)
	}

	if extracted.SpanID != ctx.SpanID {
		t.Errorf("Expected span ID '%s', got '%s'", ctx.SpanID, extracted.SpanID)
	}
}

func TestLogLevels(t *testing.T) {
	logger := New(Config{
		Identifier: "level-test",
		Color:      false,
		Timestamp:  false,
	})

	// Test all log levels with metadata
	testCases := []struct {
		name     string
		logFunc  func(string, ...map[string]interface{})
		message  string
		metadata map[string]interface{}
	}{
		{
			name:    "Log/Debug",
			logFunc: logger.Log,
			message: "Debug level message",
			metadata: map[string]interface{}{
				"debug_key": "debug_value",
				"count":     1,
			},
		},
		{
			name:    "Info",
			logFunc: logger.Info,
			message: "Info level message",
			metadata: map[string]interface{}{
				"user_id": 12345,
				"action":  "login",
			},
		},
		{
			name:    "Warn",
			logFunc: logger.Warn,
			message: "Warning level message",
			metadata: map[string]interface{}{
				"threshold": 0.85,
				"metric":    "cpu_usage",
			},
		},
		{
			name:    "Error",
			logFunc: logger.Error,
			message: "Error level message",
			metadata: map[string]interface{}{
				"error_code": "E001",
				"stack":      "main.go:42",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Should not panic
			tc.logFunc(tc.message, tc.metadata)
		})
	}
}

func TestLogWithTags(t *testing.T) {
	logger := New(Config{
		Identifier: "tags-test",
		Color:      false,
		Timestamp:  false,
	})

	tags := []string{"api", "v2", "production"}
	metadata := map[string]interface{}{
		"endpoint": "/api/v2/users",
		"method":   "GET",
	}

	// Test all tag variants
	logger.LogWithTags("Debug with tags", tags, metadata)
	logger.InfoWithTags("Info with tags", tags, metadata)
	logger.WarnWithTags("Warn with tags", tags, metadata)
	logger.ErrorWithTags("Error with tags", tags, metadata)
}

func TestTracerWithMultipleSpans(t *testing.T) {
	tracer := NewTracer(TracerConfig{
		ServiceName:    "test-service",
		ServiceVersion: "1.0.0",
		Environment:    "test",
	})
	defer tracer.Destroy()

	// Create a parent span (simulating incoming HTTP request)
	parentSpan := tracer.StartSpan("HTTP GET /api/users", &SpanOptions{
		Kind: SpanKindServer,
		Attributes: SpanAttributes{
			"http.method": "GET",
			"http.url":    "/api/users",
		},
	})

	// Create a child span (simulating database query)
	parentCtx := parentSpan.Context()
	dbSpan := tracer.StartSpan("db.query", &SpanOptions{
		Kind:   SpanKindClient,
		Parent: &parentCtx,
		Attributes: SpanAttributes{
			"db.system":    "postgresql",
			"db.statement": "SELECT * FROM users",
		},
	})

	// Verify parent-child relationship
	if dbSpan.parentSpanID == nil {
		t.Error("Expected db span to have parent span ID")
	}
	if *dbSpan.parentSpanID != parentSpan.Context().SpanID {
		t.Errorf("Expected parent span ID '%s', got '%s'", parentSpan.Context().SpanID, *dbSpan.parentSpanID)
	}

	// Verify same trace ID
	if dbSpan.Context().TraceID != parentSpan.Context().TraceID {
		t.Errorf("Expected same trace ID, got parent='%s' child='%s'",
			parentSpan.Context().TraceID, dbSpan.Context().TraceID)
	}

	dbSpan.SetStatus(SpanStatusOK, "")
	dbSpan.End()

	// Create another child span (simulating cache lookup)
	cacheSpan := tracer.StartSpan("cache.get", &SpanOptions{
		Kind:   SpanKindClient,
		Parent: &parentCtx,
		Attributes: SpanAttributes{
			"cache.type": "redis",
			"cache.key":  "users:list",
		},
	})

	cacheSpan.AddEvent("cache_miss", SpanAttributes{
		"reason": "key_not_found",
	})
	cacheSpan.SetStatus(SpanStatusOK, "")
	cacheSpan.End()

	parentSpan.SetAttribute("http.status_code", 200)
	parentSpan.SetStatus(SpanStatusOK, "")
	parentSpan.End()

	// Verify all spans have ended
	if parentSpan.IsRecording() || dbSpan.IsRecording() || cacheSpan.IsRecording() {
		t.Error("Expected all spans to have ended")
	}
}

func TestTraceLogCorrelation(t *testing.T) {
	// Create tracer
	tracer := NewTracer(TracerConfig{
		ServiceName: "correlation-test",
	})
	defer tracer.Destroy()

	// Create logger
	logger := New(Config{
		Identifier: "correlation-test",
		Color:      false,
		Timestamp:  false,
	})

	// Start a span
	span := tracer.StartSpan("process-request", &SpanOptions{
		Kind: SpanKindServer,
	})

	// Get trace context for log correlation
	traceID, spanID := tracer.GetCurrentContext()

	// Verify we have context
	if traceID == "" {
		t.Error("Expected trace ID to be available")
	}
	if spanID == "" {
		t.Error("Expected span ID to be available")
	}

	// Log with trace correlation
	logger.Info("Processing request", map[string]interface{}{
		"traceId": traceID,
		"spanId":  spanID,
		"userId":  12345,
	})

	// Verify the trace context matches the span
	if traceID != span.Context().TraceID {
		t.Errorf("Expected trace ID '%s', got '%s'", span.Context().TraceID, traceID)
	}
	if spanID != span.Context().SpanID {
		t.Errorf("Expected span ID '%s', got '%s'", span.Context().SpanID, spanID)
	}

	span.End()
}

func TestSpanEvents(t *testing.T) {
	tracer := NewTracer(TracerConfig{
		ServiceName: "events-test",
	})
	defer tracer.Destroy()

	span := tracer.StartSpan("operation-with-events", nil)

	// Add multiple events
	span.AddEvent("started", SpanAttributes{
		"component": "processor",
	})

	span.AddEvent("checkpoint", SpanAttributes{
		"items_processed": 50,
		"progress":        0.5,
	})

	span.AddEvent("exception", SpanAttributes{
		"exception.type":    "ValidationError",
		"exception.message": "Invalid input format",
	})

	span.AddEvent("completed", SpanAttributes{
		"items_processed": 100,
		"duration_ms":     1500,
	})

	span.SetStatus(SpanStatusError, "Validation failed")
	span.End()

	// Verify span data
	data := span.ToData()
	if len(data.Events) != 4 {
		t.Errorf("Expected 4 events, got %d", len(data.Events))
	}

	if data.Status != SpanStatusError {
		t.Errorf("Expected status 'error', got '%s'", data.Status)
	}

	if data.StatusMessage != "Validation failed" {
		t.Errorf("Expected status message 'Validation failed', got '%s'", data.StatusMessage)
	}
}

func TestWithSpanHelper(t *testing.T) {
	tracer := NewTracer(TracerConfig{
		ServiceName: "withspan-test",
	})
	defer tracer.Destroy()

	// Test successful operation
	err := WithSpan(tracer, "successful-operation", func() error {
		// Simulate work
		return nil
	}, SpanAttributes{"test.key": "test-value"})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test failed operation
	expectedErr := fmt.Errorf("operation failed")
	err = WithSpan(tracer, "failed-operation", func() error {
		return expectedErr
	}, nil)

	if err != expectedErr {
		t.Errorf("Expected error '%v', got '%v'", expectedErr, err)
	}
}

func TestMetricsStatusCodeCategories(t *testing.T) {
	metrics := NewMetrics(MetricsConfig{
		Token:    "test-token",
		Disabled: true,
	})
	defer metrics.Destroy()

	testCases := []struct {
		statusCode int
		expected   string
	}{
		{200, "2xx"},
		{201, "2xx"},
		{204, "2xx"},
		{301, "3xx"},
		{302, "3xx"},
		{400, "4xx"},
		{401, "4xx"},
		{404, "4xx"},
		{500, "5xx"},
		{502, "5xx"},
		{503, "5xx"},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Status%d", tc.statusCode), func(t *testing.T) {
			timer := metrics.StartRequest()
			timer.End(RequestEndOptions{
				StatusCode: tc.statusCode,
				Path:       "/test",
				Method:     "GET",
			})
		})
	}
}

func TestMetricsWithPathAndMethod(t *testing.T) {
	metrics := NewMetrics(MetricsConfig{
		Token:    "test-token",
		Disabled: true,
	})
	defer metrics.Destroy()

	endpoints := []struct {
		path   string
		method string
	}{
		{"/api/users", "GET"},
		{"/api/users", "POST"},
		{"/api/users/123", "GET"},
		{"/api/users/123", "PUT"},
		{"/api/users/123", "DELETE"},
		{"/api/orders", "GET"},
		{"/api/orders", "POST"},
		{"/health", "GET"},
	}

	for _, ep := range endpoints {
		timer := metrics.StartRequest()
		timer.End(RequestEndOptions{
			StatusCode: 200,
			BytesIn:    100,
			BytesOut:   500,
			Path:       ep.path,
			Method:     ep.method,
		})
	}

	count := metrics.GetPendingCount()
	if count != len(endpoints) {
		t.Errorf("Expected %d pending requests, got %d", len(endpoints), count)
	}
}

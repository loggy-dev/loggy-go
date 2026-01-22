package loggy

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SpanKind represents the type of span
type SpanKind string

const (
	SpanKindClient   SpanKind = "client"
	SpanKindServer   SpanKind = "server"
	SpanKindProducer SpanKind = "producer"
	SpanKindConsumer SpanKind = "consumer"
	SpanKindInternal SpanKind = "internal"
)

// SpanStatus represents the status of a span
type SpanStatus string

const (
	SpanStatusOK    SpanStatus = "ok"
	SpanStatusError SpanStatus = "error"
	SpanStatusUnset SpanStatus = "unset"
)

// SpanContext contains trace context information
type SpanContext struct {
	TraceID    string
	SpanID     string
	TraceFlags int
	TraceState string
}

// SpanAttributes is a map of span attributes
type SpanAttributes map[string]interface{}

// SpanEvent represents an event within a span
type SpanEvent struct {
	Name       string         `json:"name"`
	Timestamp  string         `json:"timestamp"`
	Attributes SpanAttributes `json:"attributes,omitempty"`
}

// SpanData represents span data for sending to the server
type SpanData struct {
	TraceID            string         `json:"traceId"`
	SpanID             string         `json:"spanId"`
	ParentSpanID       *string        `json:"parentSpanId,omitempty"`
	OperationName      string         `json:"operationName"`
	ServiceName        string         `json:"serviceName"`
	SpanKind           SpanKind       `json:"spanKind"`
	StartTime          string         `json:"startTime"`
	EndTime            string         `json:"endTime,omitempty"`
	Status             SpanStatus     `json:"status"`
	StatusMessage      string         `json:"statusMessage,omitempty"`
	Attributes         SpanAttributes `json:"attributes,omitempty"`
	Events             []SpanEvent    `json:"events,omitempty"`
	ResourceAttributes SpanAttributes `json:"resourceAttributes,omitempty"`
}

// SpanOptions contains options for creating a span
type SpanOptions struct {
	Kind       SpanKind
	Attributes SpanAttributes
	Parent     *SpanContext
	StartTime  *time.Time
}

// Span represents a single span in a trace
type Span struct {
	context            SpanContext
	operationName      string
	serviceName        string
	spanKind           SpanKind
	startTime          time.Time
	endTime            *time.Time
	status             SpanStatus
	statusMessage      string
	attributes         SpanAttributes
	events             []SpanEvent
	resourceAttributes SpanAttributes
	parentSpanID       *string
	recording          bool
	onEnd              func(*Span)
	mutex              sync.Mutex
}

// Context returns the span context
func (s *Span) Context() SpanContext {
	return s.context
}

// OperationName returns the operation name
func (s *Span) OperationName() string {
	return s.operationName
}

// ServiceName returns the service name
func (s *Span) ServiceName() string {
	return s.serviceName
}

// StartTime returns the start time
func (s *Span) StartTime() time.Time {
	return s.startTime
}

// SetStatus sets the span status
func (s *Span) SetStatus(status SpanStatus, message string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if !s.recording {
		return
	}
	s.status = status
	s.statusMessage = message
}

// SetAttribute sets a single attribute
func (s *Span) SetAttribute(key string, value interface{}) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if !s.recording {
		return
	}
	if s.attributes == nil {
		s.attributes = make(SpanAttributes)
	}
	s.attributes[key] = value
}

// SetAttributes sets multiple attributes
func (s *Span) SetAttributes(attrs SpanAttributes) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if !s.recording {
		return
	}
	if s.attributes == nil {
		s.attributes = make(SpanAttributes)
	}
	for k, v := range attrs {
		s.attributes[k] = v
	}
}

// AddEvent adds an event to the span
func (s *Span) AddEvent(name string, attrs SpanAttributes) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if !s.recording {
		return
	}
	s.events = append(s.events, SpanEvent{
		Name:       name,
		Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
		Attributes: attrs,
	})
}

// End ends the span
func (s *Span) End() {
	s.EndAt(time.Now())
}

// EndAt ends the span at a specific time
func (s *Span) EndAt(endTime time.Time) {
	s.mutex.Lock()
	if !s.recording {
		s.mutex.Unlock()
		return
	}
	s.endTime = &endTime
	s.recording = false
	onEnd := s.onEnd
	s.mutex.Unlock()

	if onEnd != nil {
		onEnd(s)
	}
}

// IsRecording returns whether the span is still recording
func (s *Span) IsRecording() bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.recording
}

// ToData converts the span to SpanData for sending
func (s *Span) ToData() SpanData {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	data := SpanData{
		TraceID:            s.context.TraceID,
		SpanID:             s.context.SpanID,
		ParentSpanID:       s.parentSpanID,
		OperationName:      s.operationName,
		ServiceName:        s.serviceName,
		SpanKind:           s.spanKind,
		StartTime:          s.startTime.UTC().Format(time.RFC3339Nano),
		Status:             s.status,
		StatusMessage:      s.statusMessage,
		ResourceAttributes: s.resourceAttributes,
	}

	if s.endTime != nil {
		data.EndTime = s.endTime.UTC().Format(time.RFC3339Nano)
	}

	if len(s.attributes) > 0 {
		data.Attributes = s.attributes
	}

	if len(s.events) > 0 {
		data.Events = s.events
	}

	return data
}

// TracerConfig configures the tracer
type TracerConfig struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	Remote         *TracerRemoteConfig
}

// TracerRemoteConfig configures remote trace sending
type TracerRemoteConfig struct {
	Token         string
	Endpoint      string
	BatchSize     int
	FlushInterval time.Duration
	PublicKey     string
}

// Tracer is the distributed tracing tracer
type Tracer struct {
	serviceName        string
	serviceVersion     string
	environment        string
	remote             *TracerRemoteConfig
	spanBuffer         []SpanData
	bufferMutex        sync.Mutex
	flushTicker        *time.Ticker
	stopChan           chan struct{}
	wg                 sync.WaitGroup
	resourceAttributes SpanAttributes
	activeSpans        map[string]*Span
	activeSpansMutex   sync.RWMutex
}

// NewTracer creates a new tracer
func NewTracer(config TracerConfig) *Tracer {
	if config.Remote != nil {
		if config.Remote.Endpoint == "" {
			config.Remote.Endpoint = "https://loggy.dev/api/traces/ingest"
		}
		if config.Remote.BatchSize == 0 {
			config.Remote.BatchSize = 100
		}
		if config.Remote.FlushInterval == 0 {
			config.Remote.FlushInterval = 5 * time.Second
		}
	}

	resourceAttrs := SpanAttributes{
		"service.name": config.ServiceName,
	}
	if config.ServiceVersion != "" {
		resourceAttrs["service.version"] = config.ServiceVersion
	}
	if config.Environment != "" {
		resourceAttrs["deployment.environment"] = config.Environment
	}

	t := &Tracer{
		serviceName:        config.ServiceName,
		serviceVersion:     config.ServiceVersion,
		environment:        config.Environment,
		remote:             config.Remote,
		spanBuffer:         make([]SpanData, 0),
		stopChan:           make(chan struct{}),
		resourceAttributes: resourceAttrs,
		activeSpans:        make(map[string]*Span),
	}

	if config.Remote != nil && config.Remote.Token != "" {
		t.flushTicker = time.NewTicker(config.Remote.FlushInterval)
		t.wg.Add(1)
		go t.flushLoop()
	}

	return t
}

func (t *Tracer) flushLoop() {
	defer t.wg.Done()
	for {
		select {
		case <-t.flushTicker.C:
			t.Flush()
		case <-t.stopChan:
			return
		}
	}
}

// GenerateTraceID generates a random 128-bit trace ID
func GenerateTraceID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// GenerateSpanID generates a random 64-bit span ID
func GenerateSpanID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// StartSpan starts a new span
func (t *Tracer) StartSpan(operationName string, opts *SpanOptions) *Span {
	if opts == nil {
		opts = &SpanOptions{}
	}

	var traceID string
	var parentSpanID *string

	if opts.Parent != nil {
		traceID = opts.Parent.TraceID
		parentSpanID = &opts.Parent.SpanID
	} else {
		traceID = GenerateTraceID()
	}

	startTime := time.Now()
	if opts.StartTime != nil {
		startTime = *opts.StartTime
	}

	kind := opts.Kind
	if kind == "" {
		kind = SpanKindInternal
	}

	span := &Span{
		context: SpanContext{
			TraceID:    traceID,
			SpanID:     GenerateSpanID(),
			TraceFlags: 1, // Sampled
		},
		operationName:      operationName,
		serviceName:        t.serviceName,
		spanKind:           kind,
		startTime:          startTime,
		status:             SpanStatusUnset,
		attributes:         make(SpanAttributes),
		events:             make([]SpanEvent, 0),
		resourceAttributes: t.resourceAttributes,
		parentSpanID:       parentSpanID,
		recording:          true,
		onEnd:              t.onSpanEnd,
	}

	if opts.Attributes != nil {
		for k, v := range opts.Attributes {
			span.attributes[k] = v
		}
	}

	t.activeSpansMutex.Lock()
	t.activeSpans[span.context.SpanID] = span
	t.activeSpansMutex.Unlock()

	return span
}

func (t *Tracer) onSpanEnd(span *Span) {
	t.activeSpansMutex.Lock()
	delete(t.activeSpans, span.context.SpanID)
	t.activeSpansMutex.Unlock()

	if t.remote != nil && t.remote.Token != "" {
		t.bufferMutex.Lock()
		t.spanBuffer = append(t.spanBuffer, span.ToData())
		shouldFlush := len(t.spanBuffer) >= t.remote.BatchSize
		t.bufferMutex.Unlock()

		if shouldFlush {
			go t.Flush()
		}
	}
}

// Inject injects trace context into a carrier (HTTP headers)
func (t *Tracer) Inject(carrier map[string]string) map[string]string {
	t.activeSpansMutex.RLock()
	defer t.activeSpansMutex.RUnlock()

	if len(t.activeSpans) == 0 {
		return carrier
	}

	// Get the most recent span
	var currentSpan *Span
	for _, span := range t.activeSpans {
		currentSpan = span
	}

	if currentSpan == nil {
		return carrier
	}

	return InjectContext(currentSpan.context, carrier)
}

// Extract extracts trace context from a carrier (HTTP headers)
func (t *Tracer) Extract(carrier map[string]string) *SpanContext {
	return ExtractContext(carrier)
}

// GetCurrentSpan returns the current active span
func (t *Tracer) GetCurrentSpan() *Span {
	t.activeSpansMutex.RLock()
	defer t.activeSpansMutex.RUnlock()

	for _, span := range t.activeSpans {
		return span
	}
	return nil
}

// GetCurrentContext returns the current trace context
func (t *Tracer) GetCurrentContext() (traceID, spanID string) {
	span := t.GetCurrentSpan()
	if span == nil {
		return "", ""
	}
	return span.context.TraceID, span.context.SpanID
}

// Flush sends all buffered spans to the server
func (t *Tracer) Flush() error {
	if t.remote == nil || t.remote.Token == "" {
		return nil
	}

	t.bufferMutex.Lock()
	if len(t.spanBuffer) == 0 {
		t.bufferMutex.Unlock()
		return nil
	}

	spansToSend := make([]SpanData, len(t.spanBuffer))
	copy(spansToSend, t.spanBuffer)
	t.spanBuffer = t.spanBuffer[:0]
	t.bufferMutex.Unlock()

	payload := map[string]interface{}{
		"spans": spansToSend,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.bufferMutex.Lock()
		t.spanBuffer = append(spansToSend, t.spanBuffer...)
		t.bufferMutex.Unlock()
		return err
	}

	req, err := http.NewRequest("POST", t.remote.Endpoint, bytes.NewReader(body))
	if err != nil {
		t.bufferMutex.Lock()
		t.spanBuffer = append(spansToSend, t.spanBuffer...)
		t.bufferMutex.Unlock()
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-loggy-token", t.remote.Token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.bufferMutex.Lock()
		t.spanBuffer = append(spansToSend, t.spanBuffer...)
		t.bufferMutex.Unlock()
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		t.bufferMutex.Lock()
		t.spanBuffer = append(spansToSend, t.spanBuffer...)
		t.bufferMutex.Unlock()
		return fmt.Errorf("trace flush failed with status %d", resp.StatusCode)
	}

	return nil
}

// Destroy stops the tracer and flushes remaining spans
func (t *Tracer) Destroy() error {
	if t.flushTicker != nil {
		t.flushTicker.Stop()
		close(t.stopChan)
		t.wg.Wait()
	}
	return t.Flush()
}

// W3C Trace Context functions

const (
	traceparentHeader = "traceparent"
	tracestateHeader  = "tracestate"
	traceVersion      = "00"
)

var traceparentRegex = regexp.MustCompile(`^([0-9a-f]{2})-([0-9a-f]{32})-([0-9a-f]{16})-([0-9a-f]{2})$`)

// ParseTraceparent parses a traceparent header into a SpanContext
func ParseTraceparent(header string) *SpanContext {
	if header == "" {
		return nil
	}

	matches := traceparentRegex.FindStringSubmatch(strings.TrimSpace(header))
	if matches == nil || len(matches) != 5 {
		return nil
	}

	version := matches[1]
	traceID := matches[2]
	spanID := matches[3]
	flags := matches[4]

	if version != traceVersion {
		return nil
	}

	// Validate not all zeros
	if traceID == strings.Repeat("0", 32) || spanID == strings.Repeat("0", 16) {
		return nil
	}

	flagsInt, err := strconv.ParseInt(flags, 16, 64)
	if err != nil {
		return nil
	}

	return &SpanContext{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: int(flagsInt),
	}
}

// FormatTraceparent formats a SpanContext into a traceparent header
func FormatTraceparent(ctx SpanContext) string {
	flags := fmt.Sprintf("%02x", ctx.TraceFlags)
	return fmt.Sprintf("%s-%s-%s-%s", traceVersion, ctx.TraceID, ctx.SpanID, flags)
}

// InjectContext injects trace context into a carrier
func InjectContext(ctx SpanContext, carrier map[string]string) map[string]string {
	carrier[traceparentHeader] = FormatTraceparent(ctx)
	if ctx.TraceState != "" {
		carrier[tracestateHeader] = ctx.TraceState
	}
	return carrier
}

// ExtractContext extracts trace context from a carrier
func ExtractContext(carrier map[string]string) *SpanContext {
	// Try lowercase first, then other cases
	traceparent := carrier[traceparentHeader]
	if traceparent == "" {
		traceparent = carrier["Traceparent"]
	}
	if traceparent == "" {
		traceparent = carrier["TRACEPARENT"]
	}

	if traceparent == "" {
		return nil
	}

	return ParseTraceparent(traceparent)
}

// WithSpan wraps a function with a span
func WithSpan(t *Tracer, operationName string, fn func() error, attrs SpanAttributes) error {
	span := t.StartSpan(operationName, &SpanOptions{
		Kind:       SpanKindInternal,
		Attributes: attrs,
	})

	err := fn()
	if err != nil {
		span.SetStatus(SpanStatusError, err.Error())
		span.AddEvent("exception", SpanAttributes{
			"exception.message": err.Error(),
		})
	} else {
		span.SetStatus(SpanStatusOK, "")
	}
	span.End()

	return err
}

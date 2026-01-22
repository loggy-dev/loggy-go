package loggy

import (
	"net/http"
	"time"
)

// TracingMiddlewareOptions configures the tracing middleware
type TracingMiddlewareOptions struct {
	Tracer             *Tracer
	IgnoreRoutes       []string
	RecordRequestBody  bool
	RecordResponseBody bool
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// TracingMiddleware creates HTTP middleware for automatic request tracing
func TracingMiddleware(opts TracingMiddlewareOptions) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip ignored routes
			for _, route := range opts.IgnoreRoutes {
				if len(r.URL.Path) >= len(route) && r.URL.Path[:len(route)] == route {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Extract parent context from incoming headers
			headers := make(map[string]string)
			for key := range r.Header {
				headers[key] = r.Header.Get(key)
			}
			parentContext := opts.Tracer.Extract(headers)

			// Build operation name
			operationName := r.Method + " " + r.URL.Path

			// Start span
			spanOpts := &SpanOptions{
				Kind: SpanKindServer,
				Attributes: SpanAttributes{
					"http.method":     r.Method,
					"http.url":        r.URL.String(),
					"http.target":     r.URL.Path,
					"http.host":       r.Host,
					"http.scheme":     r.URL.Scheme,
					"http.user_agent": r.UserAgent(),
					"net.peer.ip":     r.RemoteAddr,
				},
			}
			if parentContext != nil {
				spanOpts.Parent = parentContext
			}

			span := opts.Tracer.StartSpan(operationName, spanOpts)

			// Wrap response writer to capture status code
			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			// Serve the request
			next.ServeHTTP(wrapped, r)

			// Set response attributes
			span.SetAttribute("http.status_code", wrapped.statusCode)

			// Set status based on HTTP status code
			if wrapped.statusCode >= 400 {
				span.SetStatus(SpanStatusError, http.StatusText(wrapped.statusCode))
			} else {
				span.SetStatus(SpanStatusOK, "")
			}

			span.End()
		})
	}
}

// MetricsMiddleware creates HTTP middleware for automatic metrics tracking
func MetricsMiddleware(m *Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			timer := m.StartRequest()

			// Wrap response writer to capture status code
			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(wrapped, r)

			timer.End(RequestEndOptions{
				StatusCode: wrapped.statusCode,
				Path:       r.URL.Path,
				Method:     r.Method,
			})
		})
	}
}

// LoggingMiddleware creates HTTP middleware for request logging
func LoggingMiddleware(logger *Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap response writer to capture status code
			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(wrapped, r)

			duration := time.Since(start)

			logger.Info(r.Method+" "+r.URL.Path, map[string]interface{}{
				"status":   wrapped.statusCode,
				"duration": duration.String(),
				"method":   r.Method,
				"path":     r.URL.Path,
				"ip":       r.RemoteAddr,
			})
		})
	}
}

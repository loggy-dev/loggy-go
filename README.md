# Loggy Go

A simple, lightweight, and colorful logger for Go applications with remote logging, performance metrics, and distributed tracing support.

[![Go Reference](https://pkg.go.dev/badge/github.com/loggy-dev/loggy-go.svg)](https://pkg.go.dev/github.com/loggy-dev/loggy-go)

## Features

- **Colorful output** with customizable colors
- **Log levels** - debug, info, warn, error
- **Timestamps** - optional timestamp prefixes
- **Metadata** - structured JSON metadata support
- **Tags** - user-defined tags for categorization
- **Remote logging** - send logs to Loggy.dev for centralized viewing
- **Performance metrics** - track RPM, response times, and throughput
- **Distributed tracing** - W3C Trace Context support

## Installation

```bash
go get github.com/loggy-dev/loggy-go
```

## Usage

### Basic Logging

```go
package main

import (
    "github.com/loggy-dev/loggy-go"
)

func main() {
    logger := loggy.New(loggy.Config{
        Identifier: "my-app",
        Color:      true,
        Compact:    false,
        Timestamp:  true,
    })

    logger.Log("This is a log message")
    logger.Info("This is an info message")
    logger.Warn("This is a warn message")
    logger.Error("This is an error message")

    // Log with metadata
    logger.Info("User logged in", map[string]interface{}{
        "userId": 123,
        "email":  "user@example.com",
    })

    // Add blank lines
    logger.Blank(2)
}
```

### Remote Logging (Loggy.dev)

Send your logs to Loggy.dev for centralized viewing and searching:

```go
logger := loggy.New(loggy.Config{
    Identifier: "my-app",
    Remote: &loggy.RemoteConfig{
        Token:         "your-project-token", // Get this from loggy.dev dashboard
        Endpoint:      "https://loggy.dev/api/logs/ingest", // Optional
        BatchSize:     50,                   // Optional (default: 50)
        FlushInterval: 5 * time.Second,      // Optional (default: 5s)
    },
})

logger.Info("This log will appear locally AND on loggy.dev")

// Manually flush logs (useful before process exit)
logger.Flush()

// Clean up on shutdown
defer logger.Destroy()
```

## Configuration Options

| Option       | Type           | Default | Description                              |
| :----------- | :------------- | :------ | :--------------------------------------- |
| `Identifier` | string         | -       | Label for your app/service               |
| `Color`      | bool           | false   | Enable colored output                    |
| `Compact`    | bool           | false   | Compact mode for metadata inspection     |
| `Timestamp`  | bool           | false   | Show timestamps in log output            |
| `Remote`     | *RemoteConfig  | nil     | Remote logging configuration             |

### Remote Configuration

| Option          | Type          | Default                              | Description                        |
| :-------------- | :------------ | :----------------------------------- | :--------------------------------- |
| `Token`         | string        | -                                    | Project token from loggy.dev       |
| `Endpoint`      | string        | `https://loggy.dev/api/logs/ingest`  | API endpoint for log ingestion     |
| `BatchSize`     | int           | `50`                                 | Logs to batch before sending       |
| `FlushInterval` | time.Duration | `5s`                                 | Duration between auto-flushes      |
| `PublicKey`     | string        | -                                    | RSA public key for encryption      |

## Performance Metrics (Pro/Team)

Track request-per-minute (RPM) and throughput metrics:

```go
metrics := loggy.NewMetrics(loggy.MetricsConfig{
    Token:         "your-project-token",
    Endpoint:      "https://loggy.dev/api/metrics/ingest", // Optional
    FlushInterval: 60 * time.Second,                       // Optional
})

// Option 1: Manual tracking with StartRequest
func handleRequest(w http.ResponseWriter, r *http.Request) {
    timer := metrics.StartRequest()
    
    // ... handle request ...
    
    timer.End(loggy.RequestEndOptions{
        StatusCode: 200,
        BytesIn:    r.ContentLength,
        BytesOut:   responseSize,
        Path:       r.URL.Path,
        Method:     r.Method,
    })
}

// Option 2: Use middleware
mux := http.NewServeMux()
handler := loggy.MetricsMiddleware(metrics)(mux)

// Clean up on shutdown
defer metrics.Destroy()
```

### Metrics Configuration

| Option          | Type          | Default                                | Description                        |
| :-------------- | :------------ | :------------------------------------- | :--------------------------------- |
| `Token`         | string        | -                                      | Project token from loggy.dev       |
| `Endpoint`      | string        | `https://loggy.dev/api/metrics/ingest` | API endpoint for metrics ingestion |
| `FlushInterval` | time.Duration | `60s`                                  | Duration between auto-flushes      |
| `Disabled`      | bool          | `false`                                | Disable metrics collection         |

## Distributed Tracing (Pro/Team)

Track requests as they flow through your microservices:

### Basic Setup

```go
tracer := loggy.NewTracer(loggy.TracerConfig{
    ServiceName:    "api-gateway",
    ServiceVersion: "1.0.0",
    Environment:    "production",
    Remote: &loggy.TracerRemoteConfig{
        Token:         "your-project-token",
        Endpoint:      "https://loggy.dev/api/traces/ingest", // Optional
        BatchSize:     100,              // Optional
        FlushInterval: 5 * time.Second,  // Optional
    },
})

// Use middleware for automatic request tracing
mux := http.NewServeMux()
handler := loggy.TracingMiddleware(loggy.TracingMiddlewareOptions{
    Tracer: tracer,
})(mux)

defer tracer.Destroy()
```

### Manual Span Creation

```go
// Start a span for a database query
span := tracer.StartSpan("db.query", &loggy.SpanOptions{
    Kind: loggy.SpanKindClient,
    Attributes: loggy.SpanAttributes{
        "db.system":    "postgresql",
        "db.statement": "SELECT * FROM users WHERE id = $1",
    },
})

result, err := db.Query("SELECT * FROM users WHERE id = $1", userID)
if err != nil {
    span.SetStatus(loggy.SpanStatusError, err.Error())
    span.AddEvent("exception", loggy.SpanAttributes{
        "exception.message": err.Error(),
    })
} else {
    span.SetStatus(loggy.SpanStatusOK, "")
}
span.End()
```

### Using WithSpan Helper

```go
err := loggy.WithSpan(tracer, "fetchUserData", func() error {
    user, err := db.Query("SELECT * FROM users WHERE id = $1", userID)
    return err
}, loggy.SpanAttributes{"user.id": userID})
```

### Context Propagation

```go
// Inject trace context into outgoing requests
headers := tracer.Inject(map[string]string{})
req, _ := http.NewRequest("GET", "http://user-service/api/users/123", nil)
for k, v := range headers {
    req.Header.Set(k, v)
}

// Extract trace context from incoming requests
headers := make(map[string]string)
for key := range r.Header {
    headers[key] = r.Header.Get(key)
}
parentContext := tracer.Extract(headers)
span := tracer.StartSpan("handle-request", &loggy.SpanOptions{Parent: parentContext})
```

### Tracer Configuration

| Option             | Type                 | Default                                | Description                        |
| :----------------- | :------------------- | :------------------------------------- | :--------------------------------- |
| `ServiceName`      | string               | -                                      | Name of your service (required)    |
| `ServiceVersion`   | string               | -                                      | Version of your service            |
| `Environment`      | string               | -                                      | Deployment environment             |
| `Remote.Token`     | string               | -                                      | Project token from loggy.dev       |
| `Remote.Endpoint`  | string               | `https://loggy.dev/api/traces/ingest`  | API endpoint for trace ingestion   |
| `Remote.BatchSize` | int                  | `100`                                  | Spans to batch before sending      |
| `Remote.FlushInterval` | time.Duration    | `5s`                                   | Duration between auto-flushes      |

### Span Kinds

- `SpanKindServer` - Incoming request handler
- `SpanKindClient` - Outgoing request to another service
- `SpanKindProducer` - Message queue producer
- `SpanKindConsumer` - Message queue consumer
- `SpanKindInternal` - Internal operation (default)

## Log Levels

Use the appropriate method for each log level:

- `logger.Log()` - General logging (debug level)
- `logger.Info()` - Informational messages
- `logger.Warn()` - Warning messages
- `logger.Error()` - Error messages

## HTTP Middleware

The package provides middleware for common Go HTTP frameworks:

```go
// Tracing middleware
handler := loggy.TracingMiddleware(loggy.TracingMiddlewareOptions{
    Tracer:       tracer,
    IgnoreRoutes: []string{"/health", "/metrics"},
})(mux)

// Metrics middleware
handler := loggy.MetricsMiddleware(metrics)(mux)

// Logging middleware
handler := loggy.LoggingMiddleware(logger)(mux)
```

## Publishing

To publish a new version:

1. Update the version tag
2. Push to GitHub:

```bash
git tag v0.1.0
git push origin v0.1.0
```

3. The module will be available at `github.com/loggy-dev/loggy-go`

## License

ISC
# loggy-go

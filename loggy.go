// Package loggy provides a lightweight and colorful logger for Go applications
// with remote logging support to Loggy.dev
package loggy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/fatih/color"
)

// LogLevel represents the severity level of a log entry
type LogLevel string

const (
	LevelDebug LogLevel = "debug"
	LevelInfo  LogLevel = "info"
	LevelWarn  LogLevel = "warn"
	LevelError LogLevel = "error"
)

// LogEntry represents a single log entry
type LogEntry struct {
	Level     LogLevel               `json:"level"`
	Message   string                 `json:"message"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Tags      []string               `json:"tags,omitempty"`
	Timestamp string                 `json:"timestamp"`
}

// RemoteConfig configures remote logging to Loggy.dev
type RemoteConfig struct {
	Token         string
	Endpoint      string
	BatchSize     int
	FlushInterval time.Duration
	PublicKey     string // For end-to-end encryption (optional)
}

// Config configures the Loggy logger
type Config struct {
	Identifier string
	Color      bool
	Compact    bool
	Timestamp  bool
	Remote     *RemoteConfig
}

// Logger is the main Loggy logger instance
type Logger struct {
	config      Config
	logBuffer   []LogEntry
	bufferMutex sync.Mutex
	flushTicker *time.Ticker
	stopChan    chan struct{}
	wg          sync.WaitGroup

	// Color printers
	debugColor *color.Color
	infoColor  *color.Color
	warnColor  *color.Color
	errorColor *color.Color
	idColor    *color.Color
	dateColor  *color.Color
	timeColor  *color.Color
}

// New creates a new Loggy logger instance
func New(config Config) *Logger {
	// Set defaults
	if config.Remote != nil {
		if config.Remote.Endpoint == "" {
			config.Remote.Endpoint = "https://loggy.dev/api/logs/ingest"
		}
		if config.Remote.BatchSize == 0 {
			config.Remote.BatchSize = 50
		}
		if config.Remote.FlushInterval == 0 {
			config.Remote.FlushInterval = 5 * time.Second
		}
	}

	l := &Logger{
		config:      config,
		logBuffer:   make([]LogEntry, 0),
		stopChan:    make(chan struct{}),
		debugColor:  color.New(color.FgHiCyan),
		infoColor:   color.New(color.FgHiCyan),
		warnColor:   color.New(color.FgYellow),
		errorColor:  color.New(color.FgRed),
		idColor:     color.New(color.FgHiWhite),
		dateColor:   color.New(color.FgMagenta),
		timeColor:   color.New(color.FgHiYellow),
	}

	// Start flush timer if remote is configured
	if config.Remote != nil && config.Remote.Token != "" {
		l.flushTicker = time.NewTicker(config.Remote.FlushInterval)
		l.wg.Add(1)
		go l.flushLoop()
	}

	return l
}

func (l *Logger) flushLoop() {
	defer l.wg.Done()
	for {
		select {
		case <-l.flushTicker.C:
			l.Flush()
		case <-l.stopChan:
			return
		}
	}
}

func (l *Logger) formatTimestamp() string {
	now := time.Now()
	if !l.config.Color {
		return now.Format("01/02/2006 15:04:05")
	}
	date := l.dateColor.Sprint(now.Format("01/02/2006"))
	timeStr := l.timeColor.Sprint(now.Format("15:04:05"))
	return fmt.Sprintf("%s %s", date, timeStr)
}

func (l *Logger) formatLevel(level LogLevel) string {
	levelStr := string(level)
	if level == LevelDebug {
		levelStr = "LOG"
	} else {
		levelStr = string(level)
	}

	if !l.config.Color {
		return fmt.Sprintf("[%s]", levelStr)
	}

	var coloredLevel string
	switch level {
	case LevelDebug:
		coloredLevel = l.debugColor.Sprint("LOG")
	case LevelInfo:
		coloredLevel = l.infoColor.Sprint("INFO")
	case LevelWarn:
		coloredLevel = l.warnColor.Sprint("WARN")
	case LevelError:
		coloredLevel = l.errorColor.Sprint("ERROR")
	}
	return fmt.Sprintf("[%s]", coloredLevel)
}

func (l *Logger) formatIdentifier() string {
	if !l.config.Color {
		return l.config.Identifier
	}
	return l.idColor.Sprint(l.config.Identifier)
}

func (l *Logger) formatMetadata(metadata map[string]interface{}) string {
	if len(metadata) == 0 {
		return ""
	}

	var output []byte
	var err error
	if l.config.Compact {
		output, err = json.Marshal(metadata)
	} else {
		output, err = json.MarshalIndent(metadata, "", "  ")
	}
	if err != nil {
		return ""
	}
	return "\n" + string(output)
}

func (l *Logger) log(level LogLevel, message string, metadata map[string]interface{}, tags []string) {
	// Format and print to console
	var prefix string
	if l.config.Timestamp {
		prefix = l.formatTimestamp() + " "
	}
	levelStr := l.formatLevel(level)
	idStr := l.formatIdentifier()
	metaStr := l.formatMetadata(metadata)

	output := fmt.Sprintf("%s%s %s: %s%s", prefix, levelStr, idStr, message, metaStr)

	switch level {
	case LevelError:
		fmt.Fprintln(os.Stderr, output)
	default:
		fmt.Println(output)
	}

	// Queue for remote logging
	l.queueLog(LogEntry{
		Level:     level,
		Message:   message,
		Metadata:  metadata,
		Tags:      tags,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

func (l *Logger) queueLog(entry LogEntry) {
	if l.config.Remote == nil || l.config.Remote.Token == "" {
		return
	}

	l.bufferMutex.Lock()
	l.logBuffer = append(l.logBuffer, entry)
	shouldFlush := len(l.logBuffer) >= l.config.Remote.BatchSize
	l.bufferMutex.Unlock()

	if shouldFlush {
		go l.Flush()
	}
}

// Log logs a message at debug level
func (l *Logger) Log(message string, metadata ...map[string]interface{}) {
	var meta map[string]interface{}
	if len(metadata) > 0 {
		meta = metadata[0]
	}
	l.log(LevelDebug, message, meta, nil)
}

// LogWithTags logs a message at debug level with tags
func (l *Logger) LogWithTags(message string, tags []string, metadata ...map[string]interface{}) {
	var meta map[string]interface{}
	if len(metadata) > 0 {
		meta = metadata[0]
	}
	l.log(LevelDebug, message, meta, tags)
}

// Info logs a message at info level
func (l *Logger) Info(message string, metadata ...map[string]interface{}) {
	var meta map[string]interface{}
	if len(metadata) > 0 {
		meta = metadata[0]
	}
	l.log(LevelInfo, message, meta, nil)
}

// InfoWithTags logs a message at info level with tags
func (l *Logger) InfoWithTags(message string, tags []string, metadata ...map[string]interface{}) {
	var meta map[string]interface{}
	if len(metadata) > 0 {
		meta = metadata[0]
	}
	l.log(LevelInfo, message, meta, tags)
}

// Warn logs a message at warn level
func (l *Logger) Warn(message string, metadata ...map[string]interface{}) {
	var meta map[string]interface{}
	if len(metadata) > 0 {
		meta = metadata[0]
	}
	l.log(LevelWarn, message, meta, nil)
}

// WarnWithTags logs a message at warn level with tags
func (l *Logger) WarnWithTags(message string, tags []string, metadata ...map[string]interface{}) {
	var meta map[string]interface{}
	if len(metadata) > 0 {
		meta = metadata[0]
	}
	l.log(LevelWarn, message, meta, tags)
}

// Error logs a message at error level
func (l *Logger) Error(message string, metadata ...map[string]interface{}) {
	var meta map[string]interface{}
	if len(metadata) > 0 {
		meta = metadata[0]
	}
	l.log(LevelError, message, meta, nil)
}

// ErrorWithTags logs a message at error level with tags
func (l *Logger) ErrorWithTags(message string, tags []string, metadata ...map[string]interface{}) {
	var meta map[string]interface{}
	if len(metadata) > 0 {
		meta = metadata[0]
	}
	l.log(LevelError, message, meta, tags)
}

// Blank prints blank lines
func (l *Logger) Blank(lines int) {
	for i := 0; i < lines; i++ {
		fmt.Println()
	}
}

// Flush sends all buffered logs to the remote server
func (l *Logger) Flush() error {
	if l.config.Remote == nil || l.config.Remote.Token == "" {
		return nil
	}

	l.bufferMutex.Lock()
	if len(l.logBuffer) == 0 {
		l.bufferMutex.Unlock()
		return nil
	}

	logsToSend := make([]LogEntry, len(l.logBuffer))
	copy(logsToSend, l.logBuffer)
	l.logBuffer = l.logBuffer[:0]
	l.bufferMutex.Unlock()

	payload := map[string]interface{}{
		"logs": logsToSend,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		// Put logs back in buffer
		l.bufferMutex.Lock()
		l.logBuffer = append(logsToSend, l.logBuffer...)
		l.bufferMutex.Unlock()
		return err
	}

	req, err := http.NewRequest("POST", l.config.Remote.Endpoint, bytes.NewReader(body))
	if err != nil {
		l.bufferMutex.Lock()
		l.logBuffer = append(logsToSend, l.logBuffer...)
		l.bufferMutex.Unlock()
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-loggy-token", l.config.Remote.Token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		l.bufferMutex.Lock()
		l.logBuffer = append(logsToSend, l.logBuffer...)
		l.bufferMutex.Unlock()
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		l.bufferMutex.Lock()
		l.logBuffer = append(logsToSend, l.logBuffer...)
		l.bufferMutex.Unlock()
		return fmt.Errorf("remote logging failed with status %d", resp.StatusCode)
	}

	return nil
}

// Destroy stops the logger and flushes remaining logs
func (l *Logger) Destroy() error {
	if l.flushTicker != nil {
		l.flushTicker.Stop()
		close(l.stopChan)
		l.wg.Wait()
	}
	return l.Flush()
}

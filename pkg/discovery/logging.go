// Package discovery provides application discovery and analysis functionality.
package discovery

import (
	"encoding/json"
	"io"
	"os"
	"time"
)

// Logger provides structured JSON logging for discovery operations.
type Logger struct {
	writer    io.Writer
	level     LogLevel
	component string
	fields    map[string]interface{}
}

// LogLevel represents the severity of a log message.
type LogLevel int

const (
	// LevelDebug is for detailed debugging information.
	LevelDebug LogLevel = iota
	// LevelInfo is for general operational information.
	LevelInfo
	// LevelWarn is for warning messages.
	LevelWarn
	// LevelError is for error messages.
	LevelError
)

// LogEntry represents a structured log entry.
type LogEntry struct {
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	Component string                 `json:"component,omitempty"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
	Error     string                 `json:"error,omitempty"`
}

// LoggerOptions configures the logger.
type LoggerOptions struct {
	// Writer is where to write log entries.
	Writer io.Writer

	// Level is the minimum log level to output.
	Level LogLevel

	// Component is the component name to include in logs.
	Component string

	// Fields are default fields to include in all log entries.
	Fields map[string]interface{}
}

// NewLogger creates a new structured JSON logger.
func NewLogger(opts LoggerOptions) *Logger {
	if opts.Writer == nil {
		opts.Writer = os.Stdout
	}

	return &Logger{
		writer:    opts.Writer,
		level:     opts.Level,
		component: opts.Component,
		fields:    opts.Fields,
	}
}

// WithComponent creates a new logger with a specific component name.
func (l *Logger) WithComponent(component string) *Logger {
	return &Logger{
		writer:    l.writer,
		level:     l.level,
		component: component,
		fields:    l.fields,
	}
}

// WithFields creates a new logger with additional default fields.
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	mergedFields := make(map[string]interface{})
	for k, v := range l.fields {
		mergedFields[k] = v
	}
	for k, v := range fields {
		mergedFields[k] = v
	}

	return &Logger{
		writer:    l.writer,
		level:     l.level,
		component: l.component,
		fields:    mergedFields,
	}
}

// Debug logs a debug-level message.
func (l *Logger) Debug(msg string, fields ...map[string]interface{}) {
	l.log(LevelDebug, msg, nil, fields...)
}

// Info logs an info-level message.
func (l *Logger) Info(msg string, fields ...map[string]interface{}) {
	l.log(LevelInfo, msg, nil, fields...)
}

// Warn logs a warning-level message.
func (l *Logger) Warn(msg string, fields ...map[string]interface{}) {
	l.log(LevelWarn, msg, nil, fields...)
}

// Error logs an error-level message.
func (l *Logger) Error(msg string, err error, fields ...map[string]interface{}) {
	l.log(LevelError, msg, err, fields...)
}

func (l *Logger) log(level LogLevel, msg string, err error, extraFields ...map[string]interface{}) {
	if level < l.level {
		return
	}

	entry := LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Level:     levelToString(level),
		Component: l.component,
		Message:   msg,
	}

	// Merge fields
	if len(l.fields) > 0 || len(extraFields) > 0 {
		entry.Fields = make(map[string]interface{})
		for k, v := range l.fields {
			entry.Fields[k] = v
		}
		for _, fields := range extraFields {
			for k, v := range fields {
				entry.Fields[k] = v
			}
		}
	}

	if err != nil {
		entry.Error = err.Error()
	}

	data, jsonErr := json.Marshal(entry)
	if jsonErr != nil {
		// Fallback to simple output
		l.writer.Write([]byte(msg + "\n"))
		return
	}

	l.writer.Write(append(data, '\n'))
}

func levelToString(level LogLevel) string {
	switch level {
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	default:
		return "unknown"
	}
}

// DiscoveryLogFields returns standard fields for discovery operations.
func DiscoveryLogFields(projectPath string, operation string) map[string]interface{} {
	return map[string]interface{}{
		"project_path": projectPath,
		"operation":    operation,
	}
}

// SkillLogFields returns standard fields for skill execution.
func SkillLogFields(skillName string, duration time.Duration) map[string]interface{} {
	return map[string]interface{}{
		"skill":       skillName,
		"duration_ms": duration.Milliseconds(),
	}
}

// AnalyzerLogFields returns standard fields for analyzer operations.
func AnalyzerLogFields(analyzerName string, filesAnalyzed int, dependenciesFound int) map[string]interface{} {
	return map[string]interface{}{
		"analyzer":           analyzerName,
		"files_analyzed":     filesAnalyzed,
		"dependencies_found": dependenciesFound,
	}
}

// MCPLogFields returns standard fields for MCP operations.
func MCPLogFields(method string, toolName string, requestID string) map[string]interface{} {
	return map[string]interface{}{
		"method":     method,
		"tool":       toolName,
		"request_id": requestID,
	}
}

// DefaultLogger is a package-level logger instance.
var DefaultLogger = NewLogger(LoggerOptions{
	Writer: os.Stdout,
	Level:  LevelInfo,
})

// SetDefaultLogger sets the package-level logger.
func SetDefaultLogger(logger *Logger) {
	DefaultLogger = logger
}

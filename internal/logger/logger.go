package logger

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type LogLevel int

const (
	DebugLevel LogLevel = iota
	InfoLevel
	WarnLevel
	ErrorLevel
)

func (l LogLevel) String() string {
	switch l {
	case DebugLevel:
		return "DEBUG"
	case InfoLevel:
		return "INFO"
	case WarnLevel:
		return "WARN"
	case ErrorLevel:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

type LogEntry struct {
	Time      string                 `json:"time"`
	Level     string                 `json:"level"`
	ThreadID  string                 `json:"thread_id"`
	Event     string                 `json:"event"`
	Tool      string                 `json:"tool,omitempty"`
	Result    string                 `json:"result,omitempty"`
	Message   string                 `json:"message,omitempty"`
	ExtraData map[string]interface{} `json:"extra,omitempty"`
}

type AuditLogger struct {
	mu         sync.Mutex
	logDir     string
	currentDay string
	file       *os.File
}

var (
	globalAuditLogger *AuditLogger
	auditOnce         sync.Once
)

func InitAuditLogger(logDir string) error {
	var initErr error
	auditOnce.Do(func() {
		globalAuditLogger = &AuditLogger{
			logDir: logDir,
		}
		initErr = globalAuditLogger.rotateFile()
	})
	return initErr
}

func GetAuditLogger() *AuditLogger {
	return globalAuditLogger
}

func (l *AuditLogger) rotateFile() error {
	today := time.Now().Format("2006-01-02")
	if l.currentDay == today && l.file != nil {
		return nil
	}

	if l.file != nil {
		l.file.Close()
	}

	os.MkdirAll(l.logDir, 0755)
	filename := filepath.Join(l.logDir, fmt.Sprintf("audit_%s.json", today))
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	l.file = f
	l.currentDay = today
	return nil
}

func (l *AuditLogger) LogEvent(threadID, event, tool, result string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.rotateFile()

	entry := LogEntry{
		Time:     time.Now().Format(time.RFC3339),
		Level:    "INFO",
		ThreadID: threadID,
		Event:    event,
		Tool:     tool,
		Result:   truncate(result, 200),
	}

	data, _ := json.Marshal(entry)
	l.file.Write(append(data, '\n'))
}

func (l *AuditLogger) LogError(threadID, event, message string, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.rotateFile()

	entry := LogEntry{
		Time:     time.Now().Format(time.RFC3339),
		Level:    "ERROR",
		ThreadID: threadID,
		Event:    event,
		Message:  message,
	}
	if err != nil {
		entry.ExtraData = map[string]interface{}{"error": err.Error()}
	}

	data, _ := json.Marshal(entry)
	l.file.Write(append(data, '\n'))
}

func (l *AuditLogger) LogToolResult(threadID, toolName, result string) {
	l.LogEvent(threadID, "tool_result", toolName, result)
}

func (l *AuditLogger) LogToolCall(threadID, toolName string, args map[string]interface{}) {
	argsJSON, _ := json.Marshal(args)
	l.LogEvent(threadID, "tool_call", toolName, string(argsJSON))
}

func (l *AuditLogger) LogLLMInput(threadID string, msgCount int) {
	l.LogEvent(threadID, "llm_input", "", fmt.Sprintf("message_count=%d", msgCount))
}

func (l *AuditLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func LogEvent(threadID, event, tool, result string) {
	if globalAuditLogger != nil {
		globalAuditLogger.LogEvent(threadID, event, tool, result)
	}
}

func LogError(threadID, event, message string, err error) {
	if globalAuditLogger != nil {
		globalAuditLogger.LogError(threadID, event, message, err)
	}
}

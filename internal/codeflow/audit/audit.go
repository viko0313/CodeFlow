package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Event struct {
	Time          string `json:"time"`
	SessionID     string `json:"session_id"`
	ProjectRoot   string `json:"project_root"`
	OperationID   string `json:"operation_id,omitempty"`
	Event         string `json:"event"`
	ToolName      string `json:"tool_name,omitempty"`
	ArgsSummary   string `json:"args_summary,omitempty"`
	ResultSummary string `json:"result_summary,omitempty"`
	DurationMS    int64  `json:"duration_ms,omitempty"`
	Confirmed     *bool  `json:"confirmed,omitempty"`
}

type Logger struct {
	mu  sync.Mutex
	dir string
}

func NewLogger(dataDir string) (*Logger, error) {
	dir := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	return &Logger{dir: dir}, nil
}

func (l *Logger) Record(event Event) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if event.Time == "" {
		event.Time = time.Now().Format(time.RFC3339)
	}
	path := filepath.Join(l.dir, "audit_"+time.Now().Format("2006-01-02")+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = f.Write(append(data, '\n'))
	return err
}

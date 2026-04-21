package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	hertzserver "github.com/cloudwego/hertz/pkg/app/server"
	"github.com/gorilla/websocket"

	cfconfig "github.com/cloudwego/codeflow/internal/codeflow/config"
	"github.com/cloudwego/codeflow/internal/codeflow/engine"
	cfsession "github.com/cloudwego/codeflow/internal/codeflow/session"
)

func TestSessionAPIListAndSwitch(t *testing.T) {
	store := newFakeStore(t.TempDir())
	server := NewServer(Dependencies{ProjectRoot: store.root, Config: testConfig(), Store: store})
	baseURL, cleanup := startHertzTestServer(t, server)
	defer cleanup()

	resp, err := http.Get(baseURL + "/api/sessions")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET sessions status = %d", resp.StatusCode)
	}
	var payload struct {
		Sessions []cfsession.Session `json:"sessions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(payload.Sessions))
	}

	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/sessions/s2/switch", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("switch status = %d", resp.StatusCode)
	}
	if active, _ := store.GetActive(store.root); active == nil || active.ID != "s2" {
		t.Fatalf("active session was not switched: %#v", active)
	}
}

func TestWebSocketPermissionApproveRunsShell(t *testing.T) {
	store := newFakeStore(t.TempDir())
	server := NewServer(Dependencies{ProjectRoot: store.root, Config: testConfig(), Store: store, Engine: fakeEngine{}})
	baseURL, cleanup := startHertzTestServer(t, server)
	defer cleanup()

	conn := dialWS(t, baseURL, "s1")
	defer conn.Close()
	readUntil(t, conn, "session.updated")
	if err := conn.WriteJSON(clientMessage{Type: "terminal.run", ID: "run1", Command: "echo codeflow"}); err != nil {
		t.Fatal(err)
	}
	required := readUntil(t, conn, "permission.required")
	if required.OperationID == "" {
		t.Fatal("permission.required did not include operation_id")
	}
	if err := conn.WriteJSON(clientMessage{Type: "permission.decide", OperationID: required.OperationID, Allowed: true}); err != nil {
		t.Fatal(err)
	}
	output := readUntil(t, conn, "terminal.output")
	if !strings.Contains(strings.ToLower(output.Output), "codeflow") {
		t.Fatalf("terminal output did not include command output: %q", output.Output)
	}
	done := readUntil(t, conn, "operation.done")
	if done.Confirmed == nil || !*done.Confirmed {
		t.Fatalf("operation was not confirmed: %#v", done)
	}
}

func TestWebSocketBlockedShellCommand(t *testing.T) {
	store := newFakeStore(t.TempDir())
	server := NewServer(Dependencies{ProjectRoot: store.root, Config: testConfig(), Store: store})
	baseURL, cleanup := startHertzTestServer(t, server)
	defer cleanup()

	conn := dialWS(t, baseURL, "s1")
	defer conn.Close()
	readUntil(t, conn, "session.updated")
	if err := conn.WriteJSON(clientMessage{Type: "terminal.run", ID: "bad1", Command: `python -c "print(1)"`}); err != nil {
		t.Fatal(err)
	}
	event := readUntil(t, conn, "operation.error")
	if !strings.Contains(event.Error, "blocked") {
		t.Fatalf("expected blocked command error, got %q", event.Error)
	}
}

func TestWebSocketUnknownMessageValidation(t *testing.T) {
	store := newFakeStore(t.TempDir())
	server := NewServer(Dependencies{ProjectRoot: store.root, Config: testConfig(), Store: store})
	baseURL, cleanup := startHertzTestServer(t, server)
	defer cleanup()

	conn := dialWS(t, baseURL, "s1")
	defer conn.Close()
	readUntil(t, conn, "session.updated")
	if err := conn.WriteJSON(clientMessage{Type: "bogus", ID: "oops"}); err != nil {
		t.Fatal(err)
	}
	event := readUntil(t, conn, "operation.error")
	if !strings.Contains(event.Error, "unknown message type") {
		t.Fatalf("expected validation error, got %q", event.Error)
	}
}

func TestDocumentUploadAPI(t *testing.T) {
	store := newFakeStore(t.TempDir())
	cfg := testConfig()
	cfg.Documents.UploadDir = t.TempDir()
	cfg.Documents.AllowedExtensions = []string{".txt"}
	cfg.Documents.MaxUploadBytes = 1024 * 1024
	server := NewServer(Dependencies{ProjectRoot: store.root, Config: cfg, Store: store})
	baseURL, cleanup := startHertzTestServer(t, server)
	defer cleanup()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "note.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("hello uploaded document")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/documents/upload", &body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload status = %d", resp.StatusCode)
	}
	var uploaded struct {
		FileName string `json:"file_name"`
		Content  string `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&uploaded); err != nil {
		t.Fatal(err)
	}
	if uploaded.FileName != "note.txt" || !strings.Contains(uploaded.Content, "hello uploaded document") {
		t.Fatalf("unexpected upload response: %+v", uploaded)
	}
}

func startHertzTestServer(t *testing.T, s *Server) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	h := hertzserver.Default(
		hertzserver.WithListener(ln),
		hertzserver.WithDisablePrintRoute(true),
		hertzserver.WithExitWaitTime(50*time.Millisecond),
	)
	done := make(chan struct{})
	h.SetCustomSignalWaiter(func(errCh chan error) error {
		select {
		case err := <-errCh:
			return err
		case <-done:
			return nil
		}
	})
	s.Routes(h)
	go h.Spin()
	baseURL := "http://" + ln.Addr().String()
	waitForHTTP(t, baseURL+"/api/health")
	return baseURL, func() {
		close(done)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = h.Shutdown(shutdownCtx)
	}
}

func waitForHTTP(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode < 500 {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("server did not become ready: %s", url)
}

func dialWS(t *testing.T, baseURL, sessionID string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(baseURL, "http") + "/api/ws?session_id=" + sessionID
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

func readUntil(t *testing.T, conn *websocket.Conn, eventType string) serverMessage {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var event serverMessage
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatal(err)
		}
		if event.Type == eventType {
			return event
		}
	}
	t.Fatalf("timed out waiting for %s", eventType)
	return serverMessage{}
}

func testConfig() *cfconfig.Config {
	return &cfconfig.Config{
		Provider: "test",
		Model:    "test-model",
		Storage:  cfconfig.StorageConfig{RedisAddr: "localhost:6379"},
		Runtime:  cfconfig.RuntimeConfig{MaxTurns: 50, MaxActions: 20},
		Agent:    cfconfig.AgentConfig{Mode: "react"},
		Skills:   cfconfig.SkillsConfig{Enabled: true, MaxContentBytes: 6000},
		MCP:      cfconfig.MCPConfig{Enabled: true, Preload: true},
		Documents: cfconfig.DocumentsConfig{
			MaxUploadBytes:    10 * 1024 * 1024,
			AllowedExtensions: []string{".txt", ".md"},
		},
	}
}

type fakeEngine struct{}

func (fakeEngine) Run(ctx context.Context, req engine.Request) (<-chan engine.Event, error) {
	out := make(chan engine.Event, 3)
	out <- engine.Event{Type: engine.EventStatus, Content: "thinking"}
	out <- engine.Event{Type: engine.EventToken, Content: "ok"}
	out <- engine.Event{Type: engine.EventStats, Content: "duration=1ms"}
	close(out)
	return out, nil
}

type fakeStore struct {
	mu       sync.Mutex
	root     string
	sessions []cfsession.Session
}

func newFakeStore(root string) *fakeStore {
	now := time.Now().UTC()
	return &fakeStore{
		root: root,
		sessions: []cfsession.Session{
			{ID: "s1", ProjectRoot: root, Title: "One", Active: true, CreatedAt: now, UpdatedAt: now},
			{ID: "s2", ProjectRoot: root, Title: "Two", Active: false, CreatedAt: now, UpdatedAt: now},
		},
	}
}

func (s *fakeStore) Create(projectRoot, title, agentMD string) (*cfsession.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.sessions {
		s.sessions[i].Active = false
	}
	item := cfsession.Session{
		ID:          fmt.Sprintf("s%d", len(s.sessions)+1),
		ProjectRoot: projectRoot,
		Title:       title,
		AgentMD:     agentMD,
		Active:      true,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	s.sessions = append(s.sessions, item)
	return &item, nil
}

func (s *fakeStore) GetActive(projectRoot string) (*cfsession.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.sessions {
		if item.ProjectRoot == projectRoot && item.Active {
			copy := item
			return &copy, nil
		}
	}
	return nil, nil
}

func (s *fakeStore) List(projectRoot string) ([]cfsession.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []cfsession.Session
	for _, item := range s.sessions {
		if item.ProjectRoot == projectRoot {
			out = append(out, item)
		}
	}
	return out, nil
}

func (s *fakeStore) Switch(projectRoot, sessionID string) (*cfsession.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var switched *cfsession.Session
	for i := range s.sessions {
		if s.sessions[i].ProjectRoot == projectRoot {
			s.sessions[i].Active = s.sessions[i].ID == sessionID
			if s.sessions[i].Active {
				s.sessions[i].UpdatedAt = time.Now().UTC()
				copy := s.sessions[i]
				switched = &copy
			}
		}
	}
	if switched == nil {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	return switched, nil
}

func (s *fakeStore) Delete(projectRoot, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, item := range s.sessions {
		if item.ProjectRoot == projectRoot && item.ID == sessionID {
			s.sessions = append(s.sessions[:i], s.sessions[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("session not found: %s", sessionID)
}

func (s *fakeStore) Close() {}

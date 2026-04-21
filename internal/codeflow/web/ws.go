package web

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/hertz-contrib/websocket"

	"github.com/cloudwego/codeflow/internal/codeflow/engine"
	"github.com/cloudwego/codeflow/internal/codeflow/permission"
	cftools "github.com/cloudwego/codeflow/internal/codeflow/tools"
)

type clientMessage struct {
	Type           string `json:"type"`
	ID             string `json:"id,omitempty"`
	SessionID      string `json:"session_id,omitempty"`
	Input          string `json:"input,omitempty"`
	Command        string `json:"command,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
	Path           string `json:"path,omitempty"`
	Content        string `json:"content,omitempty"`
	Append         bool   `json:"append,omitempty"`
	OperationID    string `json:"operation_id,omitempty"`
	Allowed        bool   `json:"allowed,omitempty"`
	PlanEnabled    bool   `json:"plan_enabled,omitempty"`
}

type serverMessage struct {
	Type           string `json:"type"`
	ID             string `json:"id,omitempty"`
	SessionID      string `json:"session_id,omitempty"`
	OperationID    string `json:"operation_id,omitempty"`
	Kind           string `json:"kind,omitempty"`
	Path           string `json:"path,omitempty"`
	Command        string `json:"command,omitempty"`
	Preview        string `json:"preview,omitempty"`
	Risk           string `json:"risk,omitempty"`
	Timeout        string `json:"timeout,omitempty"`
	Content        string `json:"content,omitempty"`
	Output         string `json:"output,omitempty"`
	Error          string `json:"error,omitempty"`
	DurationMillis int64  `json:"duration_ms,omitempty"`
	Confirmed      *bool  `json:"confirmed,omitempty"`
}

type wsClient struct {
	server    *Server
	conn      *websocket.Conn
	sessionID string
	sendMu    sync.Mutex
	pendingMu sync.Mutex
	pending   map[string]chan permission.Decision
	executor  *cftools.Executor
}

var upgrader = websocket.HertzUpgrader{
	CheckOrigin: func(ctx *app.RequestContext) bool {
		return true
	},
}

func (s *Server) handleWS(ctx context.Context, c *app.RequestContext) {
	sessionID := strings.TrimSpace(c.Query("session_id"))
	_ = upgrader.Upgrade(c, func(conn *websocket.Conn) {
		if sessionID == "" && s.store != nil {
			if session, err := s.store.GetActive(s.root); err == nil && session != nil {
				sessionID = session.ID
			}
		}
		client := &wsClient{
			server:    s,
			conn:      conn,
			sessionID: sessionID,
			pending:   map[string]chan permission.Decision{},
		}
		client.executor = s.newWebExecutor(client.confirm)
		client.send(serverMessage{Type: "session.updated", SessionID: sessionID})
		client.readLoop(ctx)
	})
}

func (c *wsClient) readLoop(ctx context.Context) {
	defer c.conn.Close()
	for {
		var msg clientMessage
		if err := c.conn.ReadJSON(&msg); err != nil {
			c.rejectAll("connection closed")
			return
		}
		c.handle(ctx, msg)
	}
}

func (c *wsClient) handle(ctx context.Context, msg clientMessage) {
	switch msg.Type {
	case "ping":
		c.send(serverMessage{Type: "pong", ID: msg.ID})
	case "permission.decide":
		c.resolvePermission(msg.OperationID, msg.Allowed)
	case "session.switch":
		c.switchSession(msg.SessionID)
	case "chat.send":
		go c.runChat(ctx, msg)
	case "terminal.run":
		go c.runTerminal(ctx, msg)
	case "file.preview":
		c.previewFile(msg)
	case "file.write":
		go c.writeFile(ctx, msg)
	default:
		c.send(serverMessage{Type: "operation.error", ID: msg.ID, Error: fmt.Sprintf("unknown message type %q", msg.Type)})
	}
}

func (c *wsClient) runChat(ctx context.Context, msg clientMessage) {
	if c.server.engine == nil {
		c.send(serverMessage{Type: "operation.error", ID: msg.ID, Error: "engine unavailable"})
		return
	}
	input := strings.TrimSpace(msg.Input)
	if input == "" {
		c.send(serverMessage{Type: "operation.error", ID: msg.ID, Error: "input is required"})
		return
	}
	events, err := c.server.engine.Run(ctx, engine.Request{
		SessionID:   c.sessionID,
		ProjectRoot: c.server.root,
		Input:       input,
		AgentMD:     c.server.agentMD,
		Context:     c.server.runtimeContext(),
		PlanEnabled: msg.PlanEnabled,
	})
	if err != nil {
		c.send(serverMessage{Type: "operation.error", ID: msg.ID, Error: err.Error()})
		return
	}
	for event := range events {
		switch event.Type {
		case engine.EventStatus:
			c.send(serverMessage{Type: "chat.status", ID: msg.ID, Content: event.Content})
		case engine.EventToken:
			c.send(serverMessage{Type: "chat.token", ID: msg.ID, Content: event.Content})
		case engine.EventOutput:
			c.send(serverMessage{Type: "chat.output", ID: msg.ID, Content: event.Content})
		case engine.EventStats:
			c.send(serverMessage{Type: "chat.stats", ID: msg.ID, Content: event.Content})
		case engine.EventError:
			c.send(serverMessage{Type: "operation.error", ID: msg.ID, Error: event.Content})
		}
	}
}

func (c *wsClient) runTerminal(ctx context.Context, msg clientMessage) {
	timeout := time.Duration(msg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	result, err := c.executor.Execute(ctx, cftools.Operation{
		Kind:        permission.OperationShell,
		ProjectRoot: c.server.root,
		Command:     msg.Command,
		Timeout:     timeout,
	}, c.sessionID)
	if result.Output != "" {
		c.send(serverMessage{Type: "terminal.output", ID: msg.ID, Output: result.Output})
	}
	c.sendOperationResult(msg.ID, result, err)
}

func (c *wsClient) previewFile(msg clientMessage) {
	preview, err := filePreview(c.server.root, msg.Path, msg.Content, msg.Append)
	if err != nil {
		c.send(serverMessage{Type: "operation.error", ID: msg.ID, Error: err.Error()})
		return
	}
	c.send(serverMessage{Type: "file.diff", ID: msg.ID, Path: msg.Path, Preview: preview})
}

func (c *wsClient) writeFile(ctx context.Context, msg clientMessage) {
	result, err := c.executor.Execute(ctx, cftools.Operation{
		Kind:        permission.OperationWriteFile,
		ProjectRoot: c.server.root,
		Path:        msg.Path,
		Content:     msg.Content,
		Append:      msg.Append,
	}, c.sessionID)
	c.sendOperationResult(msg.ID, result, err)
}

func (c *wsClient) switchSession(sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		c.send(serverMessage{Type: "operation.error", Error: "session_id is required"})
		return
	}
	if c.server.store != nil {
		if session, err := c.server.store.Switch(c.server.root, sessionID); err != nil {
			c.send(serverMessage{Type: "operation.error", Error: err.Error()})
			return
		} else {
			sessionID = session.ID
		}
	}
	c.sessionID = sessionID
	c.send(serverMessage{Type: "session.updated", SessionID: sessionID})
}

func (c *wsClient) confirm(ctx context.Context, op permission.Operation) (permission.Decision, error) {
	ch := make(chan permission.Decision, 1)
	c.pendingMu.Lock()
	c.pending[op.ID] = ch
	c.pendingMu.Unlock()
	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, op.ID)
		c.pendingMu.Unlock()
	}()
	c.send(serverMessage{
		Type:        "permission.required",
		OperationID: op.ID,
		Kind:        string(op.Kind),
		Path:        op.Path,
		Command:     op.Command,
		Preview:     op.Preview,
		Risk:        op.Risk,
		Timeout:     op.Timeout,
	})
	select {
	case <-ctx.Done():
		return permission.Decision{Allowed: false, Reason: "cancelled"}, ctx.Err()
	case decision := <-ch:
		return decision, nil
	}
}

func (c *wsClient) resolvePermission(operationID string, allowed bool) {
	c.pendingMu.Lock()
	ch := c.pending[operationID]
	c.pendingMu.Unlock()
	if ch == nil {
		c.send(serverMessage{Type: "operation.error", OperationID: operationID, Error: "permission request not found"})
		return
	}
	reason := "web user denied"
	if allowed {
		reason = "web user approved"
	}
	ch <- permission.Decision{Allowed: allowed, Reason: reason}
}

func (c *wsClient) rejectAll(reason string) {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	for id, ch := range c.pending {
		ch <- permission.Decision{Allowed: false, Reason: reason}
		delete(c.pending, id)
	}
}

func (c *wsClient) sendOperationResult(id string, result cftools.Result, err error) {
	confirmed := result.Confirmed
	if err != nil {
		c.send(serverMessage{
			Type:           "operation.error",
			ID:             id,
			Error:          err.Error(),
			Output:         result.Output,
			DurationMillis: result.Duration.Milliseconds(),
			Confirmed:      &confirmed,
		})
		return
	}
	c.send(serverMessage{
		Type:           "operation.done",
		ID:             id,
		Output:         result.Output,
		DurationMillis: result.Duration.Milliseconds(),
		Confirmed:      &confirmed,
	})
}

func (c *wsClient) send(msg serverMessage) {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	_ = c.conn.WriteJSON(msg)
}

func filePreview(projectRoot, relPath, newContent string, appendMode bool) (string, error) {
	target, err := permission.ValidateProjectPath(projectRoot, relPath)
	if err != nil {
		return "", err
	}
	old := ""
	if data, err := readFile(target); err == nil {
		old = data
	}
	if appendMode {
		newContent = old + newContent
	}
	return simpleDiff(old, newContent), nil
}

func readFile(path string) (string, error) {
	data, err := osReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

var osReadFile = func(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func simpleDiff(old, next string) string {
	oldLines := strings.Split(old, "\n")
	nextLines := strings.Split(next, "\n")
	var b strings.Builder
	b.WriteString("--- before\n+++ after\n")
	max := len(oldLines)
	if len(nextLines) > max {
		max = len(nextLines)
	}
	for i := 0; i < max; i++ {
		var before, after string
		if i < len(oldLines) {
			before = oldLines[i]
		}
		if i < len(nextLines) {
			after = nextLines[i]
		}
		if before == after {
			continue
		}
		if i < len(oldLines) {
			b.WriteString("-" + before + "\n")
		}
		if i < len(nextLines) {
			b.WriteString("+" + after + "\n")
		}
	}
	return b.String()
}

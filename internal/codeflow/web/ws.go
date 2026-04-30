package web

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/hertz-contrib/websocket"

	"github.com/viko0313/CodeFlow/internal/codeflow/engine"
	"github.com/viko0313/CodeFlow/internal/codeflow/observability"
	"github.com/viko0313/CodeFlow/internal/codeflow/permission"
	"github.com/viko0313/CodeFlow/internal/codeflow/storage"
	cftools "github.com/viko0313/CodeFlow/internal/codeflow/tools"
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
	ApprovalID     string `json:"approval_id,omitempty"`
	Allowed        bool   `json:"allowed,omitempty"`
	Reason         string `json:"reason,omitempty"`
	RequestID      string `json:"request_id,omitempty"`
	PlanEnabled    bool   `json:"plan_enabled,omitempty"`
}

type serverMessage struct {
	Type           string `json:"type"`
	ID             string `json:"id,omitempty"`
	RequestID      string `json:"request_id,omitempty"`
	SessionID      string `json:"session_id,omitempty"`
	OperationID    string `json:"operation_id,omitempty"`
	ApprovalID     string `json:"approval_id,omitempty"`
	Status         string `json:"status,omitempty"`
	Kind           string `json:"kind,omitempty"`
	Path           string `json:"path,omitempty"`
	Command        string `json:"command,omitempty"`
	Preview        string `json:"preview,omitempty"`
	Risk           string `json:"risk,omitempty"`
	Timeout        string `json:"timeout,omitempty"`
	Content        string `json:"content,omitempty"`
	Output         string `json:"output,omitempty"`
	Error          string `json:"error,omitempty"`
	Reason         string `json:"reason,omitempty"`
	DurationMillis int64  `json:"duration_ms,omitempty"`
	Confirmed      *bool  `json:"confirmed,omitempty"`
}

type wsClient struct {
	server    *Server
	conn      *websocket.Conn
	requestID string
	sessionID string
	sendMu    sync.Mutex
	executor  *cftools.Executor
}

var upgrader = websocket.HertzUpgrader{
	CheckOrigin: func(ctx *app.RequestContext) bool {
		return true
	},
}

func (s *Server) handleWS(ctx context.Context, c *app.RequestContext) {
	sessionID := strings.TrimSpace(c.Query("session_id"))
	requestID := strings.TrimSpace(c.Query("request_id"))
	if requestID == "" {
		requestID = observability.RequestIDFromHertz(c)
	}
	if requestID == "" {
		requestID = fmt.Sprintf("ws_%d", time.Now().UTC().UnixNano())
	}
	_ = upgrader.Upgrade(c, func(conn *websocket.Conn) {
		if sessionID == "" && s.store != nil {
			if session, err := s.store.GetActive(s.root); err == nil && session != nil {
				sessionID = session.ID
			}
		}
		client := &wsClient{
			server:    s,
			conn:      conn,
			requestID: requestID,
			sessionID: sessionID,
		}
		client.executor = s.newWebExecutor(client.confirm)
		client.send(serverMessage{
			Type:      "session.updated",
			SessionID: sessionID,
			RequestID: requestID,
		})
		client.readLoop(ctx)
	})
}

func (c *wsClient) readLoop(ctx context.Context) {
	defer c.conn.Close()
	for {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		msg, err := decodeClientMessage(raw)
		if err != nil {
			c.send(serverMessage{Type: "operation.error", Error: "invalid client message: " + err.Error(), RequestID: c.requestID})
			continue
		}
		c.handle(ctx, msg)
	}
}

func (c *wsClient) handle(ctx context.Context, msg clientMessage) {
	switch msg.Type {
	case "ping":
		c.send(serverMessage{Type: "pong", ID: msg.ID})
	case "permission.decide":
		c.decidePermission(msg)
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
	runCtx := observability.WithSessionID(observability.WithRequestID(ctx, c.requestID), c.sessionID)
	c.server.emitTaskEvent(storage.CreateTaskEventInput{
		SessionID: c.sessionID,
		RequestID: c.requestID,
		Source:    "chat",
		Level:     "info",
		EventType: "chat.started",
		Message:   "chat request started",
		Payload:   fmt.Sprintf(`{"input_len":%d}`, len(input)),
	})
	events, err := c.server.engine.Run(runCtx, engine.Request{
		SessionID:   c.sessionID,
		RequestID:   c.requestID,
		WorkspaceID: c.server.workspaceID,
		ProjectRoot: c.server.root,
		Input:       input,
		AgentMD:     c.server.agentMD,
		Context:     c.server.runtimeContext(),
		PlanEnabled: msg.PlanEnabled,
	})
	if err != nil {
		c.server.emitTaskEvent(storage.CreateTaskEventInput{
			SessionID: c.sessionID,
			RequestID: c.requestID,
			Source:    "chat",
			Level:     "error",
			EventType: "chat.failed",
			Message:   "chat execution failed",
			Payload:   fmt.Sprintf(`{"error":%q}`, err.Error()),
		})
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
		case engine.EventIteration:
			c.send(serverMessage{Type: "llm.iteration.started", ID: msg.ID, Content: event.Content, Status: "running"})
		case engine.EventToolStarted:
			c.send(serverMessage{Type: "tool.call.started", ID: msg.ID, Kind: event.ToolName, Content: event.Content, Status: "running"})
		case engine.EventToolCompleted:
			c.send(serverMessage{Type: "tool.call.completed", ID: msg.ID, Kind: event.ToolName, Content: event.Content, Status: "ok", DurationMillis: event.DurationMillis})
		case engine.EventToolFailed:
			c.send(serverMessage{Type: "tool.call.failed", ID: msg.ID, Kind: event.ToolName, Content: event.Content, Status: "error", DurationMillis: event.DurationMillis})
		}
	}
	c.server.emitTaskEvent(storage.CreateTaskEventInput{
		SessionID: c.sessionID,
		RequestID: c.requestID,
		Source:    "chat",
		Level:     "info",
		EventType: "chat.completed",
		Message:   "chat request completed",
		Payload:   fmt.Sprintf(`{"session_id":%q}`, observability.SessionIDFromContext(runCtx)),
	})
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
		RequestID:   c.requestID,
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
		RequestID:   c.requestID,
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
	if c.server.approvals == nil || !c.server.approvals.Enabled() {
		return permission.Decision{Allowed: false, Reason: "approval service unavailable"}, nil
	}
	if strings.TrimSpace(op.ApprovalID) == "" {
		return permission.Decision{Allowed: false, Reason: "approval record missing"}, nil
	}
	c.send(serverMessage{
		Type:        "permission.required",
		RequestID:   c.requestID,
		OperationID: op.ID,
		ApprovalID:  op.ApprovalID,
		Status:      string(storage.ApprovalStatusPending),
		Kind:        string(op.Kind),
		Path:        op.Path,
		Command:     op.Command,
		Preview:     op.Preview,
		Risk:        op.Risk,
		Timeout:     op.Timeout,
	})
	c.server.emitTaskEvent(storage.CreateTaskEventInput{
		SessionID:   c.sessionID,
		RequestID:   c.requestID,
		OperationID: op.ID,
		ApprovalID:  op.ApprovalID,
		Source:      "ws",
		Level:       "info",
		EventType:   "approval.requested",
		Message:     "permission request sent to websocket client",
		Payload:     fmt.Sprintf(`{"kind":%q}`, op.Kind),
	})
	allowed, reason, err := c.server.approvals.WaitForDecision(ctx, op.ApprovalID)
	if err != nil {
		return permission.Decision{Allowed: false, Reason: "approval wait failed"}, err
	}
	status := storage.ApprovalStatusRejected
	if allowed {
		status = storage.ApprovalStatusApproved
	}
	c.send(serverMessage{
		Type:        "approval.updated",
		RequestID:   c.requestID,
		OperationID: op.ID,
		ApprovalID:  op.ApprovalID,
		Status:      string(status),
		Reason:      reason,
	})
	return permission.Decision{Allowed: allowed, Reason: reason}, nil
}

func (c *wsClient) decidePermission(msg clientMessage) {
	if c.server.approvals == nil || !c.server.approvals.Enabled() {
		c.send(serverMessage{Type: "operation.error", Error: "approval service unavailable", RequestID: c.requestID})
		return
	}
	reason := strings.TrimSpace(msg.Reason)
	approvalID := strings.TrimSpace(msg.ApprovalID)
	if approvalID == "" && strings.TrimSpace(msg.OperationID) != "" {
		store := c.server.approvals.Store()
		if store != nil {
			record, err := store.GetApprovalByOperationID(strings.TrimSpace(msg.OperationID))
			if err == nil && record != nil {
				approvalID = record.ID
			}
		}
	}
	if approvalID == "" {
		c.send(serverMessage{Type: "operation.error", Error: "approval_id is required", RequestID: c.requestID})
		return
	}
	if !msg.Allowed && reason == "" {
		c.send(serverMessage{
			Type:       "operation.error",
			ApprovalID: approvalID,
			Error:      "reject reason is required",
			RequestID:  c.requestID,
		})
		return
	}
	record, err := c.server.approvals.Decide(approvalID, msg.Allowed, reason)
	if err != nil {
		c.send(serverMessage{
			Type:       "operation.error",
			ApprovalID: approvalID,
			Error:      err.Error(),
			RequestID:  c.requestID,
		})
		return
	}
	c.server.emitTaskEvent(storage.CreateTaskEventInput{
		SessionID:   record.SessionID,
		RequestID:   c.requestID,
		OperationID: record.OperationID,
		ApprovalID:  record.ID,
		Source:      "ws",
		Level:       "info",
		EventType:   "approval.decided",
		Message:     "approval decided via websocket",
		Payload:     fmt.Sprintf(`{"status":%q,"reason":%q}`, record.Status, record.DecisionReason),
	})
	c.send(serverMessage{
		Type:        "approval.updated",
		RequestID:   c.requestID,
		OperationID: record.OperationID,
		ApprovalID:  record.ID,
		Status:      string(record.Status),
		Reason:      record.DecisionReason,
	})
}

func decodeClientMessage(raw []byte) (clientMessage, error) {
	var msg clientMessage
	if err := json.Unmarshal(raw, &msg); err == nil {
		return msg, nil
	}
	repaired := repairJSONPayload(string(raw))
	if repaired == "" {
		return clientMessage{}, fmt.Errorf("invalid json payload")
	}
	if err := json.Unmarshal([]byte(repaired), &msg); err != nil {
		return clientMessage{}, err
	}
	return msg, nil
}

func (c *wsClient) sendOperationResult(id string, result cftools.Result, err error) {
	confirmed := result.Confirmed
	if err != nil {
		c.send(serverMessage{
			Type:           "operation.error",
			ID:             id,
			RequestID:      c.requestID,
			ApprovalID:     result.ApprovalID,
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
		RequestID:      c.requestID,
		ApprovalID:     result.ApprovalID,
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

func repairJSONPayload(payload string) string {
	value := strings.TrimSpace(payload)
	if value == "" {
		return ""
	}
	value = strings.TrimPrefix(value, "```json")
	value = strings.TrimPrefix(value, "```")
	value = strings.TrimSuffix(value, "```")
	value = strings.TrimSpace(value)
	start := strings.IndexByte(value, '{')
	end := strings.LastIndexByte(value, '}')
	if start >= 0 && end >= start {
		value = value[start : end+1]
	}
	value = strings.TrimSpace(value)
	for strings.HasSuffix(value, ",") {
		value = strings.TrimSuffix(value, ",")
		value = strings.TrimSpace(value)
	}
	return value
}

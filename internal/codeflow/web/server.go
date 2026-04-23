package web

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"github.com/viko0313/CodeFlow/internal/codeflow/approval"
	"github.com/viko0313/CodeFlow/internal/codeflow/audit"
	cfconfig "github.com/viko0313/CodeFlow/internal/codeflow/config"
	"github.com/viko0313/CodeFlow/internal/codeflow/documents"
	"github.com/viko0313/CodeFlow/internal/codeflow/engine"
	"github.com/viko0313/CodeFlow/internal/codeflow/mcp"
	cfmemory "github.com/viko0313/CodeFlow/internal/codeflow/memory"
	"github.com/viko0313/CodeFlow/internal/codeflow/observability"
	"github.com/viko0313/CodeFlow/internal/codeflow/permission"
	cfsession "github.com/viko0313/CodeFlow/internal/codeflow/session"
	"github.com/viko0313/CodeFlow/internal/codeflow/skills"
	"github.com/viko0313/CodeFlow/internal/codeflow/storage"
	cftools "github.com/viko0313/CodeFlow/internal/codeflow/tools"
	"github.com/viko0313/CodeFlow/internal/codeflow/version"
)

type Options struct {
	Addr        string
	ProjectRoot string
}

type Server struct {
	root    string
	cfg     *cfconfig.Config
	store   cfsession.Store
	memory  cfmemory.ShortTermMemory
	engine  engine.Engine
	auditor *audit.Logger
	agentMD string
	dataDir string
	skills  skills.Manifest
	mcp     mcp.Manifest
	docs    *documents.Store
	uploads []documents.UploadedDocument

	logger         *slog.Logger
	approvals      *approval.Service
	taskEvents     storage.TaskEventStore
	storageBackend string
	memoryBackend  string
	fallbackActive bool
}

type Dependencies struct {
	ProjectRoot string
	Config      *cfconfig.Config
	Store       cfsession.Store
	Memory      cfmemory.ShortTermMemory
	Engine      engine.Engine
	Auditor     *audit.Logger
	AgentMD     string
	DataDir     string
	Logger      *slog.Logger

	StorageBackend string
	MemoryBackend  string
	FallbackActive bool
}

func Run(ctx context.Context, opts Options) error {
	root := projectRoot(opts.ProjectRoot)
	if err := cfconfig.EnsureProjectConfig(root); err != nil {
		return err
	}
	cfg, err := cfconfig.Load(root)
	if err != nil {
		return err
	}
	logger := observability.NewLogger("codeflow-web")
	store, storageBackend, storageFallback, err := storage.OpenSessionStoreWithFallback(ctx, cfg.Storage.PostgresDSN, cfg.DataDir)
	if err != nil {
		return err
	}
	defer store.Close()
	memory, memoryBackend, memoryFallback, err := cfmemory.OpenShortTermMemoryWithFallback(ctx, cfg.Storage.RedisAddr, cfg.Storage.RedisPass, cfg.Storage.RedisDB)
	if err != nil {
		return err
	}
	defer memory.Close()
	fallbackActive := storageFallback || memoryFallback
	if fallbackActive {
		logger.WarnContext(ctx, "runtime fallback activated",
			slog.String("component", "runtime"),
			slog.String("event", "runtime.fallback"),
			slog.String("storage_backend", storageBackend),
			slog.String("memory_backend", memoryBackend),
		)
	}
	agentMD := readAgentMD(root)
	session, err := store.GetActive(root)
	if err != nil {
		return err
	}
	if session == nil {
		if _, err := store.Create(root, filepath.Base(root), agentMD); err != nil {
			return err
		}
	}
	auditor, err := audit.NewLogger(cfg.DataDir)
	if err != nil {
		return err
	}
	llm, err := engine.New(ctx, cfg, memory)
	if err != nil {
		return err
	}
	addr := strings.TrimSpace(opts.Addr)
	if addr == "" {
		addr = "localhost:8742"
	}
	s := NewServer(Dependencies{
		ProjectRoot: root,
		Config:      cfg,
		Store:       store,
		Memory:      memory,
		Engine:      llm,
		Auditor:     auditor,
		AgentMD:     agentMD,
		DataDir:     cfg.DataDir,
		Logger:      logger,

		StorageBackend: storageBackend,
		MemoryBackend:  memoryBackend,
		FallbackActive: fallbackActive,
	})
	s.logger.InfoContext(ctx, "web server configured",
		slog.String("component", "runtime"),
		slog.String("event", "server.starting"),
		slog.String("storage_backend", storageBackend),
		slog.String("memory_backend", memoryBackend),
		slog.Bool("fallback_active", fallbackActive),
	)
	h := s.Hertz(addr)
	h.SetCustomSignalWaiter(func(errCh chan error) error {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errCh:
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}
	})
	fmt.Printf("%s web server listening on http://%s\n", version.ProductName, addr)
	h.Spin()
	return nil
}

func NewServer(deps Dependencies) *Server {
	cfg := deps.Config
	if cfg == nil {
		cfg = &cfconfig.Config{}
	}
	dataDir := deps.DataDir
	if dataDir == "" && cfg.DataDir != "" {
		dataDir = cfg.DataDir
	}
	logger := deps.Logger
	if logger == nil {
		logger = observability.NewLogger("codeflow-web")
	}
	skillManifest, _ := skills.Load(cfg.Skills)
	mcpManifest, _ := mcp.Load(cfg.MCP)
	var approvalStore storage.ApprovalStore
	var eventStore storage.TaskEventStore
	if deps.Store != nil {
		if candidate, ok := deps.Store.(storage.ApprovalStore); ok {
			approvalStore = candidate
		}
		if candidate, ok := deps.Store.(storage.TaskEventStore); ok {
			eventStore = candidate
		}
	}
	return &Server{
		root:    projectRoot(deps.ProjectRoot),
		cfg:     cfg,
		store:   deps.Store,
		memory:  deps.Memory,
		engine:  deps.Engine,
		auditor: deps.Auditor,
		agentMD: deps.AgentMD,
		dataDir: dataDir,
		skills:  skillManifest,
		mcp:     mcpManifest,
		docs:    documents.NewStore(cfg.Documents),
		logger:  logger,

		approvals:      approval.NewService(approvalStore),
		taskEvents:     eventStore,
		storageBackend: strings.TrimSpace(deps.StorageBackend),
		memoryBackend:  strings.TrimSpace(deps.MemoryBackend),
		fallbackActive: deps.FallbackActive,
	}
}

func (s *Server) Hertz(addr string) *server.Hertz {
	h := server.Default(
		server.WithHostPorts(addr),
		server.WithDisablePrintRoute(true),
		server.WithExitWaitTime(2*time.Second),
	)
	s.Routes(h)
	return h
}

func (s *Server) Routes(h *server.Hertz) {
	h.Use(func(ctx context.Context, c *app.RequestContext) {
		c.Response.Header.Set("Access-Control-Allow-Origin", "http://localhost:3000")
		c.Response.Header.Set("Access-Control-Allow-Credentials", "true")
		c.Response.Header.Set("Access-Control-Allow-Headers", "content-type,x-request-id")
		c.Response.Header.Set("Access-Control-Allow-Methods", "GET,POST,DELETE,OPTIONS")
		if string(c.Method()) == consts.MethodOptions {
			c.AbortWithStatus(consts.StatusNoContent)
			return
		}
		c.Next(ctx)
	})
	h.Use(observability.RequestContextMiddleware(s.logger))
	h.Use(func(ctx context.Context, c *app.RequestContext) {
		start := time.Now()
		c.Next(ctx)
		if s.taskEvents == nil {
			return
		}
		level := "info"
		if c.Response.StatusCode() >= consts.StatusBadRequest {
			level = "error"
		}
		s.emitTaskEvent(storage.CreateTaskEventInput{
			RequestID: observability.RequestIDFromHertz(c),
			Source:    "http",
			Level:     level,
			EventType: "http.request.completed",
			Message:   fmt.Sprintf("%s %s", string(c.Method()), c.FullPath()),
			Payload: fmt.Sprintf(`{"path":%q,"status":%d,"latency_ms":%d}`,
				string(c.Path()),
				c.Response.StatusCode(),
				time.Since(start).Milliseconds(),
			),
		})
	})
	h.OPTIONS("/*path", func(ctx context.Context, c *app.RequestContext) {
		c.AbortWithStatus(consts.StatusNoContent)
	})
	h.GET("/api/health", s.handleHealth)
	h.GET("/api/config", s.handleConfig)
	h.GET("/api/skills", s.handleSkills)
	h.GET("/api/mcp", s.handleMCP)
	h.GET("/api/sessions", s.handleSessions)
	h.POST("/api/sessions", s.handleCreateSession)
	h.GET("/api/sessions/:id/history", s.handleSessionHistory)
	h.POST("/api/sessions/:id/switch", s.handleSwitchSession)
	h.DELETE("/api/sessions/:id", s.handleDeleteSession)
	h.GET("/api/approvals", s.handleApprovals)
	h.GET("/api/approvals/:id", s.handleApprovalByID)
	h.POST("/api/approvals/:id/approve", s.handleApproveApproval)
	h.POST("/api/approvals/:id/reject", s.handleRejectApproval)
	h.GET("/api/task-events", s.handleTaskEvents)
	h.GET("/api/audit/recent", s.handleRecentAudit)
	h.POST("/api/documents/upload", s.handleDocumentUpload)
	h.GET("/api/ws", s.handleWS)
	h.NoRoute(func(ctx context.Context, c *app.RequestContext) {
		writeError(c, consts.StatusNotFound, "not found")
	})
}

func (s *Server) handleHealth(ctx context.Context, c *app.RequestContext) {
	activeID := ""
	if s.store != nil {
		if session, err := s.store.GetActive(s.root); err == nil && session != nil {
			activeID = session.ID
		}
	}
	writeJSON(c, consts.StatusOK, map[string]any{
		"status":              "ok",
		"product":             version.ProductName,
		"version":             version.Version,
		"project_root":        s.root,
		"active_session_id":   activeID,
		"postgres_configured": strings.TrimSpace(s.cfg.Storage.PostgresDSN) != "",
		"redis_configured":    strings.TrimSpace(s.cfg.Storage.RedisAddr) != "",
		"model_configured":    strings.TrimSpace(s.cfg.Model) != "",
		"storage_backend":     defaultBackend(s.storageBackend, storage.BackendPostgres),
		"memory_backend":      defaultBackend(s.memoryBackend, cfmemory.BackendRedis),
		"fallback_active":     s.fallbackActive,
	})
}

func (s *Server) handleConfig(ctx context.Context, c *app.RequestContext) {
	writeJSON(c, consts.StatusOK, map[string]any{
		"provider":     s.cfg.Provider,
		"model":        s.cfg.Model,
		"base_url":     s.cfg.BaseURL,
		"project_root": s.root,
		"data_dir":     s.cfg.DataDir,
		"storage": map[string]any{
			"postgres_configured": strings.TrimSpace(s.cfg.Storage.PostgresDSN) != "",
			"redis_addr":          s.cfg.Storage.RedisAddr,
			"redis_db":            s.cfg.Storage.RedisDB,
		},
		"runtime": s.cfg.Runtime,
		"permissions": map[string]any{
			"trusted_commands": s.cfg.Permissions.TrustedCommands,
			"trusted_dirs":     s.cfg.Permissions.TrustedDirs,
			"writable_dirs":    s.cfg.Permissions.WritableDirs,
			"force_approval":   s.cfg.Permissions.ForceApproval,
		},
		"agent":     s.cfg.Agent,
		"skills":    s.cfg.Skills,
		"mcp":       s.cfg.MCP,
		"documents": s.cfg.Documents,
	})
}

func (s *Server) handleSkills(ctx context.Context, c *app.RequestContext) {
	manifest, err := skills.Load(s.cfg.Skills)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	s.skills = manifest
	writeJSON(c, consts.StatusOK, manifest)
}

func (s *Server) handleMCP(ctx context.Context, c *app.RequestContext) {
	manifest, err := mcp.Load(s.cfg.MCP)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	s.mcp = manifest
	writeJSON(c, consts.StatusOK, manifest)
}

func (s *Server) handleSessions(ctx context.Context, c *app.RequestContext) {
	if s.store == nil {
		writeError(c, consts.StatusServiceUnavailable, "session store unavailable")
		return
	}
	items, err := s.store.List(s.root)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"sessions": items})
}

func (s *Server) handleCreateSession(ctx context.Context, c *app.RequestContext) {
	if s.store == nil {
		writeError(c, consts.StatusServiceUnavailable, "session store unavailable")
		return
	}
	var req struct {
		Title string `json:"title"`
	}
	_ = c.BindJSON(&req)
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = filepath.Base(s.root)
	}
	session, err := s.store.Create(s.root, title, s.agentMD)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusCreated, session)
}

func (s *Server) handleSwitchSession(ctx context.Context, c *app.RequestContext) {
	if s.store == nil {
		writeError(c, consts.StatusServiceUnavailable, "session store unavailable")
		return
	}
	session, err := s.store.Switch(s.root, c.Param("id"))
	if err != nil {
		writeError(c, consts.StatusNotFound, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, session)
}

func (s *Server) handleSessionHistory(ctx context.Context, c *app.RequestContext) {
	sessionID := strings.TrimSpace(c.Param("id"))
	if sessionID == "" {
		writeError(c, consts.StatusBadRequest, "session id is required")
		return
	}
	if s.memory == nil {
		writeJSON(c, consts.StatusOK, map[string]any{
			"session_id": sessionID,
			"turns":      []cfmemory.Turn{},
		})
		return
	}
	turns, err := s.memory.GetRecent(ctx, sessionID, int64(parseLimit(c.Query("limit"), 20, 20)))
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{
		"session_id": sessionID,
		"turns":      turns,
	})
}

func (s *Server) handleDeleteSession(ctx context.Context, c *app.RequestContext) {
	if s.store == nil {
		writeError(c, consts.StatusServiceUnavailable, "session store unavailable")
		return
	}
	sessionID := c.Param("id")
	if err := s.store.Delete(s.root, sessionID); err != nil {
		writeError(c, consts.StatusNotFound, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"deleted": sessionID})
}

func (s *Server) handleRecentAudit(ctx context.Context, c *app.RequestContext) {
	limit := parseLimit(c.Query("limit"), 20, 200)
	if s.taskEvents != nil {
		events, err := s.taskEvents.ListTaskEvents(storage.ListTaskEventsOptions{
			SessionID: strings.TrimSpace(c.Query("session_id")),
			Limit:     limit,
		})
		if err == nil {
			out := make([]audit.Event, 0, len(events))
			for _, event := range events {
				converted := taskEventToAudit(event)
				out = append(out, converted)
			}
			writeJSON(c, consts.StatusOK, map[string]any{"events": out})
			return
		}
	}
	events, err := readRecentAudit(s.dataDir, limit)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"events": events})
}

func (s *Server) handleApprovals(ctx context.Context, c *app.RequestContext) {
	if s.approvals == nil || !s.approvals.Enabled() {
		writeError(c, consts.StatusServiceUnavailable, "approval service unavailable")
		return
	}
	status := strings.TrimSpace(c.Query("status"))
	limit := parseLimit(c.Query("limit"), 100, 500)
	items, err := s.approvals.List(status, limit)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"approvals": items})
}

func (s *Server) handleApprovalByID(ctx context.Context, c *app.RequestContext) {
	if s.approvals == nil || !s.approvals.Enabled() {
		writeError(c, consts.StatusServiceUnavailable, "approval service unavailable")
		return
	}
	record, err := s.approvals.Get(c.Param("id"))
	if err != nil {
		if errors.Is(err, approval.ErrApprovalNotFound) {
			writeError(c, consts.StatusNotFound, err.Error())
			return
		}
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"approval": record})
}

func (s *Server) handleApproveApproval(ctx context.Context, c *app.RequestContext) {
	s.decideApproval(ctx, c, true)
}

func (s *Server) handleRejectApproval(ctx context.Context, c *app.RequestContext) {
	s.decideApproval(ctx, c, false)
}

func (s *Server) decideApproval(ctx context.Context, c *app.RequestContext, allowed bool) {
	if s.approvals == nil || !s.approvals.Enabled() {
		writeError(c, consts.StatusServiceUnavailable, "approval service unavailable")
		return
	}
	reason := ""
	if !allowed {
		var req struct {
			Reason string `json:"reason"`
		}
		if err := c.BindJSON(&req); err != nil {
			writeError(c, consts.StatusBadRequest, "reason is required")
			return
		}
		reason = strings.TrimSpace(req.Reason)
	}
	record, err := s.approvals.Decide(c.Param("id"), allowed, reason)
	if err != nil {
		switch {
		case errors.Is(err, approval.ErrApprovalNotFound):
			writeError(c, consts.StatusNotFound, err.Error())
		case errors.Is(err, approval.ErrRejectReasonRequired):
			writeError(c, consts.StatusBadRequest, err.Error())
		case errors.Is(err, approval.ErrApprovalAlreadyDecided):
			writeError(c, consts.StatusConflict, err.Error())
		default:
			writeError(c, consts.StatusInternalServerError, err.Error())
		}
		return
	}
	s.emitTaskEvent(storage.CreateTaskEventInput{
		SessionID:   record.SessionID,
		RequestID:   observability.RequestIDFromHertz(c),
		OperationID: record.OperationID,
		ApprovalID:  record.ID,
		Source:      "api",
		Level:       "info",
		EventType:   "approval.decided",
		Message:     "approval decision recorded via rest",
		Payload:     fmt.Sprintf(`{"status":%q,"reason":%q}`, record.Status, record.DecisionReason),
	})
	writeJSON(c, consts.StatusOK, map[string]any{"approval": record})
}

func (s *Server) handleTaskEvents(ctx context.Context, c *app.RequestContext) {
	if s.taskEvents == nil {
		writeError(c, consts.StatusServiceUnavailable, "task event store unavailable")
		return
	}
	items, err := s.taskEvents.ListTaskEvents(storage.ListTaskEventsOptions{
		SessionID: strings.TrimSpace(c.Query("session_id")),
		Limit:     parseLimit(c.Query("limit"), 200, 1000),
	})
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"events": items})
}

func (s *Server) handleDocumentUpload(ctx context.Context, c *app.RequestContext) {
	if s.docs == nil {
		writeError(c, consts.StatusServiceUnavailable, "document store unavailable")
		return
	}
	header, err := c.FormFile("file")
	if err != nil {
		writeError(c, consts.StatusBadRequest, "file field is required")
		return
	}
	doc, err := s.docs.Save(ctx, header)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	s.uploads = append([]documents.UploadedDocument{*doc}, s.uploads...)
	if len(s.uploads) > 20 {
		s.uploads = s.uploads[:20]
	}
	writeJSON(c, consts.StatusCreated, doc)
}

func (s *Server) newWebExecutor(confirmer permission.Confirmer) *cftools.Executor {
	gate := permission.NewGate(permission.Options{
		TrustedCommands: s.cfg.Permissions.TrustedCommands,
		TrustedDirs:     s.cfg.Permissions.TrustedDirs,
		WritableDirs:    s.cfg.Permissions.WritableDirs,
		ForceApproval:   s.cfg.Permissions.ForceApproval,
		Confirmer:       confirmer,
	})
	var approvalStore storage.ApprovalStore
	if s.approvals != nil {
		approvalStore = s.approvals.Store()
	}
	return cftools.NewExecutor(gate, s.auditor, approvalStore, s.taskEvents)
}

func (s *Server) runtimeContext() string {
	var parts []string
	if text := skills.PreloadText(s.skills); text != "" {
		parts = append(parts, "[Preloaded skills]\n"+text)
	}
	if text := mcp.PreloadText(s.mcp); text != "" {
		parts = append(parts, "[Preloaded MCP servers]\n"+text)
	}
	if len(s.uploads) > 0 {
		var b strings.Builder
		for _, doc := range s.uploads {
			b.WriteString("## ")
			b.WriteString(doc.FileName)
			b.WriteString("\n")
			b.WriteString(strings.TrimSpace(doc.Content))
			b.WriteString("\n\n")
		}
		parts = append(parts, "[Uploaded documents]\n"+strings.TrimSpace(b.String()))
	}
	return strings.Join(parts, "\n\n")
}

func readRecentAudit(dataDir string, limit int) ([]audit.Event, error) {
	if strings.TrimSpace(dataDir) == "" {
		return []audit.Event{}, nil
	}
	logDir := filepath.Join(dataDir, "logs")
	entries, err := os.ReadDir(logDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []audit.Event{}, nil
		}
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "audit_") && strings.HasSuffix(entry.Name(), ".jsonl") {
			files = append(files, filepath.Join(logDir, entry.Name()))
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	var events []audit.Event
	for _, file := range files {
		if len(events) >= limit {
			break
		}
		items, err := readAuditFile(file)
		if err != nil {
			return nil, err
		}
		for i := len(items) - 1; i >= 0 && len(events) < limit; i-- {
			events = append(events, items[i])
		}
	}
	return events, nil
}

func readAuditFile(path string) ([]audit.Event, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var events []audit.Event
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var event audit.Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err == nil {
			events = append(events, event)
		}
	}
	return events, scanner.Err()
}

func writeJSON(c *app.RequestContext, status int, payload any) {
	c.JSON(status, payload)
}

func writeError(c *app.RequestContext, status int, message string) {
	writeJSON(c, status, map[string]string{"error": message})
}

func (s *Server) emitTaskEvent(input storage.CreateTaskEventInput) {
	if s.taskEvents == nil {
		return
	}
	_, _ = s.taskEvents.CreateTaskEvent(input)
}

func parseLimit(raw string, fallback, max int) int {
	limit := fallback
	if value := strings.TrimSpace(raw); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 && parsed <= max {
			limit = parsed
		}
	}
	return limit
}

func taskEventToAudit(event storage.TaskEvent) audit.Event {
	confirmed := true
	switch event.EventType {
	case "approval.failed", "execution.failed":
		confirmed = false
	}
	if strings.Contains(strings.ToLower(event.Payload), `"allowed":false`) {
		confirmed = false
	}
	return audit.Event{
		Time:          event.CreatedAt.Format(time.RFC3339),
		SessionID:     event.SessionID,
		ProjectRoot:   "",
		OperationID:   event.OperationID,
		Event:         event.EventType,
		ToolName:      event.Source,
		ArgsSummary:   event.Message,
		ResultSummary: truncateForAudit(event.Payload),
		Confirmed:     &confirmed,
	}
}

func truncateForAudit(payload string) string {
	payload = strings.TrimSpace(payload)
	if len(payload) <= 200 {
		return payload
	}
	return payload[:200] + "..."
}

func defaultBackend(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func projectRoot(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		if wd, err := os.Getwd(); err == nil {
			root = wd
		} else {
			root = "."
		}
	}
	if abs, err := filepath.Abs(root); err == nil {
		return abs
	}
	return root
}

func readAgentMD(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "AGENT.md"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

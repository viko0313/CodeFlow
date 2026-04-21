package web

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"github.com/cloudwego/codeflow/internal/codeflow/audit"
	cfconfig "github.com/cloudwego/codeflow/internal/codeflow/config"
	"github.com/cloudwego/codeflow/internal/codeflow/documents"
	"github.com/cloudwego/codeflow/internal/codeflow/engine"
	"github.com/cloudwego/codeflow/internal/codeflow/mcp"
	cfmemory "github.com/cloudwego/codeflow/internal/codeflow/memory"
	"github.com/cloudwego/codeflow/internal/codeflow/permission"
	cfsession "github.com/cloudwego/codeflow/internal/codeflow/session"
	"github.com/cloudwego/codeflow/internal/codeflow/skills"
	"github.com/cloudwego/codeflow/internal/codeflow/storage"
	cftools "github.com/cloudwego/codeflow/internal/codeflow/tools"
	"github.com/cloudwego/codeflow/internal/codeflow/version"
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
	store, err := storage.NewPostgresSessionStore(ctx, cfg.Storage.PostgresDSN)
	if err != nil {
		return err
	}
	defer store.Close()
	memory, err := cfmemory.NewRedisShortTermMemory(ctx, cfg.Storage.RedisAddr, cfg.Storage.RedisPass, cfg.Storage.RedisDB)
	if err != nil {
		return err
	}
	defer memory.Close()
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
	})
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
	skillManifest, _ := skills.Load(cfg.Skills)
	mcpManifest, _ := mcp.Load(cfg.MCP)
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
		c.Response.Header.Set("Access-Control-Allow-Headers", "content-type")
		c.Response.Header.Set("Access-Control-Allow-Methods", "GET,POST,DELETE,OPTIONS")
		if string(c.Method()) == consts.MethodOptions {
			c.AbortWithStatus(consts.StatusNoContent)
			return
		}
		c.Next(ctx)
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
	h.POST("/api/sessions/:id/switch", s.handleSwitchSession)
	h.DELETE("/api/sessions/:id", s.handleDeleteSession)
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
		"runtime":   s.cfg.Runtime,
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
	limit := 20
	if value := strings.TrimSpace(c.Query("limit")); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}
	events, err := readRecentAudit(s.dataDir, limit)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"events": events})
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
		Confirmer:       confirmer,
	})
	return cftools.NewExecutor(gate, s.auditor)
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

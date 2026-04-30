package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/viko0313/CodeFlow/internal/codeflow/audit"
	"github.com/viko0313/CodeFlow/internal/codeflow/checkpoint"
	cfconfig "github.com/viko0313/CodeFlow/internal/codeflow/config"
	"github.com/viko0313/CodeFlow/internal/codeflow/engine"
	cfmemory "github.com/viko0313/CodeFlow/internal/codeflow/memory"
	"github.com/viko0313/CodeFlow/internal/codeflow/observability"
	"github.com/viko0313/CodeFlow/internal/codeflow/permission"
	"github.com/viko0313/CodeFlow/internal/codeflow/plan"
	"github.com/viko0313/CodeFlow/internal/codeflow/run"
	cfsession "github.com/viko0313/CodeFlow/internal/codeflow/session"
	"github.com/viko0313/CodeFlow/internal/codeflow/storage"
	cftools "github.com/viko0313/CodeFlow/internal/codeflow/tools"
	"github.com/viko0313/CodeFlow/internal/codeflow/version"
	"github.com/viko0313/CodeFlow/internal/codeflow/web"
	"github.com/viko0313/CodeFlow/internal/codeflow/workspace"
)

type appOptions struct {
	projectRoot string
	once        string
	webAddr     string
}

func Execute(args []string) error {
	opts := &appOptions{}
	root := &cobra.Command{
		Use:           "codeflow",
		Short:         "CodeFlow Agent is a terminal-native enterprise coding assistant.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&opts.projectRoot, "project-root", "", "Project root; defaults to the current directory")
	root.AddCommand(startCommand(opts), webCommand(opts), sessionCommand(opts), workspaceCommand(opts), planCommand(opts), runCommand(opts), checkpointCommand(opts), memoryCommand(opts), configCommand(opts), versionCommand())
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		return err
	}
	return nil
}

func startCommand(opts *appOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start or resume a CodeFlow session in the current project",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStart(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.once, "once", "", "Run one prompt and exit; useful for smoke tests")
	_ = cmd.Flags().MarkHidden("once")
	return cmd
}

func webCommand(opts *appOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "web",
		Short: "Start the CodeFlow local Web API and WebSocket server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return web.Run(cmd.Context(), web.Options{Addr: opts.webAddr, ProjectRoot: opts.projectRoot})
		},
	}
	cmd.Flags().StringVar(&opts.webAddr, "addr", "localhost:8742", "HTTP listen address for the local Web API")
	return cmd
}

func sessionCommand(opts *appOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "session", Short: "Manage CodeFlow sessions"}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List sessions for the current project",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, cleanup, err := openSessionStore(cmd.Context(), opts.projectRoot)
			if err != nil {
				return err
			}
			defer cleanup()
			root := projectRoot(opts.projectRoot)
			items, err := store.List(root)
			if err != nil {
				return err
			}
			if len(items) == 0 {
				fmt.Println("No CodeFlow sessions.")
				return nil
			}
			for _, item := range items {
				active := " "
				if item.Active {
					active = "*"
				}
				fmt.Printf("%s %s  %s  %s\n", active, item.ID, item.UpdatedAt.Format(time.RFC3339), item.Title)
			}
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "switch <session-id>",
		Short: "Switch the active session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, cleanup, err := openSessionStore(cmd.Context(), opts.projectRoot)
			if err != nil {
				return err
			}
			defer cleanup()
			session, err := store.Switch(projectRoot(opts.projectRoot), args[0])
			if err != nil {
				return err
			}
			fmt.Printf("Active session: %s\n", session.ID)
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "delete <session-id>",
		Short: "Delete a session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, cleanup, err := openSessionStore(cmd.Context(), opts.projectRoot)
			if err != nil {
				return err
			}
			defer cleanup()
			if err := store.Delete(projectRoot(opts.projectRoot), args[0]); err != nil {
				return err
			}
			fmt.Printf("Deleted session: %s\n", args[0])
			return nil
		},
	})
	return cmd
}

func workspaceCommand(opts *appOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "workspace", Short: "Manage registered workspaces"}
	cmd.AddCommand(&cobra.Command{
		Use:   "register",
		Short: "Register the current directory or --project-root as a workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, cleanup, err := openWorkspaceStore(cmd.Context(), opts.projectRoot)
			if err != nil {
				return err
			}
			defer cleanup()
			svc := workspace.NewService(store)
			item, err := svc.EnsureRegistered(projectRoot(opts.projectRoot))
			if err != nil {
				return err
			}
			fmt.Printf("%s %s\n", item.ID, item.RootPath)
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List registered workspaces",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, cleanup, err := openWorkspaceStore(cmd.Context(), opts.projectRoot)
			if err != nil {
				return err
			}
			defer cleanup()
			items, err := workspace.NewService(store).List()
			if err != nil {
				return err
			}
			if len(items) == 0 {
				fmt.Println("No registered workspaces.")
				return nil
			}
			for _, item := range items {
				active := " "
				if item.Active {
					active = "*"
				}
				fmt.Printf("%s %s  %s  %s\n", active, item.ID, item.Name, item.RootPath)
			}
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "switch <workspace-id>",
		Short: "Switch the active workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, cleanup, err := openWorkspaceStore(cmd.Context(), opts.projectRoot)
			if err != nil {
				return err
			}
			defer cleanup()
			item, err := workspace.NewService(store).Switch(args[0])
			if err != nil {
				return err
			}
			fmt.Printf("Active workspace: %s %s\n", item.ID, item.RootPath)
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "current",
		Short: "Show the active workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, cleanup, err := openWorkspaceStore(cmd.Context(), opts.projectRoot)
			if err != nil {
				return err
			}
			defer cleanup()
			item, err := workspace.NewService(store).Current()
			if err != nil {
				return err
			}
			if item == nil {
				fmt.Println("No active workspace.")
				return nil
			}
			fmt.Printf("%s %s\n", item.ID, item.RootPath)
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "remove <workspace-id>",
		Short: "Remove a workspace registry record without deleting files",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, cleanup, err := openWorkspaceStore(cmd.Context(), opts.projectRoot)
			if err != nil {
				return err
			}
			defer cleanup()
			if err := workspace.NewService(store).Remove(args[0]); err != nil {
				return err
			}
			fmt.Printf("Removed workspace: %s\n", args[0])
			return nil
		},
	})
	return cmd
}

func configCommand(opts *appOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "Manage project CodeFlow config"}
	cmd.AddCommand(&cobra.Command{
		Use:   "get <key>",
		Short: "Get a config value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			value, err := cfconfig.Get(projectRoot(opts.projectRoot), args[0])
			if err != nil {
				return err
			}
			fmt.Println(value)
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a project config value",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cfconfig.Set(projectRoot(opts.projectRoot), args[0], args[1]); err != nil {
				return err
			}
			fmt.Printf("Updated %s\n", args[0])
			return nil
		},
	})
	return cmd
}

func versionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print CodeFlow version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("%s %s\n", version.ProductName, version.Version)
		},
	}
}

func runStart(ctx context.Context, opts *appOptions) error {
	root := projectRoot(opts.projectRoot)
	if err := cfconfig.EnsureProjectConfig(root); err != nil {
		return err
	}
	cfg, err := cfconfig.Load(root)
	if err != nil {
		return err
	}
	logger := observability.NewLogger("codeflow-cli")
	store, storageBackend, storageFallback, err := storage.OpenSessionStoreWithFallback(ctx, cfg.Storage.PostgresDSN, cfg.DataDir)
	if err != nil {
		return err
	}
	defer store.Close()
	shortMemory, memoryBackend, memoryFallback, err := cfmemory.OpenShortTermMemoryWithFallback(ctx, cfg.Storage.RedisAddr, cfg.Storage.RedisPass, cfg.Storage.RedisDB)
	if err != nil {
		return err
	}
	defer shortMemory.Close()
	if storageFallback || memoryFallback {
		logger.WarnContext(ctx, "runtime fallback activated",
			"component", "runtime",
			"event", "runtime.fallback",
			"storage_backend", storageBackend,
			"memory_backend", memoryBackend,
		)
	}
	agentMD := readAgentMD(root)
	wsStore, wsCleanup, err := openWorkspaceStore(ctx, opts.projectRoot)
	if err != nil {
		return err
	}
	defer wsCleanup()
	ws, err := workspace.NewService(wsStore).EnsureRegistered(root)
	if err != nil {
		return err
	}
	session, err := store.GetActive(root)
	if err != nil {
		return err
	}
	if session == nil {
		session, err = store.Create(root, filepath.Base(root), agentMD)
		if err != nil {
			return err
		}
	}
	auditor, err := audit.NewLogger(cfg.DataDir)
	if err != nil {
		return err
	}
	gate := permission.NewGate(permission.Options{
		TrustedCommands: cfg.Permissions.TrustedCommands,
		TrustedDirs:     cfg.Permissions.TrustedDirs,
		WritableDirs:    cfg.Permissions.WritableDirs,
		ForceApproval:   cfg.Permissions.ForceApproval,
	})
	var approvalStore storage.ApprovalStore
	var eventStore storage.TaskEventStore
	if candidate, ok := store.(storage.ApprovalStore); ok {
		approvalStore = candidate
	}
	if candidate, ok := store.(storage.TaskEventStore); ok {
		eventStore = candidate
	}
	var messageStore storage.MessageStore
	var runStore storage.RunStore
	var summaryStore storage.SummaryStore
	var checkpointStore storage.CheckpointStore
	var planStore storage.PlanStore
	if candidate, ok := store.(storage.MessageStore); ok {
		messageStore = candidate
	}
	if candidate, ok := store.(storage.RunStore); ok {
		runStore = candidate
	}
	if candidate, ok := store.(storage.SummaryStore); ok {
		summaryStore = candidate
	}
	if candidate, ok := store.(storage.CheckpointStore); ok {
		checkpointStore = candidate
	}
	if candidate, ok := store.(storage.PlanStore); ok {
		planStore = candidate
	}
	executor := cftools.NewExecutor(gate, auditor, approvalStore, eventStore)
	runRecorder := run.NewRecorder(runStore)
	executor.SetRunRecorder(runRecorder)
	executor.SetCheckpointService(checkpoint.NewService(checkpointStore, runRecorder))
	compressor := cfmemory.NewCompressor(summaryStore, runRecorder)
	planService := plan.NewService(planStore)
	llm, err := engine.New(ctx, cfg, shortMemory, engine.WithToolExecutor(executor), engine.WithMessageStore(messageStore), engine.WithSummaryStore(summaryStore), engine.WithMemoryCompressor(compressor), engine.WithPlanService(planService), engine.WithRunRecorder(runRecorder), engine.WithTraceStore(storage.NewTraceRecorder(eventStore)))
	if err != nil {
		return err
	}
	fmt.Printf("%s %s\n", version.ProductName, version.Version)
	fmt.Printf("Project: %s\n", root)
	fmt.Printf("Session: %s\n", session.ID)
	fmt.Printf("Storage backend: %s\n", storageBackend)
	fmt.Printf("Memory backend: %s\n", memoryBackend)
	if agentMD != "" {
		fmt.Println("Loaded AGENT.md project rules.")
	}
	if opts.once != "" {
		return runPrompt(ctx, llm, session, ws.ID, root, agentMD, opts.once)
	}
	return repl(ctx, llm, shortMemory, executor, store, session, ws.ID, root, agentMD)
}

func repl(ctx context.Context, llm engine.Engine, memory cfmemory.ShortTermMemory, executor *cftools.Executor, store cfsession.Store, session *cfsession.Session, workspaceID, root, agentMD string) error {
	reader := bufio.NewReader(os.Stdin)
	var cancelCurrent context.CancelFunc
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	go func() {
		for range signals {
			if cancelCurrent != nil {
				cancelCurrent()
				fmt.Println("\nCancelled current task.")
				continue
			}
			fmt.Println("\nType /exit to leave CodeFlow.")
		}
	}()
	for {
		fmt.Print("codeflow > ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}
		if input == "exit" || input == "quit" || input == "/exit" {
			return nil
		}
		taskCtx, cancel := context.WithCancel(ctx)
		cancelCurrent = cancel
		err = handleInput(taskCtx, input, llm, memory, executor, store, session, workspaceID, root, agentMD, reader)
		cancel()
		cancelCurrent = nil
		if err != nil && !errors.Is(err, context.Canceled) {
			fmt.Println("Error:", err)
		}
	}
}

func handleInput(ctx context.Context, input string, llm engine.Engine, memory cfmemory.ShortTermMemory, executor *cftools.Executor, store cfsession.Store, session *cfsession.Session, workspaceID, root, agentMD string, reader *bufio.Reader) error {
	if !strings.HasPrefix(input, "/") {
		return runPrompt(ctx, llm, session, workspaceID, root, agentMD, input)
	}
	fields := strings.Fields(input)
	switch fields[0] {
	case "/help":
		printHelp()
	case "/version":
		fmt.Printf("%s %s\n", version.ProductName, version.Version)
	case "/clear":
		if err := memory.Clear(ctx, session.ID); err != nil {
			return err
		}
		fmt.Println("Short-term memory cleared.")
	case "/session":
		return handleSessionSlash(store, session, root, fields)
	case "/run":
		command := strings.TrimSpace(strings.TrimPrefix(input, "/run"))
		result, err := executor.Execute(ctx, cftools.Operation{
			Kind:        permission.OperationShell,
			WorkspaceID: workspaceID,
			ProjectRoot: root,
			Command:     command,
			Timeout:     60 * time.Second,
			RequestID:   nextRequestID(),
		}, session.ID)
		if result.Output != "" {
			fmt.Println(result.Output)
		}
		return err
	case "/edit":
		if len(fields) < 2 {
			return fmt.Errorf("usage: /edit <path>")
		}
		fmt.Println("Enter file content. Finish with a single '.' line.")
		var b strings.Builder
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return err
			}
			if strings.TrimSpace(line) == "." {
				break
			}
			b.WriteString(line)
		}
		result, err := executor.Execute(ctx, cftools.Operation{
			Kind:        permission.OperationWriteFile,
			WorkspaceID: workspaceID,
			ProjectRoot: root,
			Path:        fields[1],
			Content:     b.String(),
			RequestID:   nextRequestID(),
		}, session.ID)
		if result.Output != "" {
			fmt.Println(result.Output)
		}
		return err
	case "/diff":
		fmt.Println("No pending diff. File diffs are shown immediately before write confirmation.")
	default:
		return fmt.Errorf("unknown command %s; try /help", fields[0])
	}
	return nil
}

func runPrompt(ctx context.Context, llm engine.Engine, session *cfsession.Session, workspaceID, root, agentMD, input string) error {
	events, err := llm.Run(ctx, engine.Request{SessionID: session.ID, RequestID: nextRequestID(), WorkspaceID: workspaceID, ProjectRoot: root, Input: input, AgentMD: agentMD})
	if err != nil {
		return err
	}
	for event := range events {
		switch event.Type {
		case engine.EventToken:
			fmt.Print(event.Content)
		case engine.EventOutput:
			fmt.Println(event.Content)
		case engine.EventStatus:
			fmt.Printf("[%s]\n", event.Content)
		case engine.EventStats:
			fmt.Printf("\n[%s]\n", event.Content)
		case engine.EventError:
			return errors.New(event.Content)
		}
	}
	return nil
}

func handleSessionSlash(store cfsession.Store, active *cfsession.Session, root string, fields []string) error {
	if len(fields) < 2 {
		return fmt.Errorf("usage: /session list|switch <session-id>")
	}
	switch fields[1] {
	case "list":
		items, err := store.List(root)
		if err != nil {
			return err
		}
		for _, item := range items {
			marker := " "
			if item.ID == active.ID || item.Active {
				marker = "*"
			}
			fmt.Printf("%s %s %s\n", marker, item.ID, item.Title)
		}
	case "switch":
		if len(fields) != 3 {
			return fmt.Errorf("usage: /session switch <session-id>")
		}
		switched, err := store.Switch(root, fields[2])
		if err != nil {
			return err
		}
		*active = *switched
		fmt.Printf("Active session: %s\n", active.ID)
	default:
		return fmt.Errorf("usage: /session list|switch <session-id>")
	}
	return nil
}

func printHelp() {
	fmt.Println("Commands:")
	fmt.Println("  /help [command]              Show help")
	fmt.Println("  /clear                       Clear short-term memory")
	fmt.Println("  /version                     Show version")
	fmt.Println("  /session list                List project sessions")
	fmt.Println("  /session switch <session-id> Switch active session")
	fmt.Println("  /run <command>               Run a shell command after permission review")
	fmt.Println("  /edit <path>                 Write a file after diff review")
	fmt.Println("  /diff                        Explain pending diff behavior")
	fmt.Println("  /exit                        Exit CodeFlow")
}

func openSessionStore(ctx context.Context, rootFlag string) (cfsession.Store, func(), error) {
	root := projectRoot(rootFlag)
	cfg, err := cfconfig.Load(root)
	if err != nil {
		return nil, nil, err
	}
	store, _, _, err := storage.OpenSessionStoreWithFallback(ctx, cfg.Storage.PostgresDSN, cfg.DataDir)
	if err != nil {
		return nil, nil, err
	}
	return store, store.Close, nil
}

func openWorkspaceStore(ctx context.Context, rootFlag string) (storage.WorkspaceStore, func(), error) {
	root := projectRoot(rootFlag)
	postgresDSN := strings.TrimSpace(os.Getenv("CODEFLOW_POSTGRES_DSN"))
	if cfg, err := cfconfig.Load(root); err == nil && strings.TrimSpace(cfg.Storage.PostgresDSN) != "" {
		postgresDSN = cfg.Storage.PostgresDSN
	}
	if postgresDSN != "" {
		store, err := storage.NewPostgresSessionStore(ctx, postgresDSN)
		if err == nil {
			return store, store.Close, nil
		}
	}
	home, _ := os.UserHomeDir()
	registryDir := filepath.Join(home, ".codeflow")
	if strings.TrimSpace(home) == "" {
		registryDir = filepath.Join(root, ".codeflow")
	}
	store, err := storage.NewSQLiteSessionStore(filepath.Join(registryDir, "registry.db"))
	if err != nil {
		return nil, nil, err
	}
	return store, store.Close, nil
}

func nextRequestID() string {
	return "cli_" + uuid.NewString()[:8]
}

func projectRoot(flag string) string {
	root := strings.TrimSpace(flag)
	if root == "" {
		wd, _ := os.Getwd()
		root = wd
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

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

	"github.com/spf13/cobra"

	"github.com/cloudwego/codeflow/internal/codeflow/audit"
	cfconfig "github.com/cloudwego/codeflow/internal/codeflow/config"
	"github.com/cloudwego/codeflow/internal/codeflow/engine"
	cfmemory "github.com/cloudwego/codeflow/internal/codeflow/memory"
	"github.com/cloudwego/codeflow/internal/codeflow/permission"
	cfsession "github.com/cloudwego/codeflow/internal/codeflow/session"
	"github.com/cloudwego/codeflow/internal/codeflow/storage"
	cftools "github.com/cloudwego/codeflow/internal/codeflow/tools"
	"github.com/cloudwego/codeflow/internal/codeflow/version"
)

type appOptions struct {
	projectRoot string
	once        string
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
	root.AddCommand(startCommand(opts), sessionCommand(opts), configCommand(opts), versionCommand())
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
	store, err := storage.NewPostgresSessionStore(ctx, cfg.Storage.PostgresDSN)
	if err != nil {
		return err
	}
	defer store.Close()
	shortMemory, err := cfmemory.NewRedisShortTermMemory(ctx, cfg.Storage.RedisAddr, cfg.Storage.RedisPass, cfg.Storage.RedisDB)
	if err != nil {
		return err
	}
	defer shortMemory.Close()
	agentMD := readAgentMD(root)
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
	})
	executor := cftools.NewExecutor(gate, auditor)
	llm, err := engine.New(ctx, cfg, shortMemory)
	if err != nil {
		return err
	}
	fmt.Printf("%s %s\n", version.ProductName, version.Version)
	fmt.Printf("Project: %s\n", root)
	fmt.Printf("Session: %s\n", session.ID)
	if agentMD != "" {
		fmt.Println("Loaded AGENT.md project rules.")
	}
	if opts.once != "" {
		return runPrompt(ctx, llm, session, root, agentMD, opts.once)
	}
	return repl(ctx, llm, shortMemory, executor, store, session, root, agentMD)
}

func repl(ctx context.Context, llm engine.Engine, memory cfmemory.ShortTermMemory, executor *cftools.Executor, store cfsession.Store, session *cfsession.Session, root, agentMD string) error {
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
		err = handleInput(taskCtx, input, llm, memory, executor, store, session, root, agentMD, reader)
		cancel()
		cancelCurrent = nil
		if err != nil && !errors.Is(err, context.Canceled) {
			fmt.Println("Error:", err)
		}
	}
}

func handleInput(ctx context.Context, input string, llm engine.Engine, memory cfmemory.ShortTermMemory, executor *cftools.Executor, store cfsession.Store, session *cfsession.Session, root, agentMD string, reader *bufio.Reader) error {
	if !strings.HasPrefix(input, "/") {
		return runPrompt(ctx, llm, session, root, agentMD, input)
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
		result, err := executor.Execute(ctx, cftools.Operation{Kind: permission.OperationShell, ProjectRoot: root, Command: command, Timeout: 60 * time.Second}, session.ID)
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
		result, err := executor.Execute(ctx, cftools.Operation{Kind: permission.OperationWriteFile, ProjectRoot: root, Path: fields[1], Content: b.String()}, session.ID)
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

func runPrompt(ctx context.Context, llm engine.Engine, session *cfsession.Session, root, agentMD, input string) error {
	events, err := llm.Run(ctx, engine.Request{SessionID: session.ID, ProjectRoot: root, Input: input, AgentMD: agentMD})
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
	fmt.Println("  /clear                       Clear Redis short-term memory")
	fmt.Println("  /version                     Show version")
	fmt.Println("  /session list                List project sessions")
	fmt.Println("  /session switch <session-id> Switch active session")
	fmt.Println("  /run <command>               Run a shell command after permission review")
	fmt.Println("  /edit <path>                 Write a file after diff review")
	fmt.Println("  /diff                        Explain pending diff behavior")
	fmt.Println("  /exit                        Exit CodeFlow")
}

func openSessionStore(ctx context.Context, rootFlag string) (cfsession.Store, func(), error) {
	cfg, err := cfconfig.Load(projectRoot(rootFlag))
	if err != nil {
		return nil, nil, err
	}
	store, err := storage.NewPostgresSessionStore(ctx, cfg.Storage.PostgresDSN)
	if err != nil {
		return nil, nil, err
	}
	return store, store.Close, nil
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

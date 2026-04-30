package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/viko0313/CodeFlow/internal/codeflow/checkpoint"
	cfconfig "github.com/viko0313/CodeFlow/internal/codeflow/config"
	"github.com/viko0313/CodeFlow/internal/codeflow/engine"
	cfmemory "github.com/viko0313/CodeFlow/internal/codeflow/memory"
	"github.com/viko0313/CodeFlow/internal/codeflow/permission"
	"github.com/viko0313/CodeFlow/internal/codeflow/plan"
	"github.com/viko0313/CodeFlow/internal/codeflow/run"
	cfsession "github.com/viko0313/CodeFlow/internal/codeflow/session"
	"github.com/viko0313/CodeFlow/internal/codeflow/storage"
	cftools "github.com/viko0313/CodeFlow/internal/codeflow/tools"
	"github.com/viko0313/CodeFlow/internal/codeflow/workspace"
)

func planCommand(opts *appOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "plan", Short: "Manage structured plans"}
	cmd.AddCommand(&cobra.Command{
		Use:   "create <goal>",
		Short: "Generate and persist a plan for the current session",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runtime, cleanup, err := openInteractiveRuntime(cmd.Context(), opts.projectRoot)
			if err != nil {
				return err
			}
			defer cleanup()
			input := strings.Join(args, " ")
			events, err := runtime.Engine.Run(cmd.Context(), engine.Request{
				SessionID:   runtime.Session.ID,
				RequestID:   nextRequestID(),
				WorkspaceID: runtime.WorkspaceID,
				ProjectRoot: runtime.Root,
				Input:       input,
				AgentMD:     runtime.AgentMD,
				PlanEnabled: true,
			})
			if err != nil {
				return err
			}
			for event := range events {
				if event.Type == engine.EventOutput {
					fmt.Println(event.Content)
				}
			}
			if runtime.PlanStore != nil {
				items, err := runtime.PlanStore.ListPlanRecords(runtime.Session.ID, runtime.WorkspaceID, 1)
				if err == nil && len(items) > 0 {
					fmt.Printf("Plan ID: %s\n", items[0].ID)
				}
			}
			return nil
		},
	})
	cmd.AddCommand(planStateCommand(opts, "list"))
	cmd.AddCommand(planStateCommand(opts, "get"))
	cmd.AddCommand(planStateCommand(opts, "approve"))
	cmd.AddCommand(planStateCommand(opts, "pause"))
	cmd.AddCommand(planStateCommand(opts, "resume"))
	return cmd
}

func planStateCommand(opts *appOptions, action string) *cobra.Command {
	switch action {
	case "list":
		return &cobra.Command{Use: "list", Short: "List plans", RunE: func(cmd *cobra.Command, args []string) error {
			store, cleanup, err := openSessionStore(cmd.Context(), opts.projectRoot)
			if err != nil {
				return err
			}
			defer cleanup()
			planStore, ok := store.(storage.PlanStore)
			if !ok {
				return fmt.Errorf("plan store unavailable")
			}
			items, err := planStore.ListPlanRecords("", "", 50)
			if err != nil {
				return err
			}
			for _, item := range items {
				fmt.Printf("%s  %s  %s\n", item.ID, item.Status, item.Goal)
			}
			return nil
		}}
	case "get":
		return &cobra.Command{Use: "get <plan-id>", Short: "Show a plan", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			store, cleanup, err := openSessionStore(cmd.Context(), opts.projectRoot)
			if err != nil {
				return err
			}
			defer cleanup()
			planStore, ok := store.(storage.PlanStore)
			if !ok {
				return fmt.Errorf("plan store unavailable")
			}
			item, err := planStore.GetPlanRecord(args[0])
			if err != nil {
				return err
			}
			if item == nil {
				return fmt.Errorf("plan not found: %s", args[0])
			}
			fmt.Printf("%s  %s  %s\n", item.ID, item.Status, item.Goal)
			for _, step := range item.Steps {
				fmt.Printf("- %s [%s] %s\n", step.Title, step.Status, step.Type)
			}
			return nil
		}}
	default:
		return &cobra.Command{Use: action + " <plan-id>", Short: strings.Title(action) + " a plan", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			store, cleanup, err := openSessionStore(cmd.Context(), opts.projectRoot)
			if err != nil {
				return err
			}
			defer cleanup()
			planStore, ok := store.(storage.PlanStore)
			if !ok {
				return fmt.Errorf("plan store unavailable")
			}
			eventStore, _ := store.(storage.TaskEventStore)
			svc := plan.NewService(planStore)
			var item *plan.Plan
			switch action {
			case "approve":
				item, err = svc.Approve(args[0])
			case "pause":
				item, err = svc.Pause(args[0])
			case "resume":
				item, err = svc.Resume(args[0])
			}
			_ = eventStore
			if err != nil {
				return err
			}
			fmt.Printf("%s  %s\n", item.ID, item.Status)
			return nil
		}}
	}
}

func runCommand(opts *appOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "run", Short: "Inspect recorded runs"}
	cmd.AddCommand(&cobra.Command{Use: "list", Short: "List runs", RunE: func(cmd *cobra.Command, args []string) error {
		store, cleanup, err := openSessionStore(cmd.Context(), opts.projectRoot)
		if err != nil {
			return err
		}
		defer cleanup()
		runStore, ok := store.(storage.RunStore)
		if !ok {
			return fmt.Errorf("run store unavailable")
		}
		items, err := runStore.ListRunRecords("", "", 50)
		if err != nil {
			return err
		}
		for _, item := range items {
			fmt.Printf("%s  %s  %s/%s\n", item.ID, item.Status, item.ModelProvider, item.ModelName)
		}
		return nil
	}})
	cmd.AddCommand(&cobra.Command{Use: "show <run-id>", Short: "Show a run timeline", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		store, cleanup, err := openSessionStore(cmd.Context(), opts.projectRoot)
		if err != nil {
			return err
		}
		defer cleanup()
		runStore, ok := store.(storage.RunStore)
		if !ok {
			return fmt.Errorf("run store unavailable")
		}
		item, err := runStore.GetRunRecord(args[0])
		if err != nil {
			return err
		}
		if item == nil {
			return fmt.Errorf("run not found: %s", args[0])
		}
		fmt.Printf("%s  %s  %s/%s\n", item.ID, item.Status, item.ModelProvider, item.ModelName)
		events, err := runStore.ListRunEventRecords(args[0], 500)
		if err != nil {
			return err
		}
		for _, event := range events {
			fmt.Printf("%s  %s\n", event.Timestamp.Format(time.RFC3339), event.Type)
		}
		return nil
	}})
	return cmd
}

func checkpointCommand(opts *appOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "checkpoint", Short: "Inspect and rewind checkpoints"}
	cmd.AddCommand(&cobra.Command{Use: "list", Short: "List checkpoints", RunE: func(cmd *cobra.Command, args []string) error {
		rt, cleanup, err := openInteractiveRuntime(cmd.Context(), opts.projectRoot)
		if err != nil {
			return err
		}
		defer cleanup()
		if rt.CheckpointStore == nil {
			return fmt.Errorf("checkpoint store unavailable")
		}
		items, err := rt.CheckpointStore.ListCheckpointRecords(rt.Session.ID, rt.WorkspaceID, 50)
		if err != nil {
			return err
		}
		for _, item := range items {
			fmt.Printf("%s  %s  %d files\n", item.ID, item.CreatedAt.Format(time.RFC3339), len(item.ChangedFiles))
		}
		return nil
	}})
	cmd.AddCommand(&cobra.Command{Use: "show <checkpoint-id>", Short: "Show a checkpoint", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		rt, cleanup, err := openInteractiveRuntime(cmd.Context(), opts.projectRoot)
		if err != nil {
			return err
		}
		defer cleanup()
		item, err := checkpoint.NewService(rt.CheckpointStore, nil).Get(args[0])
		if err != nil {
			return err
		}
		if item == nil {
			return fmt.Errorf("checkpoint not found: %s", args[0])
		}
		fmt.Printf("%s  %s  %v\n", item.ID, item.Reason, item.ChangedFiles)
		return nil
	}})
	cmd.AddCommand(&cobra.Command{Use: "rewind <checkpoint-id>", Short: "Rewind files to a checkpoint", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		rt, cleanup, err := openInteractiveRuntime(cmd.Context(), opts.projectRoot)
		if err != nil {
			return err
		}
		defer cleanup()
		svc := checkpoint.NewService(rt.CheckpointStore, nil)
		item, err := svc.Get(args[0])
		if err != nil {
			return err
		}
		if item == nil {
			return fmt.Errorf("checkpoint not found: %s", args[0])
		}
		gate := permission.NewGate(permission.Options{ForceApproval: true})
		if _, err := gate.Review(cmd.Context(), permission.Operation{Kind: permission.OperationWriteFile, ProjectRoot: rt.Root, Path: strings.Join(item.ChangedFiles, ", "), Preview: "checkpoint rewind", Risk: "high"}); err != nil {
			return err
		}
		return svc.Rewind(cmd.Context(), rt.Root, item)
	}})
	return cmd
}

func memoryCommand(opts *appOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "memory", Short: "Inspect and compress session memory"}
	cmd.AddCommand(&cobra.Command{Use: "show", Short: "Show stored summary", RunE: func(cmd *cobra.Command, args []string) error {
		rt, cleanup, err := openInteractiveRuntime(cmd.Context(), opts.projectRoot)
		if err != nil {
			return err
		}
		defer cleanup()
		if rt.SummaryStore == nil {
			return fmt.Errorf("summary store unavailable")
		}
		item, err := rt.SummaryStore.GetSessionSummary(rt.Session.ID)
		if err != nil {
			return err
		}
		if item == nil {
			fmt.Println("No summary.")
			return nil
		}
		fmt.Println(item.Summary)
		return nil
	}})
	cmd.AddCommand(&cobra.Command{Use: "compress", Short: "Compress current session messages into a summary", RunE: func(cmd *cobra.Command, args []string) error {
		rt, cleanup, err := openInteractiveRuntime(cmd.Context(), opts.projectRoot)
		if err != nil {
			return err
		}
		defer cleanup()
		if rt.SummaryStore == nil || rt.MessageStore == nil {
			return fmt.Errorf("summary or message store unavailable")
		}
		records, err := rt.MessageStore.ListMessages(cmd.Context(), rt.Session.ID, 40)
		if err != nil {
			return err
		}
		item, err := cfmemory.NewCompressor(rt.SummaryStore, nil).Compress(cmd.Context(), rt.Session.ID, rt.WorkspaceID, records)
		if err != nil {
			return err
		}
		fmt.Println(item.Summary)
		return nil
	}})
	cmd.AddCommand(&cobra.Command{Use: "clear", Short: "Clear short-term memory and summary", RunE: func(cmd *cobra.Command, args []string) error {
		rt, cleanup, err := openInteractiveRuntime(cmd.Context(), opts.projectRoot)
		if err != nil {
			return err
		}
		defer cleanup()
		if rt.Memory != nil {
			_ = rt.Memory.Clear(cmd.Context(), rt.Session.ID)
		}
		if rt.SummaryStore != nil {
			_ = rt.SummaryStore.ClearSessionSummary(rt.Session.ID)
		}
		fmt.Println("Memory cleared.")
		return nil
	}})
	return cmd
}

type interactiveRuntime struct {
	Root            string
	AgentMD         string
	Session         *cfsession.Session
	WorkspaceID     string
	Engine          engine.Engine
	Memory          cfmemory.ShortTermMemory
	MessageStore    storage.MessageStore
	SummaryStore    storage.SummaryStore
	CheckpointStore storage.CheckpointStore
	PlanStore       storage.PlanStore
}

func openInteractiveRuntime(ctx context.Context, rootFlag string) (*interactiveRuntime, func(), error) {
	root := projectRoot(rootFlag)
	if err := cfconfig.EnsureProjectConfig(root); err != nil {
		return nil, nil, err
	}
	cfg, err := cfconfig.Load(root)
	if err != nil {
		return nil, nil, err
	}
	store, _, _, err := storage.OpenSessionStoreWithFallback(ctx, cfg.Storage.PostgresDSN, cfg.DataDir)
	if err != nil {
		return nil, nil, err
	}
	memory, _, _, err := cfmemory.OpenShortTermMemoryWithFallback(ctx, cfg.Storage.RedisAddr, cfg.Storage.RedisPass, cfg.Storage.RedisDB)
	if err != nil {
		store.Close()
		return nil, nil, err
	}
	var session *cfsession.Session
	session, err = store.GetActive(root)
	if err != nil {
		store.Close()
		_ = memory.Close()
		return nil, nil, err
	}
	agentMD := readAgentMD(root)
	if session == nil {
		session, err = store.Create(root, filepath.Base(root), agentMD)
		if err != nil {
			store.Close()
			_ = memory.Close()
			return nil, nil, err
		}
	}
	wsStore, wsCleanup, err := openWorkspaceStore(ctx, rootFlag)
	if err != nil {
		store.Close()
		_ = memory.Close()
		return nil, nil, err
	}
	ws, err := workspace.NewService(wsStore).EnsureRegistered(root)
	if err != nil {
		wsCleanup()
		store.Close()
		_ = memory.Close()
		return nil, nil, err
	}
	var msgStore storage.MessageStore
	var summaryStore storage.SummaryStore
	var runStore storage.RunStore
	var checkpointStore storage.CheckpointStore
	var planStore storage.PlanStore
	var approvalStore storage.ApprovalStore
	var eventStore storage.TaskEventStore
	if candidate, ok := store.(storage.MessageStore); ok {
		msgStore = candidate
	}
	if candidate, ok := store.(storage.SummaryStore); ok {
		summaryStore = candidate
	}
	if candidate, ok := store.(storage.RunStore); ok {
		runStore = candidate
	}
	if candidate, ok := store.(storage.CheckpointStore); ok {
		checkpointStore = candidate
	}
	if candidate, ok := store.(storage.PlanStore); ok {
		planStore = candidate
	}
	if candidate, ok := store.(storage.ApprovalStore); ok {
		approvalStore = candidate
	}
	if candidate, ok := store.(storage.TaskEventStore); ok {
		eventStore = candidate
	}
	executor := cftools.NewExecutor(permission.NewGate(permission.Options{
		TrustedCommands: cfg.Permissions.TrustedCommands,
		TrustedDirs:     cfg.Permissions.TrustedDirs,
		WritableDirs:    cfg.Permissions.WritableDirs,
		ForceApproval:   cfg.Permissions.ForceApproval,
	}), nil, approvalStore, eventStore)
	runRecorder := run.NewRecorder(runStore)
	executor.SetRunRecorder(runRecorder)
	executor.SetCheckpointService(checkpoint.NewService(checkpointStore, runRecorder))
	llm, err := engine.New(ctx, cfg, memory, engine.WithToolExecutor(executor), engine.WithMessageStore(msgStore), engine.WithSummaryStore(summaryStore), engine.WithMemoryCompressor(cfmemory.NewCompressor(summaryStore, runRecorder)), engine.WithPlanService(plan.NewService(planStore)), engine.WithRunRecorder(runRecorder), engine.WithTraceStore(storage.NewTraceRecorder(eventStore)))
	if err != nil {
		wsCleanup()
		store.Close()
		_ = memory.Close()
		return nil, nil, err
	}
	cleanup := func() {
		wsCleanup()
		store.Close()
		_ = memory.Close()
	}
	return &interactiveRuntime{
		Root:            root,
		AgentMD:         agentMD,
		Session:         session,
		WorkspaceID:     ws.ID,
		Engine:          llm,
		Memory:          memory,
		MessageStore:    msgStore,
		SummaryStore:    summaryStore,
		CheckpointStore: checkpointStore,
		PlanStore:       planStore,
	}, cleanup, nil
}

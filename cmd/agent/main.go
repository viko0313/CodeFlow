package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudwego/cyberclaw/internal/agent"
	"github.com/cloudwego/cyberclaw/internal/bus"
	"github.com/cloudwego/cyberclaw/internal/config"
	"github.com/cloudwego/cyberclaw/internal/heartbeat"
	"github.com/cloudwego/cyberclaw/internal/logger"
)

var (
	cyanColor  = "\033[36m"
	greenColor = "\033[32m"
	resetColor = "\033[0m"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "config" {
		runConfigWizard()
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := config.InitConfig(""); err != nil {
		fmt.Printf("Failed to init config: %v\n", err)
		os.Exit(1)
	}

	logDir := filepath.Join(config.GlobalConfig.Workspace, "logs")
	if err := logger.InitAuditLogger(logDir); err != nil {
		fmt.Printf("Failed to init logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.GetAuditLogger().Close()

	heartbeat.StartPacemaker(ctx, config.GlobalConfig.TasksFile(), 10*time.Second)

	cyberAgent, err := agent.NewCyberClawAgent(ctx, config.GlobalConfig)
	if err != nil {
		fmt.Printf("Failed to create agent: %v\n", err)
		fmt.Println("Tip: run `cyberclaw config` or set ARK_API_KEY before starting.")
		os.Exit(1)
	}

	printBanner(config.GlobalConfig)
	reader := bufio.NewReader(os.Stdin)
	threadID := fmt.Sprintf("session_%d", os.Getpid())

	for {
		drainTaskEvents()
		fmt.Print(fmt.Sprintf("%sUser%s > ", cyanColor, resetColor))
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Printf("Error reading input: %v\n", err)
			continue
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}
		if strings.EqualFold(input, "exit") || strings.EqualFold(input, "quit") {
			fmt.Println("Goodbye!")
			break
		}

		fmt.Print(fmt.Sprintf("%sCyberClaw%s > ", greenColor, resetColor))
		runResult, err := cyberAgent.RunDetailed(ctx, input, threadID)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}
		fmt.Println(runResult.Response)
		if len(runResult.ToolResults) > 0 {
			fmt.Println("\n--- Tool Results ---")
			for _, tr := range runResult.ToolResults {
				fmt.Printf("  - %s\n", tr)
			}
		}
		fmt.Print(renderRuntimePanel(runResult.Stats))
		drainTaskEvents()
	}
}

func runConfigWizard() {
	reader := bufio.NewReader(os.Stdin)
	defaults := config.DefaultConfig()
	fmt.Println("CyberClaw configuration")

	provider := prompt(reader, "Provider", defaults.Provider)
	model := prompt(reader, "Model", defaultModelForProvider(provider, defaults.Model))
	baseURL := prompt(reader, "Base URL", defaultBaseURLForProvider(provider, defaults.BaseURL))
	keyEnv := apiKeyEnvForProvider(provider)
	if keyEnv != "" {
		key := promptSecret(reader, fmt.Sprintf("API key (stored in local .env as %s)", keyEnv))
		if strings.TrimSpace(key) != "" {
			if err := appendEnv(".env", keyEnv, strings.TrimSpace(key)); err != nil {
				fmt.Printf("Failed to write .env: %v\n", err)
				return
			}
			_ = os.Setenv(keyEnv, strings.TrimSpace(key))
		}
	} else {
		fmt.Println("API key skipped for local Ollama.")
	}

	cfg := defaults
	cfg.Provider = provider
	cfg.Model = model
	cfg.BaseURL = baseURL
	if err := config.WriteLocalConfig("config.local.yaml", cfg); err != nil {
		fmt.Printf("Failed to write config.local.yaml: %v\n", err)
		return
	}

	fmt.Println("Configuration saved to config.local.yaml and .env. The API key was not written to config.yaml.")
	fmt.Println("Run `cyberclaw` to start.")
}

func defaultModelForProvider(provider, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "ollama":
		return "qwen3.5:4b"
	default:
		return fallback
	}
}

func defaultBaseURLForProvider(provider, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "qwen", "dashscope", "aliyun":
		return "https://dashscope.aliyuncs.com/compatible-mode/v1"
	case "ark", "volcengine":
		return "https://ark.cn-beijing.volces.com/api/v3"
	case "ollama":
		return "http://localhost:11434"
	default:
		return fallback
	}
}

func apiKeyEnvForProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "qwen", "dashscope", "aliyun":
		return "QWEN_API_KEY"
	case "ark", "volcengine":
		return "ARK_API_KEY"
	case "ollama":
		return ""
	default:
		return "OPENAI_API_KEY"
	}
}

func prompt(reader *bufio.Reader, label, fallback string) string {
	fmt.Printf("%s [%s]: ", label, fallback)
	text, _ := reader.ReadString('\n')
	text = strings.TrimSpace(text)
	if text == "" {
		return fallback
	}
	return text
}

func promptSecret(reader *bufio.Reader, label string) string {
	fmt.Printf("%s: ", label)
	text, _ := reader.ReadString('\n')
	return strings.TrimSpace(text)
}

func appendEnv(path, key, value string) error {
	lines := []string{}
	if data, err := os.ReadFile(path); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.TrimSpace(line) == "" || strings.HasPrefix(line, key+"=") {
				continue
			}
			lines = append(lines, line)
		}
	}
	lines = append(lines, fmt.Sprintf("%s=%s", key, value))
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0600)
}

func printBanner(cfg *config.Config) {
	fmt.Println("============================================================")
	fmt.Println("          CyberClaw AI Assistant (Go Edition)")
	fmt.Println("============================================================")
	fmt.Printf("Model: %s/%s\n", cfg.Provider, cfg.Model)
	fmt.Printf("Workspace: %s\n", truncatePath(cfg.Workspace, 70))
	fmt.Println("Type 'exit' or 'quit' to end the session.")
	fmt.Println()
}

func drainTaskEvents() {
	for {
		select {
		case msg := <-bus.GetTaskQueue():
			fmt.Printf("\n[Reminder] %s\n", msg.Content)
		default:
			return
		}
	}
}

func truncatePath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}
	return "..." + path[len(path)-maxLen+3:]
}

func renderRuntimePanel(stats agent.RuntimeStats) string {
	model := fmt.Sprintf("%s/%s", stats.Provider, stats.Model)
	duration := fmt.Sprintf("model=%s total=%s", stats.ModelDuration.Round(time.Millisecond), stats.TotalDuration.Round(time.Millisecond))
	memory := fmt.Sprintf("recent_turns=%d profile=%t", stats.RecentTurns, stats.ProfileLoaded)
	sandbox := fmt.Sprintf("enabled=%t", stats.SandboxEnabled)
	lines := []string{
		"+ Runtime --------------------------------------------------+",
		fmt.Sprintf("| Model   %-50s |", trimPanel(model, 50)),
		fmt.Sprintf("| Tokens  %-50s |", trimPanel(stats.TokenSummary(), 50)),
		fmt.Sprintf("| Time    %-50s |", trimPanel(duration, 50)),
		fmt.Sprintf("| Flow    %-50s |", trimPanel(stats.FlowSummary(), 50)),
		fmt.Sprintf("| Tools   %-50s |", trimPanel(stats.ToolsSummary(), 50)),
		fmt.Sprintf("| Memory  %-50s |", trimPanel(memory, 50)),
		fmt.Sprintf("| Sandbox %-50s |", trimPanel(sandbox, 50)),
		"+-----------------------------------------------------------+",
	}
	return "\n" + strings.Join(lines, "\n") + "\n"
}

func trimPanel(s string, width int) string {
	if len(s) <= width {
		return s
	}
	if width <= 3 {
		return s[:width]
	}
	return s[:width-3] + "..."
}

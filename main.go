package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/SuperCoolPencil/cue/internal/config"
	"github.com/SuperCoolPencil/cue/internal/library"
	"github.com/SuperCoolPencil/cue/internal/log"
	"github.com/SuperCoolPencil/cue/internal/mediaserver"
	"github.com/SuperCoolPencil/cue/internal/player"
	"github.com/SuperCoolPencil/cue/internal/playlist"
	"github.com/SuperCoolPencil/cue/internal/search"
	"github.com/SuperCoolPencil/cue/internal/store"
	"github.com/SuperCoolPencil/cue/internal/tui"
	"github.com/SuperCoolPencil/cue/internal/tui/styles"
	tea "github.com/charmbracelet/bubbletea"
)

// Version is set at build time via -ldflags
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// clearSpinnerLine clears the spinner line from the terminal
const clearSpinnerLine = "\r                                    \r"

func getVersion() string {
	if Version != "dev" {
		return Version
	}

	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			return info.Main.Version
		}
	}

	return Version
}

func main() {
	os.Exit(runCLI(os.Args[1:], os.Stdout, os.Stderr))
}

func runCLI(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("cue", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var showVersion bool
	fs.BoolVar(&showVersion, "v", false, "print version")
	fs.BoolVar(&showVersion, "version", false, "print version")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if showVersion {
		_, _ = fmt.Fprintf(stdout, "cue %s\n", getVersion())
		return 0
	}

	// Handle subcommands
	remainingArgs := fs.Args()
	if len(remainingArgs) > 0 {
		switch remainingArgs[0] {
		case "completion":
			if len(remainingArgs) < 2 {
				_, _ = fmt.Fprintln(stderr, "Usage: cue completion [bash|zsh|fish|powershell]")
				return 1
			}
			shell := remainingArgs[1]
			switch shell {
			case "fish":
				_, _ = fmt.Fprint(stdout, fishCompletion)
			case "bash":
				_, _ = fmt.Fprint(stdout, bashCompletion)
			case "zsh":
				_, _ = fmt.Fprint(stdout, zshCompletion)
			case "powershell":
				_, _ = fmt.Fprint(stdout, psCompletion)
			default:
				_, _ = fmt.Fprintf(stderr, "Unknown shell: %s\n", shell)
				return 1
			}
			return 0
		case "help":
			fs.SetOutput(stdout)
			fs.Usage()
			_, _ = fmt.Fprintln(stdout, "\nCommands:")
			_, _ = fmt.Fprintln(stdout, "  completion   Generate shell completion scripts")
			_, _ = fmt.Fprintln(stdout, "  help         Show this help")
			return 0
		default:
			_, _ = fmt.Fprintf(stderr, "Error: unknown command %q\n", remainingArgs[0])
			fs.Usage()
			return 1
		}
	}

	if err := run(); err != nil {
		_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	return 0
}

const bashCompletion = `_cue_completions() {
    local cur prev opts
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    opts="completion help"

    case "${prev}" in
        completion)
            COMPREPLY=( $(compgen -W "bash zsh fish powershell" -- ${cur}) )
            return 0
            ;;
        *)
            ;;
    esac

    if [[ ${cur} == -* ]] ; then
        COMPREPLY=( $(compgen -W "-v -version" -- ${cur}) )
        return 0
    fi

    COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
}
complete -F _cue_completions cue
`

const zshCompletion = `#compdef cue
_cue() {
    local line
    _arguments -C \
        "-v[print version]" \
        "--version[print version]" \
        "1: :((completion\:'Generate shell completion scripts' help\:'Show help'))" \
        "*::arg:->args"
    case $line[1] in
        completion)
            _arguments "1:shell:((bash zsh fish powershell))"
        ;;
    esac
}
_cue "$@"
`

const psCompletion = `Register-ArgumentCompleter -CommandName cue -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)
    $completions = @()
    if ($commandAst.CommandElements.Count -eq 1) {
        $completions += New-Object System.Management.Automation.CompletionResult "completion", "completion", "ParameterValue", "Generate shell completion scripts"
        $completions += New-Object System.Management.Automation.CompletionResult "help", "help", "ParameterValue", "Show help"
    } elseif ($commandAst.CommandElements[1].Value -eq "completion") {
        $completions += New-Object System.Management.Automation.CompletionResult "bash", "bash", "ParameterValue", "bash"
        $completions += New-Object System.Management.Automation.CompletionResult "zsh", "zsh", "ParameterValue", "zsh"
        $completions += New-Object System.Management.Automation.CompletionResult "fish", "fish", "ParameterValue", "fish"
        $completions += New-Object System.Management.Automation.CompletionResult "powershell", "powershell", "ParameterValue", "powershell"
    }
    $completions | Where-Object { $_.CompletionText -like "$wordToComplete*" }
}
`

const fishCompletion = `function __fish_cue_no_subcommand
    set -l cmd (commandline -opc)
    if test (count $cmd) -eq 1
        return 0
    end
    return 1
end

complete -c cue -f
complete -c cue -n "__fish_cue_no_subcommand" -a "completion" -d "Generate shell completion scripts"
complete -c cue -n "__fish_cue_no_subcommand" -a "help" -d "Show help"
complete -c cue -s v -l version -d "Print version"
complete -c cue -n "__fish_seen_subcommand_from completion" -a "bash zsh fish powershell" -d "Shell type"
`

func run() error {
	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Setup logger
	logger, err := log.SetupLogger(&cfg.Logging)
	if err != nil {
		// Fall back to null logger if file logging fails
		logger = log.NullLogger()
	}
	slog.SetDefault(logger)

	logger.Info("starting cue", "version", Version)

	// Check if configured
	if !cfg.IsConfigured() {
		return runSetupFlow(cfg, logger)
	}

	// Create media source client
	client, err := mediaserver.NewClient(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to create media client: %w", err)
	}

	// Create store (persistence layer)
	libraryStore, err := store.NewLibraryStore(config.DefaultCachePath(), cfg.Server.URL, cfg.Server.UserID)
	if err != nil {
		logger.Warn("store unavailable, continuing memory-only", "error", err)
		libraryStore, _ = store.NewLibraryStore("", "", "") // Memory-only fallback
	}
	defer func() {
		if err := libraryStore.Close(); err != nil {
			logger.Error("failed to close store", "error", err)
		}
	}() // Clean shutdown

	// Create launcher (uses configured player or auto-detects)
	launcher := player.NewLauncher(cfg.Player.Command, cfg.Player.Args, cfg.Player.StartFlag, logger)

	// Create services
	librarySvc := library.NewService(client, libraryStore, logger)
	playlistSvc := playlist.NewService(client, libraryStore, logger)
	searchSvc := search.NewService(libraryStore)
	searchSvc.SetRemote(client)
	playbackSvc := player.NewService(launcher, client, logger)

	// Create TUI model with Store and concrete service types
	model := tui.NewModel(libraryStore, librarySvc, playlistSvc, searchSvc, playbackSvc, client, cfg, cfg.UI, Version)

	// Run the TUI
	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	logger.Info("starting TUI")

	if _, err := p.Run(); err != nil {
		logger.Error("TUI error", "error", err)
		return fmt.Errorf("TUI error: %w", err)
	}

	logger.Info("shutting down")
	return nil
}

// runSetupFlow handles the initial setup when not configured
func runSetupFlow(cfg *config.Config, logger *slog.Logger) error {
	fmt.Println()
	fmt.Println("Welcome to Cue!")
	fmt.Println()

	// Loop until we get a valid server URL
	var serverURL string
	var serverType config.SourceType

	for {
		// Prompt for server URL
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Enter your server URL (e.g., http://192.168.1.100:32400): ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		serverURL = strings.TrimSpace(input)

		if serverURL == "" {
			fmt.Println("Server URL cannot be empty. Please try again.")
			continue
		}

		// Detect server type with spinner
		fmt.Println()
		detectedType, err := detectServerWithSpinner(serverURL)
		if err != nil {
			fmt.Printf("\n✗ Could not detect server type: %v\n", err)
			fmt.Println("Please check the URL and try again.")
			fmt.Println()
			continue
		}

		serverType = detectedType
		break
	}

	// Update config with server info
	cfg.Server.URL = serverURL
	cfg.Server.Type = serverType

	// Run the appropriate auth flow
	authFlow, err := mediaserver.NewAuthFlow(serverType, cfg.Server.DeviceID, logger)
	if err != nil {
		return fmt.Errorf("failed to create auth flow: %w", err)
	}

	ctx := context.Background()
	result, err := authFlow.Run(ctx, serverURL)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Save credentials
	cfg.Server.Token = result.Token
	cfg.Server.UserID = result.UserID
	cfg.Server.Username = result.Username

	if err := config.SaveConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Println()
	fmt.Println("✓ Configuration saved!")
	fmt.Println()
	fmt.Println("Run cue again to start the application.")

	return nil
}

// detectServerWithSpinner detects the server type with a visual spinner
func detectServerWithSpinner(serverURL string) (config.SourceType, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Channel to receive result
	type result struct {
		serverType config.SourceType
		err        error
	}
	resultCh := make(chan result, 1)

	// Start detection in background
	go func() {
		serverType, err := mediaserver.DetectServerType(ctx, serverURL)
		resultCh <- result{serverType, err}
	}()

	// Spinner animation
	frame := 0

	// Print initial spinner
	fmt.Printf("\r%s Detecting server type...", styles.SpinnerFrames[frame])

	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case res := <-resultCh:
			// Clear spinner line
			fmt.Print(clearSpinnerLine)

			if res.err != nil {
				return "", res.err
			}

			// Show success with server type
			serverName := "Unknown"
			switch res.serverType {
			case config.SourceTypePlex:
				serverName = "Plex Media Server"
			case config.SourceTypeJellyfin:
				serverName = "Jellyfin"
			}
			fmt.Printf("✓ Detected: %s\n", serverName)

			return res.serverType, nil

		case <-ticker.C:
			frame++
			fmt.Printf("\r%s Detecting server type...", styles.SpinnerFrames[frame%len(styles.SpinnerFrames)])

		case <-ctx.Done():
			fmt.Print(clearSpinnerLine)
			return "", fmt.Errorf("detection timed out")
		}
	}
}

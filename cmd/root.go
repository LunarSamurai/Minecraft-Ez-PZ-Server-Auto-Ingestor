package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"mcserver-manager/internal/server"
	"mcserver-manager/internal/tui"
)

var (
	// Server configuration flags
	ramMin    string
	ramMax    string
	port      int
	serverDir string
	javaPath  string
	javaArgs  string

	// Modpack flags
	modpackID      string
	modpackVersion string

	// Feature flags
	autoRestart    bool
	backupEnabled  bool
	backupInterval int
	backupDir      string
	maxBackups     int

	// Display flags
	noTUI bool
)

var rootCmd = &cobra.Command{
	Use:   "mcserver",
	Short: "🎮 High-performance Minecraft Server Manager",
	Long: `
╔══════════════════════════════════════════════════════════════════════╗
║     __  __  ____   ____                                              ║
║    |  \/  |/ ___| / ___|  ___ _ ____   _____ _ __                    ║
║    | |\/| | |     \___ \ / _ \ '__\ \ / / _ \ '__|                   ║
║    | |  | | |___   ___) |  __/ |   \ V /  __/ |                      ║
║    |_|  |_|\____| |____/ \___|_|    \_/ \___|_|                      ║
║   =================================================                  ║
║                                                                      ║
║    High-Performance Minecraft Server Manager                         ║
║    CurseForge Modpack Support                                        ║
║    Real-time Statistics & Beautiful TUI                              ║
╚══════════════════════════════════════════════════════════════════════╝

A powerful, feature-rich Minecraft server manager with:
  • CurseForge modpack auto-download and installation
  • Beautiful terminal UI with real-time statistics
  • Player tracking with join/leave events
  • TPS, memory, CPU, and bandwidth monitoring
  • Auto-restart on crash
  • Scheduled world backups
  • Graceful shutdown with save-all
  • Local mods support (./Mods folder)

Examples:
  mcserver --ram-max 8G --port 25565 --modpack 123456
  mcserver -M 4G -p 25566 --modpack 123456 --auto-restart
  mcserver --server-dir ./my-server --backup-enabled --backup-interval 30`,
	Run: runServer,
}

func init() {
	// Memory configuration
	rootCmd.Flags().StringVarP(&ramMin, "ram-min", "m", "1G", "Minimum RAM allocation (e.g., 1G, 512M)")
	rootCmd.Flags().StringVarP(&ramMax, "ram-max", "M", "4G", "Maximum RAM allocation (e.g., 4G, 8G)")

	// Network configuration
	rootCmd.Flags().IntVarP(&port, "port", "p", 25565, "Server port")

	// Paths
	rootCmd.Flags().StringVarP(&serverDir, "server-dir", "d", "./server", "Server directory path")
	rootCmd.Flags().StringVar(&javaPath, "java", "java", "Path to Java executable")
	rootCmd.Flags().StringVar(&javaArgs, "java-args", "", "Additional Java arguments")

	// Modpack configuration
	rootCmd.Flags().StringVarP(&modpackID, "modpack", "k", "", "CurseForge modpack project ID or slug")
	rootCmd.Flags().StringVar(&modpackVersion, "modpack-version", "latest", "Modpack version (latest, specific version ID)")

	// Features
	rootCmd.Flags().BoolVarP(&autoRestart, "auto-restart", "r", true, "Auto-restart server on crash")
	rootCmd.Flags().BoolVar(&backupEnabled, "backup-enabled", false, "Enable scheduled backups")
	rootCmd.Flags().IntVar(&backupInterval, "backup-interval", 60, "Backup interval in minutes")
	rootCmd.Flags().StringVar(&backupDir, "backup-dir", "./backups", "Backup directory path")
	rootCmd.Flags().IntVar(&maxBackups, "max-backups", 10, "Maximum number of backups to keep")

	// Display
	rootCmd.Flags().BoolVar(&noTUI, "no-tui", false, "Disable TUI, use simple console output")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runServer(cmd *cobra.Command, args []string) {
	// Create absolute paths
	absServerDir, err := filepath.Abs(serverDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving server directory: %v\n", err)
		os.Exit(1)
	}

	absBackupDir, err := filepath.Abs(backupDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving backup directory: %v\n", err)
		os.Exit(1)
	}

	// Build server configuration
	config := &server.Config{
		RamMin:         ramMin,
		RamMax:         ramMax,
		Port:           port,
		ServerDir:      absServerDir,
		JavaPath:       javaPath,
		JavaArgs:       javaArgs,
		ModpackID:      modpackID,
		ModpackVersion: modpackVersion,
		AutoRestart:    autoRestart,
		BackupEnabled:  backupEnabled,
		BackupInterval: backupInterval,
		BackupDir:      absBackupDir,
		MaxBackups:     maxBackups,
	}

	if noTUI {
		// Run in simple console mode
		srv := server.New(config)
		if err := srv.RunConsole(); err != nil {
			fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Run with beautiful TUI
		if err := tui.Run(config); err != nil {
			fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
			os.Exit(1)
		}
	}
}

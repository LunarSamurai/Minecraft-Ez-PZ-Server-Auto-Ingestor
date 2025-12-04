package server

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/process"

	"mcserver-manager/internal/backup"
	"mcserver-manager/internal/curseforge"
)

// Server manages the Minecraft server process
type Server struct {
	config *Config

	// Process management
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	process *process.Process

	// State
	stats      ServerStats
	statsMutex sync.RWMutex

	// Channels
	outputChan chan string
	eventChan  chan ServerEvent
	stopChan   chan struct{}

	// Network tracking
	lastBytesIn  uint64
	lastBytesOut uint64
	lastNetCheck time.Time

	// Context for cancellation
	ctx        context.Context
	cancelFunc context.CancelFunc

	// Backup manager
	backupMgr *backup.Manager
}

// Regex patterns for parsing server output
var (
	playerJoinRegex  = regexp.MustCompile(`\[Server thread/INFO\].*?: (\w+) joined the game`)
	playerLeaveRegex = regexp.MustCompile(`\[Server thread/INFO\].*?: (\w+) left the game`)
	playerListRegex  = regexp.MustCompile(`There are (\d+) of a max of (\d+) players online`)
	tpsRegex         = regexp.MustCompile(`Mean TPS: ([\d.]+)`)
	doneRegex        = regexp.MustCompile(`Done \([\d.]+s\)! For help, type "help"`)
	chatRegex        = regexp.MustCompile(`<(\w+)> (.+)`)
	uuidRegex        = regexp.MustCompile(`UUID of player (\w+) is ([a-f0-9-]+)`)
	ipRegex          = regexp.MustCompile(`(\w+)\[/(\d+\.\d+\.\d+\.\d+):\d+\] logged in`)
)

// New creates a new Server instance
func New(config *Config) *Server {
	ctx, cancel := context.WithCancel(context.Background())

	s := &Server{
		config:     config,
		outputChan: make(chan string, 1000),
		eventChan:  make(chan ServerEvent, 100),
		stopChan:   make(chan struct{}),
		ctx:        ctx,
		cancelFunc: cancel,
		stats: ServerStats{
			Status:       StatusStopped,
			Players:      make([]Player, 0),
			RecentEvents: make([]ServerEvent, 0),
			MaxPlayers:   20,
			TPS:          20.0, // Default to 20 TPS
		},
	}

	if config.BackupEnabled {
		s.backupMgr = backup.NewManager(config.ServerDir, config.BackupDir, config.MaxBackups)
	}

	return s
}

// GetStats returns a copy of current server stats
func (s *Server) GetStats() ServerStats {
	s.statsMutex.RLock()
	defer s.statsMutex.RUnlock()

	stats := s.stats
	stats.Players = make([]Player, len(s.stats.Players))
	copy(stats.Players, s.stats.Players)
	stats.RecentEvents = make([]ServerEvent, len(s.stats.RecentEvents))
	copy(stats.RecentEvents, s.stats.RecentEvents)

	if s.stats.Status == StatusRunning {
		stats.Uptime = time.Since(s.stats.StartTime)
	}

	return stats
}

// OutputChan returns the channel for server output
func (s *Server) OutputChan() <-chan string {
	return s.outputChan
}

// EventChan returns the channel for server events
func (s *Server) EventChan() <-chan ServerEvent {
	return s.eventChan
}

// Start starts the Minecraft server
func (s *Server) Start() error {
	s.updateStatus(StatusStarting)

	// Ensure server directory exists
	if err := os.MkdirAll(s.config.ServerDir, 0755); err != nil {
		return fmt.Errorf("failed to create server directory: %w", err)
	}

	// Download and install modpack if specified
	if s.config.ModpackID != "" {
		if err := s.installModpack(); err != nil {
			s.addEvent(EventError, fmt.Sprintf("Modpack installation failed: %v", err))
			return fmt.Errorf("modpack installation failed: %w", err)
		}
	}

	// Copy local mods from ./Mods or ./mods directory
	if err := s.copyLocalMods(); err != nil {
		s.addEvent(EventWarning, fmt.Sprintf("Local mods copy warning: %v", err))
	}

	// Find server JAR
	serverJar, err := s.findServerJar()
	if err != nil {
		return fmt.Errorf("failed to find server JAR: %w", err)
	}

	// Accept EULA
	if err := s.acceptEULA(); err != nil {
		s.addEvent(EventWarning, "Could not auto-accept EULA")
	}

	// Configure server.properties
	if err := s.configureServerProperties(); err != nil {
		s.addEvent(EventWarning, fmt.Sprintf("Could not configure server.properties: %v", err))
	}

	// Build Java command
	args := s.buildJavaArgs(serverJar)

	s.cmd = exec.CommandContext(s.ctx, s.config.JavaPath, args...)
	s.cmd.Dir = s.config.ServerDir

	// Set up pipes
	stdout, err := s.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := s.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	s.stdin, err = s.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	// Start the process
	if err := s.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	// Get process for monitoring
	s.process, _ = process.NewProcess(int32(s.cmd.Process.Pid))

	s.statsMutex.Lock()
	s.stats.StartTime = time.Now()
	s.statsMutex.Unlock()

	// Start output readers
	go s.readOutput(stdout)
	go s.readOutput(stderr)

	// Start monitoring
	go s.monitorProcess()
	go s.updateStatsLoop()
	go s.requestTPSLoop()

	// Start backup scheduler if enabled
	if s.config.BackupEnabled && s.backupMgr != nil {
		go s.backupScheduler()
	}

	s.addEvent(EventInfo, "Server starting...")

	return nil
}

// Stop gracefully stops the server
func (s *Server) Stop() error {
	if s.stats.Status != StatusRunning && s.stats.Status != StatusStarting {
		return nil
	}

	s.updateStatus(StatusStopping)
	s.addEvent(EventInfo, "Stopping server gracefully...")

	// Send stop command
	if err := s.SendCommand("save-all"); err != nil {
		s.addEvent(EventWarning, "Could not send save-all command")
	}

	time.Sleep(2 * time.Second)

	if err := s.SendCommand("stop"); err != nil {
		s.addEvent(EventWarning, "Could not send stop command, forcing shutdown")
		if s.cmd != nil && s.cmd.Process != nil {
			s.cmd.Process.Kill()
		}
	}

	// Wait for process to exit with timeout
	done := make(chan error, 1)
	go func() {
		if s.cmd != nil {
			done <- s.cmd.Wait()
		} else {
			done <- nil
		}
	}()

	select {
	case <-done:
		s.addEvent(EventInfo, "Server stopped gracefully")
	case <-time.After(30 * time.Second):
		s.addEvent(EventWarning, "Server did not stop in time, forcing kill")
		if s.cmd != nil && s.cmd.Process != nil {
			s.cmd.Process.Kill()
		}
	}

	s.updateStatus(StatusStopped)
	return nil
}

// SendCommand sends a command to the server console
func (s *Server) SendCommand(command string) error {
	if s.stdin == nil {
		return fmt.Errorf("server not running")
	}

	_, err := fmt.Fprintln(s.stdin, command)
	if err != nil {
		return fmt.Errorf("failed to send command: %w", err)
	}

	// Don't log TPS commands to avoid spam
	if command != "forge tps" {
		s.addEvent(EventCommand, fmt.Sprintf("Executed: %s", command))
	}
	return nil
}

// Restart restarts the server
func (s *Server) Restart() error {
	s.addEvent(EventRestart, "Restarting server...")
	s.updateStatus(StatusRestarting)

	s.statsMutex.Lock()
	s.stats.Restarts++
	s.statsMutex.Unlock()

	if err := s.Stop(); err != nil {
		return fmt.Errorf("failed to stop server for restart: %w", err)
	}

	time.Sleep(2 * time.Second)

	return s.Start()
}

// RunConsole runs the server in simple console mode (no TUI)
func (s *Server) RunConsole() error {
	if err := s.Start(); err != nil {
		return err
	}

	// Print output to console
	go func() {
		for line := range s.outputChan {
			fmt.Println(line)
		}
	}()

	// Wait for process to exit
	if s.cmd != nil {
		return s.cmd.Wait()
	}
	return nil
}

// requestTPSLoop periodically requests TPS from the server
func (s *Server) requestTPSLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Wait for server to fully start
	time.Sleep(15 * time.Second)

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			if s.stats.Status == StatusRunning {
				s.SendCommand("forge tps")
			}
		}
	}
}

// copyLocalMods copies mods from the current directory's Mods folder to the server
func (s *Server) copyLocalMods() error {
	// Check for local Mods folder in current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return nil // Not critical, skip
	}

	localModsDir := filepath.Join(cwd, "Mods")
	if _, err := os.Stat(localModsDir); os.IsNotExist(err) {
		// Also check for lowercase "mods"
		localModsDir = filepath.Join(cwd, "mods")
		if _, err := os.Stat(localModsDir); os.IsNotExist(err) {
			return nil // No local mods folder, skip
		}
	}

	// Create server mods directory
	serverModsDir := filepath.Join(s.config.ServerDir, "mods")
	if err := os.MkdirAll(serverModsDir, 0755); err != nil {
		return fmt.Errorf("failed to create server mods directory: %w", err)
	}

	// Read local mods
	entries, err := os.ReadDir(localModsDir)
	if err != nil {
		return fmt.Errorf("failed to read local mods directory: %w", err)
	}

	modsCopied := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".jar") {
			continue
		}

		srcPath := filepath.Join(localModsDir, name)
		dstPath := filepath.Join(serverModsDir, name)

		// Check if mod already exists
		if _, err := os.Stat(dstPath); err == nil {
			continue // Already exists, skip
		}

		// Copy the mod
		if err := copyFile(srcPath, dstPath); err != nil {
			s.addEvent(EventWarning, fmt.Sprintf("Failed to copy mod %s: %v", name, err))
			continue
		}

		modsCopied++
		s.addEvent(EventInfo, fmt.Sprintf("Added local mod: %s", name))
	}

	if modsCopied > 0 {
		s.addEvent(EventInfo, fmt.Sprintf("Copied %d local mod(s) to server", modsCopied))
	}

	return nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// installModpack downloads and installs the CurseForge modpack
func (s *Server) installModpack() error {
	s.updateStatus(StatusDownloading)
	s.addEvent(EventInfo, fmt.Sprintf("Downloading modpack: %s", s.config.ModpackID))

	cf := curseforge.NewClient()

	// Download modpack
	modpackPath, err := cf.DownloadModpack(s.config.ModpackID, s.config.ModpackVersion, s.config.ServerDir)
	if err != nil {
		return fmt.Errorf("failed to download modpack: %w", err)
	}

	s.updateStatus(StatusInstalling)
	s.addEvent(EventInfo, "Installing modpack...")

	// Extract and install
	if err := cf.InstallModpack(modpackPath, s.config.ServerDir); err != nil {
		return fmt.Errorf("failed to install modpack: %w", err)
	}

	s.addEvent(EventInfo, "Modpack installed successfully")
	return nil
}

// findServerJar finds the server JAR file or detects Forge server
func (s *Server) findServerJar() (string, error) {
	// Check if this is a Forge server with run.sh
	runShPath := filepath.Join(s.config.ServerDir, "run.sh")
	if _, err := os.Stat(runShPath); err == nil {
		// Check for unix_args.txt which indicates Forge
		forgeLibPath := filepath.Join(s.config.ServerDir, "libraries/net/minecraftforge/forge")
		if _, err := os.Stat(forgeLibPath); err == nil {
			return "forge", nil // Special marker for Forge servers
		}
	}

	// Common server JAR names
	jarNames := []string{
		"server.jar",
		"forge-*.jar",
		"fabric-server-*.jar",
		"minecraft_server.*.jar",
		"paper-*.jar",
		"spigot-*.jar",
	}

	for _, pattern := range jarNames {
		matches, err := filepath.Glob(filepath.Join(s.config.ServerDir, pattern))
		if err == nil && len(matches) > 0 {
			return filepath.Base(matches[0]), nil
		}
	}

	// Look for any .jar file with "server" in the name
	entries, err := os.ReadDir(s.config.ServerDir)
	if err != nil {
		return "", fmt.Errorf("failed to read server directory: %w", err)
	}

	for _, entry := range entries {
		name := strings.ToLower(entry.Name())
		if strings.HasSuffix(name, ".jar") && strings.Contains(name, "server") {
			return entry.Name(), nil
		}
	}

	// Just look for any .jar
	for _, entry := range entries {
		if strings.HasSuffix(strings.ToLower(entry.Name()), ".jar") {
			return entry.Name(), nil
		}
	}

	return "", fmt.Errorf("no server JAR found in %s", s.config.ServerDir)
}

// acceptEULA creates/updates eula.txt
func (s *Server) acceptEULA() error {
	eulaPath := filepath.Join(s.config.ServerDir, "eula.txt")
	return os.WriteFile(eulaPath, []byte("eula=true\n"), 0644)
}

// configureServerProperties sets up server.properties
func (s *Server) configureServerProperties() error {
	propsPath := filepath.Join(s.config.ServerDir, "server.properties")

	props := make(map[string]string)

	// Read existing properties if file exists
	if data, err := os.ReadFile(propsPath); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				props[parts[0]] = parts[1]
			}
		}
	}

	// Set our configuration
	props["server-port"] = strconv.Itoa(s.config.Port)

	// Write back
	var lines []string
	lines = append(lines, "# Minecraft Server Properties")
	lines = append(lines, fmt.Sprintf("# Generated by MCServer Manager on %s", time.Now().Format(time.RFC3339)))
	lines = append(lines, "")

	for key, value := range props {
		lines = append(lines, fmt.Sprintf("%s=%s", key, value))
	}

	return os.WriteFile(propsPath, []byte(strings.Join(lines, "\n")), 0644)
}

// buildJavaArgs constructs the Java command arguments
func (s *Server) buildJavaArgs(serverJar string) []string {
	// Check if this is a Forge server (serverJar == "forge")
	if serverJar == "forge" {
		return s.buildForgeArgs()
	}

	args := []string{
		fmt.Sprintf("-Xms%s", s.config.RamMin),
		fmt.Sprintf("-Xmx%s", s.config.RamMax),
	}

	// Performance optimizations
	args = append(args,
		"-XX:+UseG1GC",
		"-XX:+ParallelRefProcEnabled",
		"-XX:MaxGCPauseMillis=200",
		"-XX:+UnlockExperimentalVMOptions",
		"-XX:+DisableExplicitGC",
		"-XX:+AlwaysPreTouch",
		"-XX:G1NewSizePercent=30",
		"-XX:G1MaxNewSizePercent=40",
		"-XX:G1HeapRegionSize=8M",
		"-XX:G1ReservePercent=20",
		"-XX:G1HeapWastePercent=5",
		"-XX:G1MixedGCCountTarget=4",
		"-XX:InitiatingHeapOccupancyPercent=15",
		"-XX:G1MixedGCLiveThresholdPercent=90",
		"-XX:G1RSetUpdatingPauseTimePercent=5",
		"-XX:SurvivorRatio=32",
		"-XX:+PerfDisableSharedMem",
		"-XX:MaxTenuringThreshold=1",
		"-Dusing.aikars.flags=https://mcflags.emc.gs",
		"-Daikars.new.flags=true",
	)

	// Additional custom args
	if s.config.JavaArgs != "" {
		args = append(args, strings.Fields(s.config.JavaArgs)...)
	}

	// Server JAR
	args = append(args, "-jar", serverJar, "nogui")

	return args
}

// buildForgeArgs builds arguments for Forge servers using @args files
func (s *Server) buildForgeArgs() []string {
	// Create user_jvm_args.txt with our memory settings
	userArgsPath := filepath.Join(s.config.ServerDir, "user_jvm_args.txt")
	userArgs := fmt.Sprintf(`-Xms%s
-Xmx%s
-XX:+UseG1GC
-XX:+ParallelRefProcEnabled
-XX:MaxGCPauseMillis=200
-XX:+UnlockExperimentalVMOptions
-XX:+DisableExplicitGC
-XX:+AlwaysPreTouch
-XX:G1NewSizePercent=30
-XX:G1MaxNewSizePercent=40
-XX:G1HeapRegionSize=8M
-XX:G1ReservePercent=20
-XX:G1HeapWastePercent=5
-XX:G1MixedGCCountTarget=4
-XX:InitiatingHeapOccupancyPercent=15
-XX:G1MixedGCLiveThresholdPercent=90
-XX:G1RSetUpdatingPauseTimePercent=5
-XX:SurvivorRatio=32
-XX:+PerfDisableSharedMem
-XX:MaxTenuringThreshold=1
`, s.config.RamMin, s.config.RamMax)

	os.WriteFile(userArgsPath, []byte(userArgs), 0644)

	// Find the unix_args.txt file (or win_args.txt on Windows)
	var argsFile string

	// Check for Windows args first
	filepath.Walk(filepath.Join(s.config.ServerDir, "libraries/net/minecraftforge/forge"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if strings.HasSuffix(path, "win_args.txt") {
			argsFile = path
			return filepath.SkipAll
		}
		if strings.HasSuffix(path, "unix_args.txt") && argsFile == "" {
			argsFile = path
		}
		return nil
	})

	if argsFile == "" {
		// Fallback - just try to run the forge jar directly
		// Find forge jar
		matches, _ := filepath.Glob(filepath.Join(s.config.ServerDir, "libraries/net/minecraftforge/forge/*/forge-*.jar"))
		if len(matches) > 0 {
			return []string{
				fmt.Sprintf("-Xms%s", s.config.RamMin),
				fmt.Sprintf("-Xmx%s", s.config.RamMax),
				"-jar", matches[0], "nogui",
			}
		}
		return []string{"-jar", "server.jar", "nogui"}
	}

	// Read the args file and parse it manually instead of using @
	argsContent, err := os.ReadFile(argsFile)
	if err != nil {
		return []string{"-jar", "server.jar", "nogui"}
	}

	// Parse the args file content
	var args []string

	// Add our JVM args first
	args = append(args,
		fmt.Sprintf("-Xms%s", s.config.RamMin),
		fmt.Sprintf("-Xmx%s", s.config.RamMax),
		"-XX:+UseG1GC",
		"-XX:+ParallelRefProcEnabled",
		"-XX:MaxGCPauseMillis=200",
		"-XX:+UnlockExperimentalVMOptions",
		"-XX:+DisableExplicitGC",
		"-XX:+AlwaysPreTouch",
	)

	// Parse the forge args file
	lines := strings.Split(string(argsContent), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Handle line continuations (backslash at end)
		line = strings.TrimSuffix(line, "\\")
		line = strings.TrimSpace(line)

		// Split by spaces but respect quotes
		parts := parseArgsLine(line)
		args = append(args, parts...)
	}

	// Add nogui at the end
	args = append(args, "nogui")

	return args
}

// parseArgsLine parses a line handling spaces and basic quoting
func parseArgsLine(line string) []string {
	var args []string
	var current strings.Builder
	inQuote := false

	for _, r := range line {
		switch r {
		case '"':
			inQuote = !inQuote
		case ' ':
			if inQuote {
				current.WriteRune(r)
			} else if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}

	if current.Len() > 0 {
		args = append(args, current.String())
	}

	return args
}

// readOutput reads from a pipe and sends to output channel
func (s *Server) readOutput(pipe io.ReadCloser) {
	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		line := scanner.Text()

		select {
		case s.outputChan <- line:
		default:
			// Channel full, skip
		}

		s.parseOutput(line)
	}
}

// parseOutput parses server output for events and stats
func (s *Server) parseOutput(line string) {
	// Check for server done starting
	if doneRegex.MatchString(line) {
		s.updateStatus(StatusRunning)
		s.addEvent(EventInfo, "Server started successfully!")
		return
	}

	// Check for player join
	if matches := playerJoinRegex.FindStringSubmatch(line); len(matches) > 1 {
		playerName := matches[1]
		s.addPlayer(playerName)
		s.addEvent(EventPlayerJoin, fmt.Sprintf("%s joined the game", playerName))
		return
	}

	// Check for player leave
	if matches := playerLeaveRegex.FindStringSubmatch(line); len(matches) > 1 {
		playerName := matches[1]
		s.removePlayer(playerName)
		s.addEvent(EventPlayerLeave, fmt.Sprintf("%s left the game", playerName))
		return
	}

	// Check for player list response
	if matches := playerListRegex.FindStringSubmatch(line); len(matches) > 2 {
		current, _ := strconv.Atoi(matches[1])
		max, _ := strconv.Atoi(matches[2])
		s.statsMutex.Lock()
		s.stats.PlayerCount = current
		s.stats.MaxPlayers = max
		s.statsMutex.Unlock()
		return
	}

	// Check for TPS (Forge format: "Mean TPS: 20.00")
	if matches := tpsRegex.FindStringSubmatch(line); len(matches) > 1 {
		tps, _ := strconv.ParseFloat(matches[1], 64)
		s.statsMutex.Lock()
		s.stats.TPS = tps
		s.statsMutex.Unlock()
		return
	}

	// Check for chat
	if matches := chatRegex.FindStringSubmatch(line); len(matches) > 2 {
		s.addEvent(EventChat, fmt.Sprintf("<%s> %s", matches[1], matches[2]))
		return
	}

	// Check for player IP (on join)
	if matches := ipRegex.FindStringSubmatch(line); len(matches) > 2 {
		s.updatePlayerIP(matches[1], matches[2])
		return
	}

	// Check for UUID
	if matches := uuidRegex.FindStringSubmatch(line); len(matches) > 2 {
		s.updatePlayerUUID(matches[1], matches[2])
		return
	}

	// Check for errors/warnings (but not TPS spam)
	if strings.Contains(line, "Mean TPS:") || strings.Contains(line, "Mean tick time:") {
		return // Skip TPS output from being logged as events
	}

	if strings.Contains(line, "[WARN]") || strings.Contains(line, "WARN]") {
		s.addEvent(EventWarning, line)
		return
	}

	if strings.Contains(line, "[ERROR]") || strings.Contains(line, "ERROR]") {
		s.addEvent(EventError, line)
		return
	}
}

// monitorProcess monitors the server process
func (s *Server) monitorProcess() {
	if s.cmd == nil {
		return
	}

	err := s.cmd.Wait()

	if s.stats.Status == StatusStopping {
		s.updateStatus(StatusStopped)
		return
	}

	// Unexpected exit
	if err != nil {
		s.updateStatus(StatusCrashed)
		s.addEvent(EventError, fmt.Sprintf("Server crashed: %v", err))

		if s.config.AutoRestart {
			s.addEvent(EventRestart, "Auto-restarting in 5 seconds...")
			time.Sleep(5 * time.Second)

			if s.stats.Status == StatusCrashed {
				go s.Restart()
			}
		}
	} else {
		s.updateStatus(StatusStopped)
	}
}

// updateStatsLoop periodically updates server statistics
func (s *Server) updateStatsLoop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.updateResourceStats()
		}
	}
}

// updateResourceStats updates CPU, memory, and network stats
func (s *Server) updateResourceStats() {
	if s.process == nil {
		return
	}

	s.statsMutex.Lock()
	defer s.statsMutex.Unlock()

	// CPU
	if cpu, err := s.process.CPUPercent(); err == nil {
		s.stats.CPUPercent = cpu
	}

	// Memory
	if mem, err := s.process.MemoryInfo(); err == nil {
		s.stats.MemoryUsed = mem.RSS
	}

	// Parse max memory from config
	s.stats.MemoryMax = parseMemoryString(s.config.RamMax)

	// Network I/O
	if ioCounters, err := s.process.IOCounters(); err == nil {
		now := time.Now()
		if !s.lastNetCheck.IsZero() {
			elapsed := now.Sub(s.lastNetCheck).Seconds()
			if elapsed > 0 {
				s.stats.BandwidthIn = float64(ioCounters.ReadBytes-s.lastBytesIn) / elapsed
				s.stats.BandwidthOut = float64(ioCounters.WriteBytes-s.lastBytesOut) / elapsed
			}
		}
		s.stats.BytesIn = ioCounters.ReadBytes
		s.stats.BytesOut = ioCounters.WriteBytes
		s.lastBytesIn = ioCounters.ReadBytes
		s.lastBytesOut = ioCounters.WriteBytes
		s.lastNetCheck = now
	}

	// Update player count
	s.stats.PlayerCount = len(s.stats.Players)
}

// backupScheduler runs scheduled backups
func (s *Server) backupScheduler() {
	ticker := time.NewTicker(time.Duration(s.config.BackupInterval) * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			if s.stats.Status == StatusRunning {
				s.performBackup()
			}
		}
	}
}

// performBackup creates a world backup
func (s *Server) performBackup() {
	s.addEvent(EventBackup, "Starting world backup...")

	// Disable autosave and save
	s.SendCommand("save-off")
	s.SendCommand("save-all flush")
	time.Sleep(2 * time.Second)

	// Create backup
	if s.backupMgr != nil {
		if err := s.backupMgr.CreateBackup(); err != nil {
			s.addEvent(EventError, fmt.Sprintf("Backup failed: %v", err))
		} else {
			s.addEvent(EventBackup, "Backup completed successfully")
		}
	}

	// Re-enable autosave
	s.SendCommand("save-on")
}

// Helper functions

func (s *Server) updateStatus(status ServerStatus) {
	s.statsMutex.Lock()
	s.stats.Status = status
	s.statsMutex.Unlock()
}

func (s *Server) addEvent(eventType EventType, message string) {
	event := ServerEvent{
		Time:    time.Now(),
		Type:    eventType,
		Message: message,
	}

	s.statsMutex.Lock()
	s.stats.RecentEvents = append(s.stats.RecentEvents, event)
	if len(s.stats.RecentEvents) > 100 {
		s.stats.RecentEvents = s.stats.RecentEvents[1:]
	}
	s.statsMutex.Unlock()

	select {
	case s.eventChan <- event:
	default:
	}
}

func (s *Server) addPlayer(name string) {
	s.statsMutex.Lock()
	defer s.statsMutex.Unlock()

	for _, p := range s.stats.Players {
		if p.Name == name {
			return
		}
	}

	s.stats.Players = append(s.stats.Players, Player{
		Name:     name,
		JoinedAt: time.Now(),
	})
	s.stats.PlayerCount = len(s.stats.Players)
}

func (s *Server) removePlayer(name string) {
	s.statsMutex.Lock()
	defer s.statsMutex.Unlock()

	for i, p := range s.stats.Players {
		if p.Name == name {
			s.stats.Players = append(s.stats.Players[:i], s.stats.Players[i+1:]...)
			break
		}
	}
	s.stats.PlayerCount = len(s.stats.Players)
}

func (s *Server) updatePlayerUUID(name, uuid string) {
	s.statsMutex.Lock()
	defer s.statsMutex.Unlock()

	for i, p := range s.stats.Players {
		if p.Name == name {
			s.stats.Players[i].UUID = uuid
			return
		}
	}
}

func (s *Server) updatePlayerIP(name, ip string) {
	s.statsMutex.Lock()
	defer s.statsMutex.Unlock()

	for i, p := range s.stats.Players {
		if p.Name == name {
			s.stats.Players[i].IPAddress = ip
			return
		}
	}
}

func parseMemoryString(mem string) uint64 {
	mem = strings.ToUpper(strings.TrimSpace(mem))

	multiplier := uint64(1)
	if strings.HasSuffix(mem, "G") {
		multiplier = 1024 * 1024 * 1024
		mem = strings.TrimSuffix(mem, "G")
	} else if strings.HasSuffix(mem, "M") {
		multiplier = 1024 * 1024
		mem = strings.TrimSuffix(mem, "M")
	} else if strings.HasSuffix(mem, "K") {
		multiplier = 1024
		mem = strings.TrimSuffix(mem, "K")
	}

	value, _ := strconv.ParseUint(mem, 10, 64)
	return value * multiplier
}

package server

import (
	"time"
)

// Config holds all server configuration
type Config struct {
	// Memory settings
	RamMin string
	RamMax string

	// Network settings
	Port int

	// Paths
	ServerDir string
	JavaPath  string
	JavaArgs  string

	// Modpack settings
	ModpackID      string
	ModpackVersion string

	// Feature flags
	AutoRestart    bool
	BackupEnabled  bool
	BackupInterval int
	BackupDir      string
	MaxBackups     int
}

// Player represents a connected player
type Player struct {
	Name      string
	UUID      string
	JoinedAt  time.Time
	IPAddress string
}

// ServerStats holds real-time server statistics
type ServerStats struct {
	// Server status
	Status    ServerStatus
	StartTime time.Time
	Uptime    time.Duration
	Restarts  int

	// Performance
	TPS        float64
	MemoryUsed uint64
	MemoryMax  uint64
	CPUPercent float64

	// Network
	BytesIn      uint64
	BytesOut     uint64
	BandwidthIn  float64 // bytes per second
	BandwidthOut float64

	// Players
	Players     []Player
	PlayerCount int
	MaxPlayers  int

	// Events
	RecentEvents []ServerEvent
}

// ServerStatus represents the current server state
type ServerStatus int

const (
	StatusStopped ServerStatus = iota
	StatusStarting
	StatusRunning
	StatusStopping
	StatusCrashed
	StatusRestarting
	StatusDownloading
	StatusInstalling
)

func (s ServerStatus) String() string {
	switch s {
	case StatusStopped:
		return "Stopped"
	case StatusStarting:
		return "Starting"
	case StatusRunning:
		return "Running"
	case StatusStopping:
		return "Stopping"
	case StatusCrashed:
		return "Crashed"
	case StatusRestarting:
		return "Restarting"
	case StatusDownloading:
		return "Downloading Modpack"
	case StatusInstalling:
		return "Installing Modpack"
	default:
		return "Unknown"
	}
}

func (s ServerStatus) Color() string {
	switch s {
	case StatusStopped:
		return "#888888"
	case StatusStarting, StatusRestarting:
		return "#FFAA00"
	case StatusRunning:
		return "#00FF00"
	case StatusStopping:
		return "#FFAA00"
	case StatusCrashed:
		return "#FF0000"
	case StatusDownloading, StatusInstalling:
		return "#00AAFF"
	default:
		return "#FFFFFF"
	}
}

// ServerEvent represents a server event for the event log
type ServerEvent struct {
	Time    time.Time
	Type    EventType
	Message string
}

// EventType categorizes server events
type EventType int

const (
	EventInfo EventType = iota
	EventWarning
	EventError
	EventPlayerJoin
	EventPlayerLeave
	EventChat
	EventCommand
	EventBackup
	EventRestart
)

func (e EventType) String() string {
	switch e {
	case EventInfo:
		return "INFO"
	case EventWarning:
		return "WARN"
	case EventError:
		return "ERROR"
	case EventPlayerJoin:
		return "JOIN"
	case EventPlayerLeave:
		return "LEAVE"
	case EventChat:
		return "CHAT"
	case EventCommand:
		return "CMD"
	case EventBackup:
		return "BACKUP"
	case EventRestart:
		return "RESTART"
	default:
		return "UNKNOWN"
	}
}

func (e EventType) Color() string {
	switch e {
	case EventInfo:
		return "#AAAAAA"
	case EventWarning:
		return "#FFAA00"
	case EventError:
		return "#FF5555"
	case EventPlayerJoin:
		return "#55FF55"
	case EventPlayerLeave:
		return "#FF5555"
	case EventChat:
		return "#55FFFF"
	case EventCommand:
		return "#AA55FF"
	case EventBackup:
		return "#5555FF"
	case EventRestart:
		return "#FFFF55"
	default:
		return "#FFFFFF"
	}
}

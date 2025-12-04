# 🎮 Minecraft Server Manager

A high-performance, feature-rich Minecraft server manager written in Go with a beautiful terminal UI, CurseForge modpack support, and comprehensive server management features.

![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Go Version](https://img.shields.io/badge/go-%3E%3D1.21-00ADD8.svg)

## ✨ Features

### 🖥️ Beautiful Terminal UI
- Real-time server statistics dashboard
- TPS, memory, CPU, and bandwidth monitoring with visual graphs
- Player list with join times and session duration
- Color-coded event log (joins, leaves, warnings, errors)
- Interactive console with command input
- Responsive layout that adapts to terminal size

### 📦 CurseForge Integration
- Download modpacks directly by project ID or name
- Automatic server pack detection and installation
- Supports Forge, Fabric, and NeoForge mod loaders
- Automatic mod downloads from manifest

### 🔧 Server Management
- Graceful shutdown with save-all
- Auto-restart on crash with configurable delay
- Optimized JVM flags (Aikar's flags) for best performance
- Automatic EULA acceptance
- Server.properties configuration

### 💾 Backup System
- Scheduled world backups
- Configurable backup interval
- Automatic cleanup of old backups
- Backup restoration support

### 📊 Statistics Tracking
- TPS (Ticks Per Second) monitoring
- Memory usage with progress bars
- CPU utilization tracking
- Network bandwidth (in/out)
- Player count and session times
- Event history with timestamps

## 📥 Installation

### Prerequisites
- Go 1.21 or later
- Java 17 or later (for Minecraft server)

### Build from Source

```bash
# Clone the repository
git clone https://github.com/yourusername/mcserver-manager.git
cd mcserver-manager

# Build the binary
go build -o mcserver .

# Or install globally
go install .
```

## 🚀 Usage

### Basic Usage

```bash
# Start a vanilla server with default settings
./mcserver

# Start with custom RAM allocation
./mcserver --ram-min 2G --ram-max 8G

# Start on a different port
./mcserver --port 25566

# Start with a CurseForge modpack
./mcserver --modpack 123456 --ram-max 8G
```

### Command Line Arguments

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--ram-min` | `-m` | `1G` | Minimum RAM allocation |
| `--ram-max` | `-M` | `4G` | Maximum RAM allocation |
| `--port` | `-p` | `25565` | Server port |
| `--server-dir` | `-d` | `./server` | Server directory path |
| `--java` | | `java` | Path to Java executable |
| `--java-args` | | | Additional Java arguments |
| `--modpack` | `-k` | | CurseForge modpack ID or slug |
| `--modpack-version` | | `latest` | Specific modpack version |
| `--auto-restart` | `-r` | `true` | Auto-restart on crash |
| `--backup-enabled` | | `false` | Enable scheduled backups |
| `--backup-interval` | | `60` | Backup interval in minutes |
| `--backup-dir` | | `./backups` | Backup directory path |
| `--max-backups` | | `10` | Maximum backups to keep |
| `--no-tui` | | `false` | Disable TUI, use console mode |

### Examples

```bash
# Full featured server with modpack and backups
./mcserver \
  --modpack 12345 \
  --ram-min 4G \
  --ram-max 12G \
  --port 25565 \
  --backup-enabled \
  --backup-interval 30 \
  --max-backups 20

# Simple server without TUI
./mcserver --no-tui --ram-max 4G

# Development server on alternate port
./mcserver --port 25566 --server-dir ./dev-server
```

## 🎮 TUI Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `Tab` | Toggle command input focus |
| `Enter` | Execute command (when input focused) |
| `R` | Restart server |
| `S` | Start/Stop server |
| `↑/↓` | Scroll console |
| `Q` | Quit application |

## 📦 CurseForge Modpacks

### Using a Modpack

1. Find your modpack on [CurseForge](https://www.curseforge.com/minecraft/modpacks)
2. Get the project ID from the URL or use the modpack name
3. Run with the `--modpack` flag:

```bash
# Using project ID
./mcserver --modpack 123456

# Using modpack name (will search for best match)
./mcserver --modpack "all-the-mods-9"
```

### API Key (Optional)

For better rate limits, set a CurseForge API key:

```bash
export CURSEFORGE_API_KEY=your-api-key
./mcserver --modpack 123456
```

Get an API key at [CurseForge Console](https://console.curseforge.com/).

## 🔧 JVM Optimization

The server manager automatically applies optimized JVM flags based on Aikar's recommendations:

- G1GC garbage collector with tuned settings
- Optimized memory allocation
- Reduced GC pause times
- Better overall performance

Custom flags can be added with `--java-args`:

```bash
./mcserver --java-args "-XX:+UseZGC -Dlog4j2.formatMsgNoLookups=true"
```

## 💾 Backup System

Enable automatic backups with:

```bash
./mcserver \
  --backup-enabled \
  --backup-interval 60 \
  --backup-dir ./my-backups \
  --max-backups 10
```

Backups include:
- World data (overworld, nether, end)
- All dimension folders
- Compressed as ZIP files
- Named with timestamps

## 📊 Statistics Explained

### TPS (Ticks Per Second)
- **20.0**: Perfect performance
- **19.0+**: Excellent
- **17.0-19.0**: Good
- **14.0-17.0**: Moderate lag
- **<14.0**: Severe lag

### Memory Usage
- Shows current heap usage vs maximum
- Color-coded based on usage percentage
- Visual progress bar

### Bandwidth
- Real-time upload/download speeds
- Total data transferred

## 🐛 Troubleshooting

### Server won't start
- Check Java version: `java -version` (needs 17+)
- Verify server JAR exists in server directory
- Check port availability: `netstat -tuln | grep 25565`

### Modpack download fails
- Verify the modpack ID is correct
- Check internet connectivity
- Try setting a CurseForge API key

### High memory usage
- Increase `--ram-max` value
- Reduce view distance in server.properties
- Check for memory leaks in mods

### TPS drops
- Reduce entity counts
- Optimize redstone contraptions
- Check for problematic chunks
- Consider using a performance mod

## 📄 License

MIT License - see LICENSE file for details.

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## 🙏 Acknowledgments

- [Aikar's Flags](https://docs.papermc.io/paper/aikars-flags) for JVM optimization
- [Charm](https://charm.sh/) for the excellent TUI libraries
- [CurseForge](https://www.curseforge.com/) for modpack hosting

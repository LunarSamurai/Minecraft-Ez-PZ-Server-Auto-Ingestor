package stats

import (
	"fmt"
	"time"
)

// FormatBytes formats bytes into a human-readable string
func FormatBytes(bytes uint64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)

	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.2f TB", float64(bytes)/float64(TB))
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// FormatBytesPerSec formats bytes per second into a human-readable string
func FormatBytesPerSec(bytesPerSec float64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytesPerSec >= GB:
		return fmt.Sprintf("%.2f GB/s", bytesPerSec/float64(GB))
	case bytesPerSec >= MB:
		return fmt.Sprintf("%.2f MB/s", bytesPerSec/float64(MB))
	case bytesPerSec >= KB:
		return fmt.Sprintf("%.2f KB/s", bytesPerSec/float64(KB))
	default:
		return fmt.Sprintf("%.0f B/s", bytesPerSec)
	}
}

// FormatDuration formats a duration into a human-readable string
func FormatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm %ds", days, hours, minutes, seconds)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

// FormatDurationShort formats a duration into a compact string
func FormatDurationShort(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd%dh", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh%dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

// FormatPercent formats a percentage with color coding thresholds
func FormatPercent(value float64) string {
	return fmt.Sprintf("%.1f%%", value)
}

// FormatTPS formats TPS with color coding
func FormatTPS(tps float64) string {
	return fmt.Sprintf("%.2f", tps)
}

// TPSColor returns a color based on TPS value
func TPSColor(tps float64) string {
	switch {
	case tps >= 19.0:
		return "#00FF00" // Green - Excellent
	case tps >= 17.0:
		return "#AAFF00" // Yellow-Green - Good
	case tps >= 14.0:
		return "#FFAA00" // Orange - Warning
	case tps >= 10.0:
		return "#FF5500" // Dark Orange - Bad
	default:
		return "#FF0000" // Red - Critical
	}
}

// MemoryColor returns a color based on memory usage percentage
func MemoryColor(usedPercent float64) string {
	switch {
	case usedPercent < 50:
		return "#00FF00" // Green
	case usedPercent < 70:
		return "#AAFF00" // Yellow-Green
	case usedPercent < 85:
		return "#FFAA00" // Orange
	case usedPercent < 95:
		return "#FF5500" // Dark Orange
	default:
		return "#FF0000" // Red
	}
}

// CPUColor returns a color based on CPU usage
func CPUColor(cpuPercent float64) string {
	switch {
	case cpuPercent < 50:
		return "#00FF00"
	case cpuPercent < 70:
		return "#AAFF00"
	case cpuPercent < 85:
		return "#FFAA00"
	case cpuPercent < 95:
		return "#FF5500"
	default:
		return "#FF0000"
	}
}

// ProgressBar creates an ASCII progress bar
func ProgressBar(percent float64, width int) string {
	filled := int(percent / 100 * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}

	bar := ""
	for i := 0; i < width; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += "░"
		}
	}

	return bar
}

// Sparkline creates a simple sparkline from a slice of values
func Sparkline(values []float64, width int) string {
	if len(values) == 0 {
		return ""
	}

	// Find min and max
	min, max := values[0], values[0]
	for _, v := range values {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}

	// Sparkline characters (from low to high)
	chars := []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

	// Sample values to fit width
	result := ""
	step := float64(len(values)) / float64(width)
	if step < 1 {
		step = 1
	}

	for i := 0; i < width && int(float64(i)*step) < len(values); i++ {
		idx := int(float64(i) * step)
		v := values[idx]

		// Normalize to 0-7 range
		var charIdx int
		if max == min {
			charIdx = 4
		} else {
			charIdx = int((v - min) / (max - min) * 7)
		}

		if charIdx < 0 {
			charIdx = 0
		}
		if charIdx > 7 {
			charIdx = 7
		}

		result += string(chars[charIdx])
	}

	return result
}

package backup

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Manager handles world backups
type Manager struct {
	serverDir  string
	backupDir  string
	maxBackups int
}

// BackupInfo holds information about a backup
type BackupInfo struct {
	Name      string
	Path      string
	Size      int64
	CreatedAt time.Time
}

// NewManager creates a new backup manager
func NewManager(serverDir, backupDir string, maxBackups int) *Manager {
	return &Manager{
		serverDir:  serverDir,
		backupDir:  backupDir,
		maxBackups: maxBackups,
	}
}

// CreateBackup creates a backup of the world folders
func (m *Manager) CreateBackup() error {
	// Ensure backup directory exists
	if err := os.MkdirAll(m.backupDir, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Generate backup filename with timestamp
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	backupName := fmt.Sprintf("backup_%s.zip", timestamp)
	backupPath := filepath.Join(m.backupDir, backupName)

	// Find world directories to backup
	worldDirs, err := m.findWorldDirs()
	if err != nil {
		return fmt.Errorf("failed to find world directories: %w", err)
	}

	if len(worldDirs) == 0 {
		return fmt.Errorf("no world directories found to backup")
	}

	// Create the backup zip file
	zipFile, err := os.Create(backupPath)
	if err != nil {
		return fmt.Errorf("failed to create backup file: %w", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// Add each world directory to the backup
	for _, worldDir := range worldDirs {
		if err := m.addDirToZip(zipWriter, worldDir, filepath.Base(worldDir)); err != nil {
			return fmt.Errorf("failed to add %s to backup: %w", worldDir, err)
		}
	}

	// Close the zip writer to finalize
	if err := zipWriter.Close(); err != nil {
		return fmt.Errorf("failed to finalize backup: %w", err)
	}

	// Cleanup old backups
	if err := m.cleanupOldBackups(); err != nil {
		// Log warning but don't fail the backup
		fmt.Printf("Warning: failed to cleanup old backups: %v\n", err)
	}

	return nil
}

// findWorldDirs finds all world directories in the server folder
func (m *Manager) findWorldDirs() ([]string, error) {
	var worldDirs []string

	entries, err := os.ReadDir(m.serverDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()

		// Common world directory names
		if name == "world" ||
			name == "world_nether" ||
			name == "world_the_end" ||
			strings.HasPrefix(name, "world_") ||
			strings.HasPrefix(name, "DIM") {
			worldDirs = append(worldDirs, filepath.Join(m.serverDir, name))
			continue
		}

		// Check if it contains level.dat (is a world folder)
		levelDat := filepath.Join(m.serverDir, name, "level.dat")
		if _, err := os.Stat(levelDat); err == nil {
			worldDirs = append(worldDirs, filepath.Join(m.serverDir, name))
		}
	}

	return worldDirs, nil
}

// addDirToZip recursively adds a directory to a zip archive
func (m *Manager) addDirToZip(zipWriter *zip.Writer, source, prefix string) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Create relative path
		relPath, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}

		zipPath := filepath.Join(prefix, relPath)
		zipPath = strings.ReplaceAll(zipPath, string(os.PathSeparator), "/")

		if info.IsDir() {
			// Add directory entry
			if zipPath != prefix {
				_, err = zipWriter.Create(zipPath + "/")
				return err
			}
			return nil
		}

		// Skip session.lock files as they're always locked
		if strings.HasSuffix(path, "session.lock") {
			return nil
		}

		// Create file header
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = zipPath
		header.Method = zip.Deflate

		// Create file in zip
		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}

		// Copy file contents
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(writer, file)
		return err
	})
}

// cleanupOldBackups removes old backups exceeding maxBackups
func (m *Manager) cleanupOldBackups() error {
	backups, err := m.ListBackups()
	if err != nil {
		return err
	}

	if len(backups) <= m.maxBackups {
		return nil
	}

	// Sort by creation time (newest first)
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt.After(backups[j].CreatedAt)
	})

	// Remove excess backups
	for i := m.maxBackups; i < len(backups); i++ {
		if err := os.Remove(backups[i].Path); err != nil {
			fmt.Printf("Warning: failed to remove old backup %s: %v\n", backups[i].Name, err)
		}
	}

	return nil
}

// ListBackups returns a list of all backups
func (m *Manager) ListBackups() ([]BackupInfo, error) {
	var backups []BackupInfo

	entries, err := os.ReadDir(m.backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return backups, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if !strings.HasPrefix(entry.Name(), "backup_") || !strings.HasSuffix(entry.Name(), ".zip") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		backups = append(backups, BackupInfo{
			Name:      entry.Name(),
			Path:      filepath.Join(m.backupDir, entry.Name()),
			Size:      info.Size(),
			CreatedAt: info.ModTime(),
		})
	}

	return backups, nil
}

// RestoreBackup restores a backup to the server directory
func (m *Manager) RestoreBackup(backupPath string) error {
	// Open the backup zip file
	r, err := zip.OpenReader(backupPath)
	if err != nil {
		return fmt.Errorf("failed to open backup: %w", err)
	}
	defer r.Close()

	// Extract all files
	for _, f := range r.File {
		destPath := filepath.Join(m.serverDir, f.Name)

		if f.FileInfo().IsDir() {
			os.MkdirAll(destPath, 0755)
			continue
		}

		// Create parent directories
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}

		// Extract file
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("failed to open file in archive: %w", err)
		}

		outFile, err := os.Create(destPath)
		if err != nil {
			rc.Close()
			return fmt.Errorf("failed to create file: %w", err)
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()

		if err != nil {
			return fmt.Errorf("failed to extract file: %w", err)
		}
	}

	return nil
}

// GetTotalBackupSize returns the total size of all backups
func (m *Manager) GetTotalBackupSize() (int64, error) {
	backups, err := m.ListBackups()
	if err != nil {
		return 0, err
	}

	var total int64
	for _, b := range backups {
		total += b.Size
	}

	return total, nil
}

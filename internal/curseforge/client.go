package curseforge

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	// CurseForge API endpoints
	cfAPIBase = "https://api.curseforge.com"
	cfCDNBase = "https://edge.forgecdn.net/files"

	// Minecraft game ID on CurseForge
	minecraftGameID = 432

	// Modpack class ID
	modpackClassID = 4471
)

// Client handles CurseForge API interactions
type Client struct {
	httpClient *http.Client
	apiKey     string
}

// Modpack represents a CurseForge modpack
type Modpack struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Slug          string `json:"slug"`
	Summary       string `json:"summary"`
	LatestFileID  int    `json:"mainFileId"`
	DownloadCount int    `json:"downloadCount"`
}

// ModpackFile represents a specific version of a modpack
type ModpackFile struct {
	ID           int    `json:"id"`
	DisplayName  string `json:"displayName"`
	FileName     string `json:"fileName"`
	DownloadURL  string `json:"downloadUrl"`
	FileLength   int64  `json:"fileLength"`
	ServerPackID int    `json:"serverPackFileId"`
}

// ModpackManifest is the manifest.json inside a modpack
type ModpackManifest struct {
	Minecraft struct {
		Version    string `json:"version"`
		ModLoaders []struct {
			ID      string `json:"id"`
			Primary bool   `json:"primary"`
		} `json:"modLoaders"`
	} `json:"minecraft"`
	ManifestType    string `json:"manifestType"`
	ManifestVersion int    `json:"manifestVersion"`
	Name            string `json:"name"`
	Version         string `json:"version"`
	Author          string `json:"author"`
	Files           []struct {
		ProjectID int  `json:"projectID"`
		FileID    int  `json:"fileID"`
		Required  bool `json:"required"`
	} `json:"files"`
	Overrides string `json:"overrides"`
}

// NewClient creates a new CurseForge client
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{},
		apiKey:     os.Getenv("CURSEFORGE_API_KEY"),
	}
}

// NewClientWithKey creates a new CurseForge client with an API key
func NewClientWithKey(apiKey string) *Client {
	return &Client{
		httpClient: &http.Client{},
		apiKey:     apiKey,
	}
}

// SearchModpack searches for a modpack by name or ID
func (c *Client) SearchModpack(query string) (*Modpack, error) {
	// Try to parse as project ID first
	if projectID, err := strconv.Atoi(query); err == nil {
		return c.GetModpack(projectID)
	}

	// Search by name/slug
	url := fmt.Sprintf("%s/v1/mods/search?gameId=%d&classId=%d&searchFilter=%s&sortField=2&sortOrder=desc",
		cfAPIBase, minecraftGameID, modpackClassID, query)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	if c.apiKey != "" {
		req.Header.Set("x-api-key", c.apiKey)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to search modpacks: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CurseForge API returned status %d", resp.StatusCode)
	}

	var result struct {
		Data []Modpack `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("no modpack found for query: %s", query)
	}

	return &result.Data[0], nil
}

// GetModpack gets a modpack by project ID
func (c *Client) GetModpack(projectID int) (*Modpack, error) {
	url := fmt.Sprintf("%s/v1/mods/%d", cfAPIBase, projectID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	if c.apiKey != "" {
		req.Header.Set("x-api-key", c.apiKey)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get modpack: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CurseForge API returned status %d", resp.StatusCode)
	}

	var result struct {
		Data Modpack `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result.Data, nil
}

// GetModpackFile gets information about a specific modpack file
func (c *Client) GetModpackFile(projectID, fileID int) (*ModpackFile, error) {
	url := fmt.Sprintf("%s/v1/mods/%d/files/%d", cfAPIBase, projectID, fileID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	if c.apiKey != "" {
		req.Header.Set("x-api-key", c.apiKey)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get modpack file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CurseForge API returned status %d", resp.StatusCode)
	}

	var result struct {
		Data ModpackFile `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result.Data, nil
}

// GetLatestServerPack gets the latest server pack for a modpack
func (c *Client) GetLatestServerPack(projectID int) (*ModpackFile, error) {
	url := fmt.Sprintf("%s/v1/mods/%d/files?gameVersionTypeId=0", cfAPIBase, projectID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	if c.apiKey != "" {
		req.Header.Set("x-api-key", c.apiKey)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get modpack files: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CurseForge API returned status %d", resp.StatusCode)
	}

	var result struct {
		Data []ModpackFile `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Find the first file with a server pack
	for _, file := range result.Data {
		if file.ServerPackID > 0 {
			return c.GetModpackFile(projectID, file.ServerPackID)
		}
	}

	// Fall back to the first file
	if len(result.Data) > 0 {
		return &result.Data[0], nil
	}

	return nil, fmt.Errorf("no files found for modpack %d", projectID)
}

// DownloadModpack downloads a modpack to the specified directory
func (c *Client) DownloadModpack(modpackQuery, version, destDir string) (string, error) {
	modpack, err := c.SearchModpack(modpackQuery)
	if err != nil {
		return "", fmt.Errorf("failed to find modpack: %w", err)
	}

	var file *ModpackFile

	if version == "latest" || version == "" {
		file, err = c.GetLatestServerPack(modpack.ID)
	} else {
		fileID, parseErr := strconv.Atoi(version)
		if parseErr != nil {
			return "", fmt.Errorf("invalid version ID: %s", version)
		}
		file, err = c.GetModpackFile(modpack.ID, fileID)
	}

	if err != nil {
		return "", fmt.Errorf("failed to get modpack file: %w", err)
	}

	// Determine download URL
	downloadURL := file.DownloadURL
	if downloadURL == "" {
		// Construct CDN URL from file ID
		idStr := strconv.Itoa(file.ID)
		part1 := idStr[:4]
		part2 := strings.TrimLeft(idStr[4:], "0")
		if part2 == "" {
			part2 = "0"
		}
		downloadURL = fmt.Sprintf("%s/%s/%s/%s", cfCDNBase, part1, part2, file.FileName)
	}

	// Create destination directory
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Download the file
	destPath := filepath.Join(destDir, file.FileName)

	resp, err := http.Get(downloadURL)
	if err != nil {
		return "", fmt.Errorf("failed to download modpack: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return destPath, nil
}

// InstallModpack extracts and installs a modpack
func (c *Client) InstallModpack(modpackPath, destDir string) error {
	// Open the zip file
	r, err := zip.OpenReader(modpackPath)
	if err != nil {
		return fmt.Errorf("failed to open modpack: %w", err)
	}
	defer r.Close()

	var manifest *ModpackManifest

	// First pass: find and parse manifest
	for _, f := range r.File {
		if f.Name == "manifest.json" {
			rc, err := f.Open()
			if err != nil {
				return fmt.Errorf("failed to open manifest: %w", err)
			}

			manifest = &ModpackManifest{}
			err = json.NewDecoder(rc).Decode(manifest)
			rc.Close()

			if err != nil {
				return fmt.Errorf("failed to parse manifest: %w", err)
			}
			break
		}
	}

	// Extract all files
	for _, f := range r.File {
		destPath := filepath.Join(destDir, f.Name)

		// Handle overrides directory specially
		if manifest != nil && manifest.Overrides != "" {
			if strings.HasPrefix(f.Name, manifest.Overrides+"/") {
				destPath = filepath.Join(destDir, strings.TrimPrefix(f.Name, manifest.Overrides+"/"))
			}
		}

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

	// Download mods if manifest exists
	if manifest != nil {
		modsDir := filepath.Join(destDir, "mods")
		os.MkdirAll(modsDir, 0755)

		for _, mod := range manifest.Files {
			if err := c.downloadMod(mod.ProjectID, mod.FileID, modsDir); err != nil {
				// Log error but continue
				fmt.Printf("Warning: failed to download mod %d: %v\n", mod.ProjectID, err)
			}
		}

		// Install mod loader if specified
		if len(manifest.Minecraft.ModLoaders) > 0 {
			for _, loader := range manifest.Minecraft.ModLoaders {
				if loader.Primary {
					if err := c.installModLoader(loader.ID, manifest.Minecraft.Version, destDir); err != nil {
						fmt.Printf("Warning: failed to install mod loader %s: %v\n", loader.ID, err)
					}
					break
				}
			}
		}
	}

	return nil
}

// downloadMod downloads a specific mod
func (c *Client) downloadMod(projectID, fileID int, destDir string) error {
	file, err := c.GetModpackFile(projectID, fileID)
	if err != nil {
		return err
	}

	downloadURL := file.DownloadURL
	if downloadURL == "" {
		idStr := strconv.Itoa(file.ID)
		part1 := idStr[:4]
		part2 := strings.TrimLeft(idStr[4:], "0")
		if part2 == "" {
			part2 = "0"
		}
		downloadURL = fmt.Sprintf("%s/%s/%s/%s", cfCDNBase, part1, part2, file.FileName)
	}

	resp, err := http.Get(downloadURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	destPath := filepath.Join(destDir, file.FileName)
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

// installModLoader installs Forge or Fabric
func (c *Client) installModLoader(loaderID, mcVersion, destDir string) error {
	parts := strings.Split(loaderID, "-")
	if len(parts) < 2 {
		return fmt.Errorf("invalid loader ID: %s", loaderID)
	}

	loaderType := parts[0]
	loaderVersion := strings.Join(parts[1:], "-")

	switch loaderType {
	case "forge":
		return c.installForge(mcVersion, loaderVersion, destDir)
	case "fabric":
		return c.installFabric(mcVersion, loaderVersion, destDir)
	case "neoforge":
		return c.installNeoForge(mcVersion, loaderVersion, destDir)
	default:
		return fmt.Errorf("unsupported mod loader: %s", loaderType)
	}
}

// installForge downloads and installs Forge
func (c *Client) installForge(mcVersion, forgeVersion, destDir string) error {
	// Download Forge installer
	installerURL := fmt.Sprintf(
		"https://maven.minecraftforge.net/net/minecraftforge/forge/%s-%s/forge-%s-%s-installer.jar",
		mcVersion, forgeVersion, mcVersion, forgeVersion,
	)

	installerPath := filepath.Join(destDir, "forge-installer.jar")

	resp, err := http.Get(installerURL)
	if err != nil {
		return fmt.Errorf("failed to download Forge installer: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Forge installer download returned status %d", resp.StatusCode)
	}

	out, err := os.Create(installerPath)
	if err != nil {
		return err
	}

	_, err = io.Copy(out, resp.Body)
	out.Close()

	if err != nil {
		return err
	}

	// Note: Running the installer requires Java, which would need to be done separately
	// For now, we just download the installer
	fmt.Printf("Forge installer downloaded to: %s\n", installerPath)
	fmt.Printf("Run: java -jar %s --installServer\n", installerPath)

	return nil
}

// installFabric downloads and installs Fabric
func (c *Client) installFabric(mcVersion, fabricVersion, destDir string) error {
	// Download Fabric server launcher
	serverURL := fmt.Sprintf(
		"https://meta.fabricmc.net/v2/versions/loader/%s/%s/stable/server/jar",
		mcVersion, fabricVersion,
	)

	serverPath := filepath.Join(destDir, "fabric-server.jar")

	resp, err := http.Get(serverURL)
	if err != nil {
		return fmt.Errorf("failed to download Fabric server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Fabric server download returned status %d", resp.StatusCode)
	}

	out, err := os.Create(serverPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

// installNeoForge downloads and installs NeoForge
func (c *Client) installNeoForge(mcVersion, neoVersion, destDir string) error {
	// Download NeoForge installer
	installerURL := fmt.Sprintf(
		"https://maven.neoforged.net/releases/net/neoforged/neoforge/%s/neoforge-%s-installer.jar",
		neoVersion, neoVersion,
	)

	installerPath := filepath.Join(destDir, "neoforge-installer.jar")

	resp, err := http.Get(installerURL)
	if err != nil {
		return fmt.Errorf("failed to download NeoForge installer: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("NeoForge installer download returned status %d", resp.StatusCode)
	}

	out, err := os.Create(installerPath)
	if err != nil {
		return err
	}

	_, err = io.Copy(out, resp.Body)
	out.Close()

	if err != nil {
		return err
	}

	fmt.Printf("NeoForge installer downloaded to: %s\n", installerPath)
	fmt.Printf("Run: java -jar %s --installServer\n", installerPath)

	return nil
}

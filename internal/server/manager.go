package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"minecraft-server-manager/internal/config"
	"minecraft-server-manager/internal/github"

	"github.com/sirupsen/logrus"
)

type Manager struct {
	config        *config.Config
	logger        *logrus.Logger
	servers       map[string]*MinecraftServer
	mu            sync.RWMutex
	lastConfig    *config.RepoConfig
	lastCommitSHA string
	bedrockPath   string
}

type MinecraftServer struct {
	Config    *config.MinecraftServerConfig
	Process   *exec.Cmd
	Status    string
	StartTime time.Time
	Port      int
	Logs      []string
	MaxLogs   int
}

type ServerStatus struct {
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	Port        int       `json:"port"`
	StartTime   time.Time `json:"start_time"`
	Uptime      string    `json:"uptime"`
	PlayerCount int       `json:"player_count"`
}

type ManagerStatus struct {
	TotalServers int            `json:"total_servers"`
	Running      int            `json:"running"`
	Stopped      int            `json:"stopped"`
	Servers      []ServerStatus `json:"servers"`
	LastUpdate   time.Time      `json:"last_update"`
	BedrockPath  string         `json:"bedrock_path"`
}

type WhitelistEntry struct {
	Name string `json:"name"`
	XUID string `json:"xuid"`
}

type PermissionsEntry struct {
	Name       string `json:"name"`
	XUID       string `json:"xuid"`
	Permission string `json:"permission"`
}

func NewManager(cfg *config.Config, logger *logrus.Logger) *Manager {
	return &Manager{
		config:  cfg,
		logger:  logger,
		servers: make(map[string]*MinecraftServer),
	}
}

func (m *Manager) Start(ctx context.Context, githubClient *github.Client) {
	m.logger.Info("Starting Minecraft Bedrock server manager")

	// Initialize Bedrock server
	if err := m.initializeBedrockServer(); err != nil {
		m.logger.Errorf("Failed to initialize Bedrock server: %v", err)
		return
	}

	// Set GitHub client configuration
	githubClient.SetBranch(m.config.GitHub.Branch)
	githubClient.SetConfigPath(m.config.GitHub.ConfigPath)

	ticker := time.NewTicker(time.Duration(m.config.GitHub.PollInterval) * time.Second)
	defer ticker.Stop()

	// Initial configuration load
	m.pollConfiguration(githubClient)

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("Shutting down server manager")
			m.stopAllServers()
			return
		case <-ticker.C:
			m.pollConfiguration(githubClient)
		}
	}
}

func (m *Manager) initializeBedrockServer() error {
	versionsDir := "versions"
	bedrockArchive := filepath.Join(versionsDir, "bedrock-server.zip")

	// Check if versions/bedrock-server.zip exists
	if _, err := os.Stat(bedrockArchive); err != nil {
		if os.IsNotExist(err) {
			m.logger.Info("No Bedrock server archive found in versions/bedrock-server.zip, using configured path")
			m.bedrockPath = m.config.Server.BedrockPath
			return nil
		}
		return fmt.Errorf("failed to check Bedrock server archive: %w", err)
	}

	m.logger.Info("Found Bedrock server archive (bedrock-server.zip), processing...")

	// Remove existing layer files and extracted directory
	if err := m.cleanupLayers(); err != nil {
		return fmt.Errorf("failed to cleanup existing files: %w", err)
	}

	// Split the archive into 10 layers
	if err := m.splitArchive(bedrockArchive); err != nil {
		return fmt.Errorf("failed to split archive: %w", err)
	}

	// Recombine the layers
	if err := m.recombineLayers(); err != nil {
		return fmt.Errorf("failed to recombine layers: %w", err)
	}

	// Extract the archive
	if err := m.extractArchive(); err != nil {
		return fmt.Errorf("failed to extract archive: %w", err)
	}

	// Set the Bedrock path to the extracted executable
	m.bedrockPath = "./bedrock-server-extracted/bedrock_server"
	m.logger.Infof("Bedrock server initialized at: %s", m.bedrockPath)

	return nil
}

func (m *Manager) cleanupLayers() error {
	// Remove existing layer files
	for i := 0; i < 10; i++ {
		layerFile := fmt.Sprintf("versions/bedrock-server.layer.%d", i)
		if err := os.Remove(layerFile); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove layer file %s: %w", layerFile, err)
		}
	}

	// Remove extracted directory
	if err := os.RemoveAll("bedrock-server-extracted"); err != nil {
		return fmt.Errorf("failed to remove extracted directory: %w", err)
	}

	// Remove recombined archive
	if err := os.Remove("versions/bedrock-server-recombined.zip"); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove recombined archive: %w", err)
	}

	return nil
}

func (m *Manager) splitArchive(archivePath string) error {
	// Open the archive file
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer file.Close()

	// Get file size
	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file stats: %w", err)
	}
	fileSize := stat.Size()

	// Calculate layer size
	layerSize := fileSize / 10
	remainder := fileSize % 10

	m.logger.Infof("Splitting archive into 10 layers (file size: %d bytes, layer size: %d bytes)", fileSize, layerSize)

	// Create layers directory if it doesn't exist
	if err := os.MkdirAll("versions", 0755); err != nil {
		return fmt.Errorf("failed to create versions directory: %w", err)
	}

	// Split the file into 10 layers
	for i := 0; i < 10; i++ {
		layerFile := fmt.Sprintf("versions/bedrock-server.layer.%d", i)

		// Calculate actual layer size (last layer gets the remainder)
		actualLayerSize := layerSize
		if i == 9 {
			actualLayerSize += remainder
		}

		// Create layer file
		layer, err := os.Create(layerFile)
		if err != nil {
			return fmt.Errorf("failed to create layer file %s: %w", layerFile, err)
		}

		// Copy data to layer
		written, err := io.CopyN(layer, file, actualLayerSize)
		if err != nil && err != io.EOF {
			layer.Close()
			return fmt.Errorf("failed to write layer %d: %w", i, err)
		}

		layer.Close()
		m.logger.Infof("Created layer %d: %s (%d bytes)", i, layerFile, written)
	}

	return nil
}

func (m *Manager) recombineLayers() error {
	m.logger.Info("Recombining layers...")

	// Create recombined file
	recombinedFile := "versions/bedrock-server-recombined.zip"
	output, err := os.Create(recombinedFile)
	if err != nil {
		return fmt.Errorf("failed to create recombined file: %w", err)
	}
	defer output.Close()

	// Combine all layers
	for i := 0; i < 10; i++ {
		layerFile := fmt.Sprintf("versions/bedrock-server.layer.%d", i)

		// Check if layer file exists
		if _, err := os.Stat(layerFile); err != nil {
			return fmt.Errorf("layer file %s not found: %w", layerFile, err)
		}

		// Open layer file
		layer, err := os.Open(layerFile)
		if err != nil {
			return fmt.Errorf("failed to open layer file %s: %w", layerFile, err)
		}

		// Copy layer data to recombined file
		written, err := io.Copy(output, layer)
		if err != nil {
			layer.Close()
			return fmt.Errorf("failed to copy layer %d: %w", i, err)
		}

		layer.Close()
		m.logger.Infof("Added layer %d to recombined file (%d bytes)", i, written)
	}

	// Verify file integrity
	if err := m.verifyIntegrity(); err != nil {
		return fmt.Errorf("integrity check failed: %w", err)
	}

	m.logger.Info("Layers recombined successfully")
	return nil
}

func (m *Manager) verifyIntegrity() error {
	originalFile := "versions/bedrock-server.zip"
	recombinedFile := "versions/bedrock-server-recombined.zip"

	// Calculate SHA256 of original file
	originalHash, err := m.calculateFileHash(originalFile)
	if err != nil {
		return fmt.Errorf("failed to calculate original file hash: %w", err)
	}

	// Calculate SHA256 of recombined file
	recombinedHash, err := m.calculateFileHash(recombinedFile)
	if err != nil {
		return fmt.Errorf("failed to calculate recombined file hash: %w", err)
	}

	// Compare hashes
	if originalHash != recombinedHash {
		return fmt.Errorf("integrity check failed: hashes don't match (original: %s, recombined: %s)", originalHash, recombinedHash)
	}

	m.logger.Infof("Integrity check passed: %s", originalHash)
	return nil
}

func (m *Manager) calculateFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (m *Manager) extractArchive() error {
	m.logger.Info("Extracting Bedrock server archive...")

	// Create extraction directory
	extractDir := "bedrock-server-extracted"
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return fmt.Errorf("failed to create extraction directory: %w", err)
	}

	// Determine archive type and extract
	archivePath := "versions/bedrock-server-recombined.zip"

	// Since we know it's a zip file, try unzip first
	m.logger.Info("Extracting zip archive...")
	cmd := exec.Command("unzip", "-o", archivePath, "-d", extractDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		// If unzip fails, try tar.gz as fallback
		m.logger.Info("zip extraction failed, trying tar.gz...")
		cmd = exec.Command("tar", "-xzf", archivePath, "-C", extractDir)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to extract archive (tried zip and tar.gz): %w", err)
		}
	}

	// Look for the bedrock_server executable
	bedrockExecutable := filepath.Join(extractDir, "bedrock_server")
	if _, err := os.Stat(bedrockExecutable); err != nil {
		// Try to find it recursively
		found, err := m.findBedrockExecutable(extractDir)
		if err != nil {
			return fmt.Errorf("failed to find bedrock_server executable: %w", err)
		}
		bedrockExecutable = found
	}

	// Make it executable
	if err := os.Chmod(bedrockExecutable, 0755); err != nil {
		return fmt.Errorf("failed to make bedrock_server executable: %w", err)
	}

	m.logger.Infof("Bedrock server extracted to: %s", bedrockExecutable)
	return nil
}

func (m *Manager) findBedrockExecutable(dir string) (string, error) {
	var found string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && info.Name() == "bedrock_server" {
			found = path
			return filepath.SkipAll
		}
		return nil
	})

	if err != nil {
		return "", err
	}

	if found == "" {
		return "", fmt.Errorf("bedrock_server executable not found in extracted directory")
	}

	return found, nil
}

func (m *Manager) pollConfiguration(githubClient *github.Client) {
	// Check if there are any changes
	commitSHA, err := githubClient.GetLastCommitSHA()
	if err != nil {
		m.logger.Errorf("Failed to get last commit SHA: %v", err)
		return
	}

	// If no changes, skip
	if commitSHA == m.lastCommitSHA {
		return
	}

	m.logger.Infof("Configuration changed, updating servers (commit: %s)", commitSHA[:8])

	// Get new configuration
	repoConfig, err := githubClient.GetConfig()
	if err != nil {
		m.logger.Errorf("Failed to get configuration from GitHub: %v", err)
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Update servers based on new configuration
	m.updateServers(repoConfig)
	m.lastConfig = repoConfig
	m.lastCommitSHA = commitSHA
}

func (m *Manager) updateServers(repoConfig *config.RepoConfig) {
	// Stop servers that are no longer in configuration
	for name := range m.servers {
		found := false
		for _, serverConfig := range repoConfig.Servers {
			if serverConfig.Name == name {
				found = true
				break
			}
		}
		if !found {
			m.logger.Infof("Stopping server %s (no longer in configuration)", name)
			m.stopServer(name)
		}
	}

	// Start/update servers from configuration
	for _, serverConfig := range repoConfig.Servers {
		if len(m.servers) >= m.config.Server.MaxInstances {
			m.logger.Warnf("Maximum number of servers reached (%d), skipping %s", m.config.Server.MaxInstances, serverConfig.Name)
			continue
		}

		existingServer, exists := m.servers[serverConfig.Name]
		if exists {
			// Update existing server if configuration changed
			if m.serverConfigChanged(existingServer.Config, &serverConfig) {
				m.logger.Infof("Restarting server %s (configuration changed)", serverConfig.Name)
				m.stopServer(serverConfig.Name)
				m.startServer(&serverConfig)
			}
		} else {
			// Start new server
			m.logger.Infof("Starting new server %s", serverConfig.Name)
			m.startServer(&serverConfig)
		}
	}
}

func (m *Manager) serverConfigChanged(old, new *config.MinecraftServerConfig) bool {
	// Simple comparison - in a real implementation, you might want more sophisticated diffing
	return old.Port != new.Port || old.Version != new.Version || old.WorldName != new.WorldName
}

func (m *Manager) startServer(serverConfig *config.MinecraftServerConfig) {
	serverDir := m.config.GetServerDir(serverConfig.Name)

	// Create server directory
	if err := os.MkdirAll(serverDir, 0755); err != nil {
		m.logger.Errorf("Failed to create server directory for %s: %v", serverConfig.Name, err)
		return
	}

	// Check if Bedrock server executable exists
	if err := m.checkBedrockServer(serverConfig.Version); err != nil {
		m.logger.Errorf("Failed to check Bedrock server for %s: %v", serverConfig.Name, err)
		return
	}

	// Create server.properties
	propertiesPath := m.config.GetServerPropertiesPath(serverConfig.Name)
	if err := m.createServerProperties(serverConfig, propertiesPath); err != nil {
		m.logger.Errorf("Failed to create server.properties for %s: %v", serverConfig.Name, err)
		return
	}

	// Create permissions.json
	permissionsPath := m.config.GetPermissionsPath(serverConfig.Name)
	if err := m.createPermissionsFile(serverConfig, permissionsPath); err != nil {
		m.logger.Errorf("Failed to create permissions.json for %s: %v", serverConfig.Name, err)
		return
	}

	// Create whitelist.json
	whitelistPath := m.config.GetWhitelistPath(serverConfig.Name)
	if err := m.createWhitelistFile(serverConfig, whitelistPath); err != nil {
		m.logger.Errorf("Failed to create whitelist.json for %s: %v", serverConfig.Name, err)
		return
	}

	// Start the server process
	cmd := exec.Command(m.bedrockPath,
		"-port", strconv.Itoa(serverConfig.Port),
		"-worldsdir", serverDir,
		"-world", serverConfig.WorldName,
		"-logpath", filepath.Join(serverDir, "logs"))

	cmd.Dir = serverDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		m.logger.Errorf("Failed to start server %s: %v", serverConfig.Name, err)
		return
	}

	server := &MinecraftServer{
		Config:    serverConfig,
		Process:   cmd,
		Status:    "starting",
		StartTime: time.Now(),
		Port:      serverConfig.Port,
		MaxLogs:   100,
	}

	m.servers[serverConfig.Name] = server

	// Monitor the process
	go m.monitorServer(serverConfig.Name, cmd)

	m.logger.Infof("Server %s started on port %d", serverConfig.Name, serverConfig.Port)
}

func (m *Manager) stopServer(name string) {
	server, exists := m.servers[name]
	if !exists {
		return
	}

	if server.Process != nil && server.Process.Process != nil {
		server.Process.Process.Kill()
		server.Process.Wait()
	}

	delete(m.servers, name)
	m.logger.Infof("Server %s stopped", name)
}

func (m *Manager) stopAllServers() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name := range m.servers {
		m.stopServer(name)
	}
}

func (m *Manager) monitorServer(name string, cmd *exec.Cmd) {
	err := cmd.Wait()

	m.mu.Lock()
	defer m.mu.Unlock()

	if server, exists := m.servers[name]; exists {
		if err != nil {
			server.Status = "crashed"
			m.logger.Errorf("Server %s crashed: %v", name, err)
		} else {
			server.Status = "stopped"
			m.logger.Infof("Server %s stopped", name)
		}
	}
}

func (m *Manager) checkBedrockServer(version string) error {
	// Check if Bedrock server executable exists
	if _, err := os.Stat(m.bedrockPath); err != nil {
		return fmt.Errorf("Bedrock server executable not found at %s", m.bedrockPath)
	}
	return nil
}

func (m *Manager) createServerProperties(serverConfig *config.MinecraftServerConfig, propertiesPath string) error {
	properties := map[string]string{
		"server-port":                              strconv.Itoa(serverConfig.Port),
		"gamemode":                                 serverConfig.Gamemode,
		"difficulty":                               serverConfig.Difficulty,
		"max-players":                              strconv.Itoa(serverConfig.MaxPlayers),
		"online-mode":                              strconv.FormatBool(serverConfig.OnlineMode),
		"allow-cheats":                             "false",
		"server-name":                              serverConfig.Name,
		"level-name":                               serverConfig.WorldName,
		"level-seed":                               serverConfig.LevelSeed,
		"level-type":                               serverConfig.LevelType,
		"default-player-permission-level":          serverConfig.DefaultPlayerPermissionLevel,
		"content-log-file-enabled":                 strconv.FormatBool(serverConfig.ContentLogFileEnabled),
		"enable-scripts":                           strconv.FormatBool(serverConfig.EnableScripts),
		"enable-command-blocking":                  strconv.FormatBool(serverConfig.EnableCommandBlocking),
		"max-threads":                              strconv.Itoa(serverConfig.MaxThreads),
		"player-idle-timeout":                      strconv.Itoa(serverConfig.PlayerIdleTimeout),
		"max-world-size":                           strconv.Itoa(serverConfig.MaxWorldSize),
		"server-authoritative-movement":            "server-auth",
		"player-movement-score-threshold":          "20",
		"player-movement-distance-threshold":       "0.3",
		"player-movement-duration-threshold-in-ms": "500",
		"correct-player-movement":                  "true",
	}

	// Add custom properties
	for key, value := range serverConfig.Properties {
		properties[key] = value
	}

	// Write properties file
	var content strings.Builder
	for key, value := range properties {
		content.WriteString(key + "=" + value + "\n")
	}

	return os.WriteFile(propertiesPath, []byte(content.String()), 0644)
}

func (m *Manager) createPermissionsFile(serverConfig *config.MinecraftServerConfig, permissionsPath string) error {
	var permissions []PermissionsEntry

	// Add operators
	for _, op := range serverConfig.Ops {
		permissions = append(permissions, PermissionsEntry{
			Name:       op,
			XUID:       "", // XUID would need to be looked up
			Permission: "operator",
		})
	}

	// Add whitelisted players with member permissions
	for _, player := range serverConfig.Whitelist {
		permissions = append(permissions, PermissionsEntry{
			Name:       player,
			XUID:       "", // XUID would need to be looked up
			Permission: "member",
		})
	}

	data, err := json.MarshalIndent(permissions, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(permissionsPath, data, 0644)
}

func (m *Manager) createWhitelistFile(serverConfig *config.MinecraftServerConfig, whitelistPath string) error {
	var whitelist []WhitelistEntry

	for _, player := range serverConfig.Whitelist {
		whitelist = append(whitelist, WhitelistEntry{
			Name: player,
			XUID: "", // XUID would need to be looked up
		})
	}

	data, err := json.MarshalIndent(whitelist, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(whitelistPath, data, 0644)
}

func (m *Manager) GetStatus() ManagerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := ManagerStatus{
		TotalServers: len(m.servers),
		LastUpdate:   time.Now(),
		BedrockPath:  m.bedrockPath,
	}

	for name, server := range m.servers {
		uptime := time.Since(server.StartTime)
		serverStatus := ServerStatus{
			Name:      name,
			Status:    server.Status,
			Port:      server.Port,
			StartTime: server.StartTime,
			Uptime:    uptime.String(),
		}

		if server.Status == "running" {
			status.Running++
		} else {
			status.Stopped++
		}

		status.Servers = append(status.Servers, serverStatus)
	}

	return status
}

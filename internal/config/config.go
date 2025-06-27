package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	GitHub GitHubConfig `yaml:"github"`
	HTTP   HTTPConfig   `yaml:"http"`
	Server ServerConfig `yaml:"server"`
}

type GitHubConfig struct {
	RepoOwner    string `yaml:"repo_owner"`
	RepoName     string `yaml:"repo_name"`
	Branch       string `yaml:"branch"`
	ConfigPath   string `yaml:"config_path"`
	PollInterval int    `yaml:"poll_interval"`
}

type HTTPConfig struct {
	Port int `yaml:"port"`
}

type ServerConfig struct {
	BaseDir      string `yaml:"base_dir"`
	MaxInstances int    `yaml:"max_instances"`
	BedrockPath  string `yaml:"bedrock_path"`
	MemoryLimit  string `yaml:"memory_limit"`
}

type MinecraftServerConfig struct {
	Name                         string            `yaml:"name"`
	Port                         int               `yaml:"port"`
	Version                      string            `yaml:"version"`
	Properties                   map[string]string `yaml:"properties"`
	WorldName                    string            `yaml:"world_name"`
	Seed                         string            `yaml:"seed"`
	Gamemode                     string            `yaml:"gamemode"`
	Difficulty                   string            `yaml:"difficulty"`
	MaxPlayers                   int               `yaml:"max_players"`
	OnlineMode                   bool              `yaml:"online_mode"`
	PvP                          bool              `yaml:"pvp"`
	AllowFlight                  bool              `yaml:"allow_flight"`
	Motd                         string            `yaml:"motd"`
	Whitelist                    []string          `yaml:"whitelist"`
	Ops                          []string          `yaml:"ops"`
	LevelType                    string            `yaml:"level_type"`
	LevelSeed                    string            `yaml:"level_seed"`
	DefaultPlayerPermissionLevel string            `yaml:"default_player_permission_level"`
	ContentLogFileEnabled        bool              `yaml:"content_log_file_enabled"`
	EnableScripts                bool              `yaml:"enable_scripts"`
	EnableCommandBlocking        bool              `yaml:"enable_command_blocking"`
	MaxThreads                   int               `yaml:"max_threads"`
	PlayerIdleTimeout            int               `yaml:"player_idle_timeout"`
	MaxWorldSize                 int               `yaml:"max_world_size"`
}

type RepoConfig struct {
	Servers []MinecraftServerConfig `yaml:"servers"`
}

// readBranchFile reads the branch from the branch file in the root directory
func readBranchFile() (string, error) {
	// Look for branch file in current directory
	branchFile := "branch"

	data, err := os.ReadFile(branchFile)
	if err != nil {
		// If branch file doesn't exist, return empty string (will use default)
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read branch file: %w", err)
	}

	// Trim whitespace and newlines
	branch := strings.TrimSpace(string(data))
	return branch, nil
}

func Load() (*Config, error) {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config.yaml"
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Read branch from branch file (takes precedence over config.yaml)
	branchFromFile, err := readBranchFile()
	if err != nil {
		return nil, fmt.Errorf("failed to read branch file: %w", err)
	}

	// Set defaults
	if branchFromFile != "" {
		config.GitHub.Branch = branchFromFile
	} else if config.GitHub.Branch == "" {
		config.GitHub.Branch = "main"
	}

	if config.GitHub.ConfigPath == "" {
		config.GitHub.ConfigPath = "servers.yaml"
	}
	if config.GitHub.PollInterval == 0 {
		config.GitHub.PollInterval = 60 // 60 seconds
	}
	if config.HTTP.Port == 0 {
		config.HTTP.Port = 8080
	}
	if config.Server.BaseDir == "" {
		config.Server.BaseDir = "./servers"
	}
	if config.Server.MaxInstances == 0 {
		config.Server.MaxInstances = 5
	}
	if config.Server.BedrockPath == "" {
		config.Server.BedrockPath = "./bedrock_server"
	}
	if config.Server.MemoryLimit == "" {
		config.Server.MemoryLimit = "1G"
	}

	return &config, nil
}

func (c *Config) GetServerDir(serverName string) string {
	return filepath.Join(c.Server.BaseDir, serverName)
}

func (c *Config) GetServerPropertiesPath(serverName string) string {
	return filepath.Join(c.GetServerDir(serverName), "server.properties")
}

func (c *Config) GetPermissionsPath(serverName string) string {
	return filepath.Join(c.GetServerDir(serverName), "permissions.json")
}

func (c *Config) GetWhitelistPath(serverName string) string {
	return filepath.Join(c.GetServerDir(serverName), "whitelist.json")
}

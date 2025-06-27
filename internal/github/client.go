package github

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"minecraft-server-manager/internal/config"

	"github.com/google/go-github/v57/github"
	"gopkg.in/yaml.v3"
)

type Client struct {
	client     *github.Client
	repoOwner  string
	repoName   string
	branch     string
	configPath string
}

func NewClient(repoOwner, repoName string) *Client {
	// For public repositories, we don't need authentication
	client := github.NewClient(nil)

	return &Client{
		client:     client,
		repoOwner:  repoOwner,
		repoName:   repoName,
		branch:     "main",
		configPath: "servers.yaml",
	}
}

func (c *Client) SetBranch(branch string) {
	c.branch = branch
}

func (c *Client) SetConfigPath(configPath string) {
	c.configPath = configPath
}

func (c *Client) GetConfig() (*config.RepoConfig, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get the file content from GitHub
	fileContent, _, resp, err := c.client.Repositories.GetContents(ctx, c.repoOwner, c.repoName, c.configPath, &github.RepositoryContentGetOptions{
		Ref: c.branch,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get config file from GitHub: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	// Decode the content
	content, err := base64.StdEncoding.DecodeString(*fileContent.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to decode file content: %w", err)
	}

	// Parse the YAML configuration
	var repoConfig config.RepoConfig
	if err := yaml.Unmarshal(content, &repoConfig); err != nil {
		return nil, fmt.Errorf("failed to parse config YAML: %w", err)
	}

	return &repoConfig, nil
}

func (c *Client) GetLastCommitSHA() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	commits, _, err := c.client.Repositories.ListCommits(ctx, c.repoOwner, c.repoName, &github.CommitsListOptions{
		SHA: c.branch,
		ListOptions: github.ListOptions{
			PerPage: 1,
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to get commits: %w", err)
	}

	if len(commits) == 0 {
		return "", fmt.Errorf("no commits found")
	}

	return *commits[0].SHA, nil
}

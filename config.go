package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config holds user configuration for gh-wut.
type Config struct {
	Repos []string
}

func configPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "gh-wut", "repos")
}

func loadConfig() Config {
	path := configPath()
	if path == "" {
		return Config{}
	}
	f, err := os.Open(path)
	if err != nil {
		return Config{}
	}
	defer f.Close()

	var repos []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			repos = append(repos, line)
		}
	}
	return Config{Repos: repos}
}

func saveConfig(cfg Config) error {
	path := configPath()
	if path == "" {
		return fmt.Errorf("could not determine home directory")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, r := range cfg.Repos {
		fmt.Fprintln(f, r)
	}
	return nil
}

func addRepo(repo string) error {
	if err := validateRepo(repo); err != nil {
		return err
	}
	cfg := loadConfig()
	for _, r := range cfg.Repos {
		if r == repo {
			return fmt.Errorf("repo %s already configured", repo)
		}
	}
	cfg.Repos = append(cfg.Repos, repo)
	return saveConfig(cfg)
}

func removeRepo(repo string) error {
	cfg := loadConfig()
	var filtered []string
	found := false
	for _, r := range cfg.Repos {
		if r == repo {
			found = true
			continue
		}
		filtered = append(filtered, r)
	}
	if !found {
		return fmt.Errorf("repo %s not found in config", repo)
	}
	cfg.Repos = filtered
	return saveConfig(cfg)
}

func listRepos() []string {
	return loadConfig().Repos
}

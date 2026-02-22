package main

import (
	"os"
	"testing"
)

func withTempHome(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
}

func TestLoadSaveConfig(t *testing.T) {
	withTempHome(t)

	cfg := Config{Repos: []string{"owner/repo1", "owner/repo2"}}
	if err := saveConfig(cfg); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	got := loadConfig()
	if len(got.Repos) != 2 || got.Repos[0] != "owner/repo1" || got.Repos[1] != "owner/repo2" {
		t.Errorf("loadConfig = %v, want %v", got.Repos, cfg.Repos)
	}
}

func TestLoadConfig_NoFile(t *testing.T) {
	withTempHome(t)
	got := loadConfig()
	if len(got.Repos) != 0 {
		t.Errorf("loadConfig (no file) = %v, want empty", got.Repos)
	}
}

func TestAddRepo(t *testing.T) {
	withTempHome(t)

	if err := addRepo("owner/new"); err != nil {
		t.Fatalf("addRepo: %v", err)
	}
	got := loadConfig()
	if len(got.Repos) != 1 || got.Repos[0] != "owner/new" {
		t.Errorf("after addRepo = %v, want [owner/new]", got.Repos)
	}
}

func TestAddRepo_Duplicate(t *testing.T) {
	withTempHome(t)

	_ = addRepo("owner/dup")
	err := addRepo("owner/dup")
	if err == nil {
		t.Error("addRepo duplicate: expected error, got nil")
	}
}

func TestRemoveRepo(t *testing.T) {
	withTempHome(t)

	_ = addRepo("owner/a")
	_ = addRepo("owner/b")
	if err := removeRepo("owner/a"); err != nil {
		t.Fatalf("removeRepo: %v", err)
	}
	got := loadConfig()
	if len(got.Repos) != 1 || got.Repos[0] != "owner/b" {
		t.Errorf("after removeRepo = %v, want [owner/b]", got.Repos)
	}
}

func TestRemoveRepo_NotFound(t *testing.T) {
	withTempHome(t)

	err := removeRepo("owner/nope")
	if err == nil {
		t.Error("removeRepo non-existent: expected error, got nil")
	}
}

func TestConfigPath(t *testing.T) {
	withTempHome(t)
	home, _ := os.UserHomeDir()
	got := configPath()
	if got == "" {
		t.Fatal("configPath returned empty")
	}
	if got != home+"/.config/gh-wut/repos" {
		t.Errorf("configPath = %q, want %q", got, home+"/.config/gh-wut/repos")
	}
}

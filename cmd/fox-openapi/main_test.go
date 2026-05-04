package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseCommonHonorsWorkdirAndConfigFlags(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fox-openapi.yaml"), []byte("entry: example.com/app.NewEngine\nout: api/from-config.yaml\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, code := parseCommon("generate", []string{"--workdir", dir, "--out", "api/from-flag.json"})
	if code != 0 {
		t.Fatalf("parseCommon code = %d", code)
	}
	if cfg.Entry != "example.com/app.NewEngine" {
		t.Fatalf("entry = %q", cfg.Entry)
	}
	if cfg.Out != "api/from-flag.json" || cfg.Format != "json" {
		t.Fatalf("out/format = %q/%q", cfg.Out, cfg.Format)
	}

	otherConfig := filepath.Join(dir, "custom.yaml")
	if err := os.WriteFile(otherConfig, []byte("entry: example.com/app.Custom\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, code = parseCommon("generate", []string{"--config", otherConfig})
	if code != 0 {
		t.Fatalf("parseCommon custom config code = %d", code)
	}
	if cfg.Entry != "example.com/app.Custom" {
		t.Fatalf("custom config entry = %q", cfg.Entry)
	}
}

func TestParseCommonHonorsInfoAndServerFlags(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/app\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, code := parseCommon("generate", []string{
		"--workdir", dir,
		"--entry", "example.com/app.NewEngine",
		"--title", "AoneSuite Infra API",
		"--version", "2.0.0",
		"--server", "https://api.example.com",
	})
	if code != 0 {
		t.Fatalf("parseCommon code = %d", code)
	}
	if cfg.Info.Title != "AoneSuite Infra API" || cfg.Info.Version != "2.0.0" {
		t.Fatalf("info = %+v", cfg.Info)
	}
	if len(cfg.Servers) != 1 || cfg.Servers[0].URL != "https://api.example.com" {
		t.Fatalf("servers = %#v", cfg.Servers)
	}
}

func TestRunSubcommandHelpSucceeds(t *testing.T) {
	if code := run([]string{"generate", "--help"}); code != 0 {
		t.Fatalf("run generate --help code = %d, want 0", code)
	}
}

func TestRunInitWritesConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/app\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code := run([]string{"init", "--workdir", dir, "--entry", "internal/server.NewEngine", "--title", "Acme API"})
	if code != 0 {
		t.Fatalf("run init code = %d, want 0", code)
	}
	data, err := os.ReadFile(filepath.Join(dir, "fox-openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "" {
		t.Fatal("expected generated config")
	}
}

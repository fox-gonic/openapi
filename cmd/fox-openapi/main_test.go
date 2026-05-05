package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/fox-gonic/openapi/internal/cli"
)

func TestResolveVersionPrefersLdflagsValue(t *testing.T) {
	orig := version
	defer func() { version = orig }()
	version = "v9.9.9-test"
	if got := resolveVersion(); got != "v9.9.9-test" {
		t.Fatalf("resolveVersion() = %q, want %q", got, "v9.9.9-test")
	}
}

func TestResolveVersionFallsBackToBuildInfo(t *testing.T) {
	orig := version
	defer func() { version = orig }()
	version = ""
	// `go test` runs against the working module — Main.Version is "(devel)"
	// and vcs.revision is populated. The expected outcome is therefore the
	// short revision (with optional +dirty), or "dev" if VCS info is absent.
	got := resolveVersion()
	if got == "" {
		t.Fatal("resolveVersion() returned empty string")
	}
	if got == "(devel)" {
		t.Fatalf("resolveVersion() leaked the (devel) sentinel")
	}
}

func parseCommon(name string, args []string) (cli.Config, int) {
	opts := newCommonOptions()
	cmd := &cobra.Command{
		Use:           name,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	bindCommonFlags(cmd.Flags(), opts)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		return cli.Config{}, cli.ExitUsage
	}
	markOverridesFromFlags(opts, cmd.Flags())
	cfg, err := configFromOptions(opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return cli.Config{}, cli.ExitUsage
	}
	return cfg, 0
}

func TestParseCommonHonorsWorkdirAndConfigFlags(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "fox-openapi.yaml")
	if err := os.WriteFile(configFile, []byte("entry: example.com/app.NewEngine\nout: api/from-config.yaml\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// CLI flags resolve relative to CWD; pass --config explicitly when the
	// config file lives outside CWD.
	cfg, code := parseCommon("generate", []string{
		"--config", configFile,
		"--workdir", dir,
		"--out", "api/from-flag.json",
	})
	if code != 0 {
		t.Fatalf("parseCommon code = %d", code)
	}
	if cfg.Entry != "example.com/app.NewEngine" {
		t.Fatalf("entry = %q", cfg.Entry)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	wantOut := filepath.Join(cwd, "api/from-flag.json")
	if cfg.Out != wantOut || cfg.Format != "json" {
		t.Fatalf("out/format = %q/%q (want %q/json)", cfg.Out, cfg.Format, wantOut)
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

func TestParseCommonOutInConfigFileIsRelativeToConfigDir(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "fox-openapi.yaml")
	if err := os.WriteFile(configFile, []byte("entry: example.com/app.NewEngine\nout: api/openapi.yaml\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, code := parseCommon("generate", []string{"--config", configFile})
	if code != 0 {
		t.Fatalf("parseCommon code = %d", code)
	}
	want := filepath.Join(dir, "api/openapi.yaml")
	if cfg.Out != want {
		t.Fatalf("cfg.Out = %q, want %q", cfg.Out, want)
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

func TestNormalizeSourcePattern(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "internal", "aone")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		in, want string
	}{
		{"internal/aone", "./internal/aone/..."},
		{"internal/aone/...", "./internal/aone/..."},
		{"./internal/aone", "./internal/aone"},
		{"./...", "./..."},
		{".", "."},
		{"github.com/foo/bar", "github.com/foo/bar"},
		{"github.com/foo/bar/...", "github.com/foo/bar/..."},
	}
	for _, tc := range cases {
		got := normalizeSourcePattern(tc.in)
		if got != tc.want {
			t.Errorf("normalizeSourcePattern(%q) = %q, want %q", tc.in, got, tc.want)
		}
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

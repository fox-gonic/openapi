package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/fox-gonic/openapi/internal/cli"
)

func TestPickVersion(t *testing.T) {
	cases := []struct {
		name     string
		override string
		info     *debug.BuildInfo
		want     string
	}{
		{
			name:     "ldflags override wins",
			override: "v9.9.9-test",
			info:     &debug.BuildInfo{Main: debug.Module{Version: "v0.1.0"}},
			want:     "v9.9.9-test",
		},
		{
			name: "module version from go install",
			info: &debug.BuildInfo{Main: debug.Module{Version: "v0.2.1"}},
			want: "v0.2.1",
		},
		{
			name: "devel sentinel falls through to vcs revision",
			info: &debug.BuildInfo{
				Main: debug.Module{Version: "(devel)"},
				Settings: []debug.BuildSetting{
					{Key: "vcs.revision", Value: "abcdef0123456789deadbeef"},
				},
			},
			want: "abcdef012345",
		},
		{
			name: "dirty revision suffixed",
			info: &debug.BuildInfo{
				Main: debug.Module{Version: "(devel)"},
				Settings: []debug.BuildSetting{
					{Key: "vcs.revision", Value: "abcdef0123456789"},
					{Key: "vcs.modified", Value: "true"},
				},
			},
			want: "abcdef012345+dirty",
		},
		{
			name: "no info at all",
			info: nil,
			want: "dev",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pickVersion(tc.override, tc.info); got != tc.want {
				t.Fatalf("pickVersion = %q, want %q", got, tc.want)
			}
		})
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

func TestRootCommandAcceptsGenerateFlags(t *testing.T) {
	cmd := newRootCommand()
	cmd.SetArgs([]string{
		"--entry", "example.com/app.NewEngine",
		"--out", "api/openapi.json",
		"--title", "Acme API",
	})
	if err := cmd.ParseFlags(cmd.Flags().Args()); err != nil {
		t.Fatalf("parse root flags: %v", err)
	}
	if cmd.Flags().Lookup("entry") == nil || cmd.Flags().Lookup("out") == nil || cmd.Flags().Lookup("title") == nil {
		t.Fatal("root command should expose common generate flags")
	}
}

func TestGenerateCommandAcceptsFilterFlags(t *testing.T) {
	cmd := newGenerateCommand()
	if err := cmd.ParseFlags([]string{
		"--filter", "x-public != false",
		"--filter", "x-product = sandbox",
		"--prune-unused-components",
	}); err != nil {
		t.Fatalf("parse filter flags: %v", err)
	}
}

func TestAdvancedGenerateFlagsAreHiddenButUsable(t *testing.T) {
	cmd := newGenerateCommand()
	for _, name := range []string{"source", "include-test-files", "metadata-hook", "entry-config-loader", "entry-config-path", "keep-driver", "verbose", "format"} {
		flag := cmd.Flags().Lookup(name)
		if flag == nil {
			t.Fatalf("flag %q is not registered", name)
		}
		if !flag.Hidden {
			t.Fatalf("flag %q should be hidden from normal help", name)
		}
	}

	var help strings.Builder
	cmd.SetOut(&help)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("generate --help: %v", err)
	}
	if strings.Contains(help.String(), "--metadata-hook") || strings.Contains(help.String(), "--entry-config-loader") {
		t.Fatalf("advanced flags leaked into help:\n%s", help.String())
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

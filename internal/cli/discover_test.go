package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func goModTidy(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy: %v\n%s", err, out)
	}
}

// writeMinimalServerModule creates a temp module with a single NewEngine entry.
func writeMinimalServerModule(t *testing.T, extraServerSrc string) string {
	t.Helper()
	dir := t.TempDir()
	foxRoot, openapiRoot := repoPaths(t)
	writeFile(t, filepath.Join(dir, "go.mod"), `module example.com/app

go 1.25

require (
	github.com/fox-gonic/fox v0.0.0
	github.com/fox-gonic/openapi v0.0.0
)

replace github.com/fox-gonic/fox => `+filepath.ToSlash(foxRoot)+`
replace github.com/fox-gonic/openapi => `+filepath.ToSlash(openapiRoot)+`
`)
	src := `package server

import "github.com/fox-gonic/fox"

func NewEngine() *fox.Engine {
	return fox.New()
}
` + extraServerSrc
	writeFile(t, filepath.Join(dir, "internal/server/server.go"), src)
	goModTidy(t, dir)
	return dir
}

func TestDiscoverEntrySingleMatch(t *testing.T) {
	dir := writeMinimalServerModule(t, "")
	entry, err := DiscoverEntry(dir, []string{"./..."})
	if err != nil {
		t.Fatal(err)
	}
	if entry.ImportPath != "example.com/app/internal/server" || entry.FuncName != "NewEngine" {
		t.Fatalf("unexpected entry: %+v", entry)
	}
}

func TestDiscoverEntryNoMatch(t *testing.T) {
	dir := t.TempDir()
	foxRoot, openapiRoot := repoPaths(t)
	writeFile(t, filepath.Join(dir, "go.mod"), `module example.com/empty

go 1.25

require github.com/fox-gonic/fox v0.0.0

replace github.com/fox-gonic/fox => `+filepath.ToSlash(foxRoot)+`
replace github.com/fox-gonic/openapi => `+filepath.ToSlash(openapiRoot)+`
`)
	writeFile(t, filepath.Join(dir, "main.go"), `package main

func main() {}
`)
	goModTidy(t, dir)
	_, err := DiscoverEntry(dir, []string{"./..."})
	if err == nil || !strings.Contains(err.Error(), "no entry function found") {
		t.Fatalf("expected no entry error, got %v", err)
	}
}

func TestDiscoverEntryMultipleMatchesError(t *testing.T) {
	dir := writeUserModule(t)
	_, err := DiscoverEntry(dir, []string{"./internal/server"})
	if err == nil || !strings.Contains(err.Error(), "multiple entry candidates found") {
		t.Fatalf("expected multiple candidates error, got %v", err)
	}
	// Error must list every candidate so the user can pick one.
	for _, want := range []string{
		"example.com/app/internal/server.NewEngine",
		"example.com/app/internal/server.NewEngineWithError",
		"example.com/app/internal/server.NewEngineWithContext",
		"example.com/app/internal/server.NewEngineWithConfig",
		"example.com/app/internal/server.NewEngineWithConfigNoError",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing candidate %s:\n%s", want, err.Error())
		}
	}
}

func TestDiscoverEntryMarkerSelectsCandidate(t *testing.T) {
	dir := t.TempDir()
	foxRoot, openapiRoot := repoPaths(t)
	writeFile(t, filepath.Join(dir, "go.mod"), `module example.com/marked

go 1.25

require github.com/fox-gonic/fox v0.0.0

replace github.com/fox-gonic/fox => `+filepath.ToSlash(foxRoot)+`
replace github.com/fox-gonic/openapi => `+filepath.ToSlash(openapiRoot)+`
`)
	writeFile(t, filepath.Join(dir, "internal/server/server.go"), `package server

import "github.com/fox-gonic/fox"

func NewEngine() *fox.Engine {
	return fox.New()
}

// NewTestEngine spins up a duplicate engine for tests.
//
// fox-openapi:entry
func NewTestEngine() *fox.Engine {
	return fox.New()
}
`)
	goModTidy(t, dir)
	entry, err := DiscoverEntry(dir, []string{"./..."})
	if err != nil {
		t.Fatalf("expected marker to disambiguate, got %v", err)
	}
	if entry.FuncName != "NewTestEngine" {
		t.Fatalf("expected NewTestEngine, got %s", entry.FuncName)
	}
}

func TestDiscoverEntryScopeLimitsSearch(t *testing.T) {
	dir := writeUserModule(t)
	// Ranged at a non-server package should produce zero matches.
	_, err := DiscoverEntry(dir, []string{"./internal/config"})
	if err == nil || !strings.Contains(err.Error(), "no entry function found") {
		t.Fatalf("expected no match in scoped path, got %v", err)
	}
}

// Discovery scope and Sources must be independent: even when the user
// narrows entry discovery to a sub-tree, Sources keeps its module-wide
// default so comment extraction does not lose field/handler docs that
// live elsewhere.
func TestLoadConfigEntryDiscoveryScopeDoesNotNarrowSources(t *testing.T) {
	dir := writeMinimalServerModule(t, "")
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(Overrides{
		EntryDiscoveryScope:    []string{"./internal/server"},
		EntryDiscoveryScopeSet: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Entry != "example.com/app/internal/server.NewEngine" {
		t.Fatalf("entry = %q", cfg.Entry)
	}
	if len(cfg.Sources) != 1 || cfg.Sources[0] != "./..." {
		t.Fatalf("sources unexpectedly narrowed: %#v", cfg.Sources)
	}
}

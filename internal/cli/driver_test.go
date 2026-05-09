package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteDriverRendersMetadataAndAbsoluteSources(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		Workdir: dir,
		Format:  "json",
		Sources: []string{"./internal/server", "./pkg/..."},
		Info: InfoConfig{
			Title:       "Acme",
			Version:     "1.0.0",
			Description: "API description",
		},
		Servers: []ServerConfig{{URL: "https://api.example.com", Description: "prod"}},
		Tags: []TagConfig{{
			Name:        "users",
			Description: "User endpoints",
			ExternalDocs: &ExternalDocsConfig{
				URL:         "https://docs.example.com/users",
				Description: "User docs",
			},
		}},
		SecuritySchemes: map[string]Scheme{
			"BearerAuth": {
				Type:         "http",
				Scheme:       "bearer",
				BearerFormat: "JWT",
			},
		},
		Filters:               []string{"x-public != false", "x-product = sandbox"},
		PruneUnusedComponents: true,
	}
	driverDir, err := WriteDriver(cfg, Entry{ImportPath: "example.com/app/internal/server", FuncName: "NewEngine"}, &Hook{ImportPath: "example.com/app/internal/server", FuncName: "ConfigureOpenAPI"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(driverDir, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		`openapi.Info("Acme", "1.0.0")`,
		`openapi.Server("https://api.example.com")`,
		`openapi.ApplySpecMetadata(spec, openapi.SpecMetadata{`,
		`InfoDescription: "API description"`,
		`"prod"`,
		`openapi.SpecTag{Name: "users"`,
		`openapi.SecuritySchemeFromConfig("BearerAuth"`,
		`openapi.FilterOperationExpression("x-public != false")`,
		`openapi.FilterOperationExpression("x-product = sandbox")`,
		`openapi.PruneUnusedComponents()`,
		`opts = append(opts, userhook.ConfigureOpenAPI()...)`,
		filepath.ToSlash(filepath.Join(dir, "internal/server")),
		filepath.ToSlash(filepath.Join(dir, "pkg")) + "/...",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("driver missing %q:\n%s", want, text)
		}
	}
}

func TestWriteDriverOmitsOpenAPI3AndYAMLImports(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		Workdir: dir,
		Format:  "yaml",
		Sources: []string{},
		Info:    InfoConfig{Title: "Acme", Version: "1.0.0"},
	}
	driverDir, err := WriteDriver(cfg, Entry{ImportPath: "example.com/app", FuncName: "NewEngine"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(driverDir, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "github.com/getkin/kin-openapi/openapi3") {
		t.Fatalf("driver imported openapi3 when unused:\n%s", data)
	}
	if strings.Contains(string(data), "github.com/goccy/go-yaml") {
		t.Fatalf("driver imported yaml package:\n%s", data)
	}
}

func TestCleanupDriverRemovesEmptyParent(t *testing.T) {
	dir := t.TempDir()
	driverDir := filepath.Join(dir, ".fox-openapi", "driver")
	if err := os.MkdirAll(driverDir, 0o755); err != nil {
		t.Fatal(err)
	}
	CleanupDriver(driverDir, false)
	if _, err := os.Stat(filepath.Join(dir, ".fox-openapi")); !os.IsNotExist(err) {
		t.Fatalf("expected .fox-openapi to be removed, got %v", err)
	}
}

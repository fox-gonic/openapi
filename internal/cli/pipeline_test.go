package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPipelineGeneratesSpecFromUserModule(t *testing.T) {
	dir := writeUserModule(t)
	cfg := Config{
		Entry:        "example.com/app/internal/server.NewEngine",
		MetadataHook: "example.com/app/internal/server.ConfigureOpenAPI",
		Out:          "api/openapi.yaml",
		Format:       "yaml",
		Sources:      []string{"./internal/server"},
		Info: InfoConfig{
			Title:       "Example API",
			Version:     "1.0.0",
			Description: "Example description",
		},
		Servers: []ServerConfig{{URL: "https://api.example.com", Description: "prod"}},
		Tags:    []TagConfig{{Name: "users", Description: "User endpoints"}},
		Workdir: dir,
	}
	data, warnings, err := RunPipeline(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v", warnings)
	}
	for _, want := range []string{
		"title: Example API",
		"description: Example description",
		"url: https://api.example.com",
		"description: prod",
		"/users/{id}:",
		"tags:",
		"- users",
		"User endpoints",
	} {
		if !bytes.Contains(data, []byte(want)) {
			t.Fatalf("generated spec missing %q:\n%s", want, data)
		}
	}
	if strings.Contains(string(data), "WARN:") {
		t.Fatalf("warnings leaked into stdout:\n%s", data)
	}
}

func TestRunPipelineDoesNotRequireOpenAPIModuleInUserGoMod(t *testing.T) {
	dir := writeUserModuleWithoutOpenAPIRequire(t)
	before, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		Entry:   "example.com/app/internal/server.NewEngine",
		Out:     "api/openapi.yaml",
		Format:  "yaml",
		Sources: []string{"./internal/server"},
		Info:    InfoConfig{Title: "Example API", Version: "1.0.0"},
		Workdir: dir,
	}
	data, _, err := RunPipeline(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("/users/{id}:")) {
		t.Fatalf("generated spec missing route:\n%s", data)
	}
	after, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("go.mod changed:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestRunPipelineSupportsContextAndConfigEntry(t *testing.T) {
	dir := writeUserModule(t)
	cfg := Config{
		Entry:   "example.com/app/internal/server.NewEngineWithConfig",
		Out:     "api/openapi.yaml",
		Format:  "yaml",
		Sources: []string{"./internal/server"},
		Info:    InfoConfig{Title: "Example API", Version: "1.0.0"},
		EntryConfig: EntryConfig{
			Loader: "example.com/app/internal/config.Load",
			Path:   "config.yaml",
		},
		Workdir: dir,
	}
	data, _, err := RunPipeline(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("/users/{id}:")) {
		t.Fatalf("generated spec missing route:\n%s", data)
	}
}

func TestRunPipelineSupportsContextAndNilConfigEntry(t *testing.T) {
	dir := writeUserModule(t)
	cfg := Config{
		Entry:   "example.com/app/internal/server.NewEngineWithConfig",
		Out:     "api/openapi.yaml",
		Format:  "yaml",
		Sources: []string{"./internal/server"},
		Info:    InfoConfig{Title: "Example API", Version: "1.0.0"},
		Workdir: dir,
	}
	data, _, err := RunPipeline(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("/users/{id}:")) {
		t.Fatalf("generated spec missing route:\n%s", data)
	}
}

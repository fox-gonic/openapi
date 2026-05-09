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

func TestRunPipelineGeneratesSpecFromRouteManifest(t *testing.T) {
	dir := writeUserModule(t)
	manifestPath := filepath.Join(dir, "routes.manifest.json")
	if err := os.WriteFile(manifestPath, []byte(`{
  "version": "fox.route-manifest/v1",
  "routes": [
    {
      "method": "GET",
      "path": "/users/:id",
      "handler": "example.com/app/internal/server.GetUser"
    }
  ]
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		RouteManifest: manifestPath,
		Out:           "api/openapi.yaml",
		Format:        "yaml",
		Sources:       []string{},
		Info:          InfoConfig{Title: "Manifest API", Version: "1.0.0"},
		Workdir:       dir,
	}
	data, warnings, err := RunPipeline(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v", warnings)
	}
	for _, want := range []string{
		"title: Manifest API",
		"/users/{id}:",
		"operationId: example_com_app_internal_server_GetUser",
		"name: id",
		"server_User",
		"default:",
	} {
		if !bytes.Contains(data, []byte(want)) {
			t.Fatalf("generated spec missing %q:\n%s", want, data)
		}
	}
}

func TestRunManifestPipelineFiltersPublicExtensionAndPrunesComponents(t *testing.T) {
	dir := writeUserModule(t)
	writeFile(t, filepath.Join(dir, "internal/server/public.go"), `package server

import "github.com/fox-gonic/fox"

// GetPublicUser fetches a public user.
//
// openapi:
//
//	x-public: true
func GetPublicUser(ctx *fox.Context, req GetUserRequest) (User, error) {
	return User{}, nil
}

// GetInternalUser fetches an internal user.
//
// openapi:
//
//	x-public: false
func GetInternalUser(ctx *fox.Context, req GetUserRequest) (User, error) {
	return User{}, nil
}
`)
	manifestPath := filepath.Join(dir, "routes.manifest.json")
	if err := os.WriteFile(manifestPath, []byte(`{
  "version": "fox.route-manifest/v1",
  "routes": [
    {
      "method": "GET",
      "path": "/public-users/:id",
      "handler": "example.com/app/internal/server.GetPublicUser"
    },
    {
      "method": "GET",
      "path": "/internal-users/:id",
      "handler": "example.com/app/internal/server.GetInternalUser"
    }
  ]
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	data, warnings, err := RunPipeline(Config{
		RouteManifest:         manifestPath,
		Out:                   "api/openapi.yaml",
		Format:                "yaml",
		Sources:               []string{"./internal/server"},
		Info:                  InfoConfig{Title: "Manifest API", Version: "1.0.0"},
		Filters:               []string{"x-public != false"},
		PruneUnusedComponents: true,
		Workdir:               dir,
	})

	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v", warnings)
	}
	out := string(data)
	if !strings.Contains(out, "/public-users/{id}:") {
		t.Fatalf("generated spec missing public path:\n%s", out)
	}
	if strings.Contains(out, "/internal-users/{id}:") {
		t.Fatalf("generated spec includes internal path:\n%s", out)
	}
}

func TestRunPipelineRejectsUnsupportedRouteManifestVersion(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "routes.manifest.json")
	if err := os.WriteFile(manifestPath, []byte(`{"version":"fox.route-manifest/v0","routes":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := RunPipeline(Config{
		RouteManifest: manifestPath,
		Out:           "api/openapi.yaml",
		Format:        "yaml",
		Info:          InfoConfig{Title: "Manifest API", Version: "1.0.0"},
		Workdir:       dir,
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported route manifest version") {
		t.Fatalf("RunPipeline error = %v, want unsupported route manifest version", err)
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

func TestRunPipelineAutoDiscoversConfigLoaderFromEntryConfigPath(t *testing.T) {
	dir := writeUserModule(t)
	cfg := Config{
		Entry:   "example.com/app/internal/server.NewEngineWithRequiredConfig",
		Out:     "api/openapi.yaml",
		Format:  "yaml",
		Sources: []string{"./internal/server"},
		Info:    InfoConfig{Title: "Example API", Version: "1.0.0"},
		EntryConfig: EntryConfig{
			Path: "config.yaml",
		},
		Workdir: dir,
	}
	data, _, err := RunPipeline(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("/configured/loaded:")) {
		t.Fatalf("generated spec missing route registered by configured engine:\n%s", data)
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

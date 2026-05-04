package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitConfigWritesDefaultConfigWithModuleEntry(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/app\n\ngo 1.25\n")

	err := InitConfig(InitOptions{
		Workdir: dir,
		Entry:   "internal/server.NewEngine",
		Title:   "Acme API",
		Version: "1.2.3",
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "fox-openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		"entry: example.com/app/internal/server.NewEngine\n",
		"out: api/openapi.yaml\n",
		"  - ./...\n",
		"  title: Acme API\n",
		"  version: 1.2.3\n",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("config missing %q:\n%s", want, text)
		}
	}
}

func TestInitConfigRefusesExistingConfigUnlessForced(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/app\n\ngo 1.25\n")
	writeFile(t, filepath.Join(dir, "fox-openapi.yaml"), "entry: old\n")

	err := InitConfig(InitOptions{Workdir: dir, Entry: "example.com/app.NewEngine"})
	if err == nil {
		t.Fatal("expected existing config error")
	}

	err = InitConfig(InitOptions{Workdir: dir, Entry: "example.com/app.NewEngine", Force: true})
	if err != nil {
		t.Fatal(err)
	}
}

func TestInitConfigIncludesJSONFormatWhenOutIsJSON(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/app\n\ngo 1.25\n")

	err := InitConfig(InitOptions{
		Workdir: dir,
		Entry:   "example.com/app.NewEngine",
		Out:     "api/openapi.json",
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "fox-openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "format: json\n") {
		t.Fatalf("expected json format:\n%s", data)
	}
}

package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestStatusWrapperExampleGeneratesOAPICodegenFriendlyResponses(t *testing.T) {
	_, openapiRoot := repoPaths(t)
	exampleDir := filepath.Join(openapiRoot, "examples", "status-wrapper-oapi-codegen")
	if _, err := os.Stat(filepath.Join(exampleDir, "go.mod")); err != nil {
		t.Fatalf("status wrapper example is missing: %v", err)
	}

	spec, warnings, err := RunPipeline(Config{
		Entry:   "github.com/fox-gonic/openapi/examples/status-wrapper-oapi-codegen/internal/handler.NewEngine",
		Out:     "api/openapi.yaml",
		Format:  "yaml",
		Sources: []string{"./internal/handler"},
		Info:    InfoConfig{Title: "Status Wrapper OAPI Codegen Example", Version: "1.0.0"},
		Workdir: exampleDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v", warnings)
	}

	specText := string(spec)
	for _, bad := range []string{"handler_StatusResponse", "StatusResponseGithub", "StatusResponse["} {
		if strings.Contains(specText, bad) {
			t.Fatalf("generated spec leaked status wrapper %q:\n%s", bad, specText)
		}
	}
	for _, want := range []string{
		`"201":`,
		`$ref: "#/components/schemas/handler_SandboxResponse"`,
		`"202":`,
		`$ref: "#/components/schemas/handler_TemplateResponse"`,
	} {
		if !strings.Contains(specText, want) {
			t.Fatalf("generated spec missing %q:\n%s", want, specText)
		}
	}

	tmp := t.TempDir()
	specPath := filepath.Join(tmp, "openapi.yaml")
	if err := os.WriteFile(specPath, spec, 0o644); err != nil {
		t.Fatal(err)
	}
	generatedPath := filepath.Join(tmp, "sandbox.gen.go")
	cmd := exec.Command("go", "run", "github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.4.1",
		"-generate", "types,client",
		"-package", "apis",
		"-o", generatedPath,
		specPath,
	)
	cmd.Dir = openapiRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("oapi-codegen: %v\n%s", err, out)
	}

	generated, err := os.ReadFile(generatedPath)
	if err != nil {
		t.Fatal(err)
	}
	generatedText := string(generated)
	for _, bad := range []string{"HandlerStatusResponse", "StatusResponseGithub"} {
		if strings.Contains(generatedText, bad) {
			t.Fatalf("generated SDK leaked status wrapper %q:\n%s", bad, generatedText)
		}
	}
	for _, want := range []string{
		"JSON201      *HandlerSandboxResponse",
		"JSON202      *HandlerTemplateResponse",
	} {
		if !strings.Contains(generatedText, want) {
			t.Fatalf("generated SDK missing %q:\n%s", want, generatedText)
		}
	}
}

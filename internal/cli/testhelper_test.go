package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func repoPaths(t *testing.T) (foxRoot, openapiRoot string) {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	openapiRoot = filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	var err error
	openapiRoot, err = filepath.Abs(openapiRoot)
	if err != nil {
		t.Fatal(err)
	}
	foxRoot = os.Getenv("FOX_ROOT")
	if foxRoot == "" {
		candidates := []string{
			filepath.Join(openapiRoot, "..", "..", "miclle", "fox"),
			filepath.Join(openapiRoot, "..", "fox"),
		}
		for _, candidate := range candidates {
			if _, err := os.Stat(filepath.Join(candidate, "go.mod")); err == nil {
				foxRoot = candidate
				break
			}
		}
	}
	if foxRoot == "" {
		t.Skip("set FOX_ROOT to a local github.com/fox-gonic/fox checkout for driver tests")
	}
	foxRoot, err = filepath.Abs(foxRoot)
	if err != nil {
		t.Fatal(err)
	}
	return foxRoot, openapiRoot
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeUserModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	foxRoot, openapiRoot := repoPaths(t)
	writeFile(t, filepath.Join(dir, "go.mod"), `module example.com/app

go 1.25

require (
	github.com/fox-gonic/fox v0.0.0
	github.com/fox-gonic/openapi v0.0.0
	github.com/getkin/kin-openapi v0.133.0
)

replace github.com/fox-gonic/fox => `+filepath.ToSlash(foxRoot)+`
replace github.com/fox-gonic/openapi => `+filepath.ToSlash(openapiRoot)+`
`)
	writeFile(t, filepath.Join(dir, "internal/server/server.go"), `package server

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/fox-gonic/fox"
	"example.com/app/internal/config"
	openapi "github.com/fox-gonic/openapi"
	"github.com/getkin/kin-openapi/openapi3"
)

type GetUserRequest struct {
	ID string `+"`uri:\"id\" binding:\"required\"`"+`
}

type User struct {
	ID string `+"`json:\"id\"`"+`
	Name string `+"`json:\"name\"`"+`
}

type GenericResponse[T any] struct {
	Data T `+"`json:\"data\"`"+`
}

type Handler struct{}

type GenericHandler[T any] struct{}

type AliasHandler = Handler

// GetUser fetches a user by id.
func GetUser(ctx *fox.Context, req GetUserRequest) (User, error) {
	return User{ID: req.ID, Name: "Ada"}, nil
}

func GetGenericUser(ctx *fox.Context) (GenericResponse[User], error) {
	return GenericResponse[User]{Data: User{ID: "1", Name: "Ada"}}, nil
}

func GetGenericRuntimeUser[T any](ctx *fox.Context) (GenericResponse[T], error) {
	return GenericResponse[T]{}, nil
}

func (h *Handler) AliasUser(ctx *fox.Context) (User, error) {
	return User{ID: "1", Name: "Ada"}, nil
}

func (h Handler) ValueUser(ctx *fox.Context) (User, error) {
	return User{ID: "1", Name: "Ada"}, nil
}

func (h GenericHandler[T]) GenericUser(ctx *fox.Context) (GenericResponse[T], error) {
	return GenericResponse[T]{}, nil
}

func NewEngine() *fox.Engine {
	e := fox.New()
	e.GET("/users/:id", GetUser)
	return e
}

func NewEngineWithError() (*fox.Engine, error) {
	return NewEngine(), nil
}

func NewEngineWithContext(ctx context.Context) (*fox.Engine, error) {
	return NewEngine(), nil
}

func NewEngineWithConfig(ctx context.Context, cfg *config.Config) (*fox.Engine, error) {
	return NewEngine(), nil
}

func NewEngineWithRequiredConfig(ctx context.Context, cfg *config.Config) (*fox.Engine, error) {
	if cfg == nil {
		return nil, errors.New("config is required")
	}
	e := NewEngine()
	e.GET(fmt.Sprintf("/configured/%s", cfg.Name), GetUser)
	return e, nil
}

func NewEngineWithConfigNoError(ctx context.Context, cfg *config.Config) *fox.Engine {
	return NewEngine()
}

func BrokenEntry() (*fox.Engine, error) {
	return nil, errors.New("boom")
}

func ConfigureOpenAPI() []openapi.Option {
	return []openapi.Option{
		openapi.RegisterFormatter(reflect.TypeOf(User{}), openapi3.NewObjectSchema()),
		openapi.Operation("GET", "/users/:id", openapi.Tags("users")),
	}
}

func badHook() []openapi.Option { return nil }

func BadEntry(arg string) *fox.Engine { return nil }
`)
	writeFile(t, filepath.Join(dir, "internal/config/config.go"), `package config

type Config struct {
	Name string
}

func Load(path string) (*Config, error) {
	return &Config{Name: "loaded"}, nil
}
`)
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy: %v\n%s", err, out)
	}
	return dir
}

func writeUserModuleWithoutOpenAPIRequire(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	foxRoot, openapiRoot := repoPaths(t)
	writeFile(t, filepath.Join(dir, "go.mod"), `module example.com/app

go 1.25

require github.com/fox-gonic/fox v0.0.0

replace github.com/fox-gonic/fox => `+filepath.ToSlash(foxRoot)+`
replace github.com/fox-gonic/openapi => `+filepath.ToSlash(openapiRoot)+`
`)
	writeFile(t, filepath.Join(dir, "internal/server/server.go"), `package server

import "github.com/fox-gonic/fox"

type User struct {
	ID string `+"`json:\"id\"`"+`
}

func GetUser(ctx *fox.Context) User {
	return User{ID: "usr_1"}
}

func NewEngine() *fox.Engine {
	e := fox.New()
	e.GET("/users/:id", GetUser)
	return e
}
`)
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy: %v\n%s", err, out)
	}
	return dir
}

# Fox OpenAPI

English | [简体中文](README.zh-CN.md)

OpenAPI 3.0.3 generator and CLI for [Fox](https://github.com/fox-gonic/fox).

The recommended workflow is `fox-openapi`: expose a function that builds a
`*fox.Engine`, then generate a committed `openapi.yaml` during development or
CI. Business code does not need to mount OpenAPI handlers or import this module
unless it uses optional OpenAPI metadata hooks or the library API directly.

## Install

```bash
go install github.com/fox-gonic/openapi/cmd/fox-openapi@latest
```

For local development in this repository:

```bash
go run ./cmd/fox-openapi version
```

## Quickstart

Expose an entry function that registers routes and returns a `*fox.Engine`.
It should not call `Run`, open listeners, or start infrastructure that is not
needed for route registration.

```go
package server

import "github.com/fox-gonic/fox"

func NewEngine() *fox.Engine {
	engine := fox.New()
	engine.GET("/users/:id", getUser)
	return engine
}
```

Then generate, verify, and preview the committed spec:

```bash
fox-openapi                                # auto-discovers entry from ./...
fox-openapi ./internal/server              # narrow entry discovery to a directory
fox-openapi check
fox-openapi serve --addr 127.0.0.1:8765
```

When `entry` is omitted from the config and `--entry` is not passed, the CLI
scans `sources` (default `./...`) for an exported function whose signature
matches one of the supported entry shapes. If exactly one is found, it is
used. If multiple are found, the CLI fails with the candidate list — pick one
with `--entry` or annotate the canonical entry with a doc comment marker:

```go
// NewEngine builds the production HTTP engine.
//
// fox-openapi:entry
func NewEngine() *fox.Engine { ... }
```

When at least one function carries the `fox-openapi:entry` marker, only
marked candidates are considered, so adding the marker disambiguates without
deleting other entry-shaped helpers.

`serve` exposes `/openapi.yaml`, `/openapi.json`, `/docs`, `/scalar`, and
`/redoc` with embedded offline UI assets.

For small projects, no config file is required. Pass flags only when you want
to override defaults:

```bash
fox-openapi \
  --entry github.com/acme/myapp/internal/server.NewEngine \
  --out api/openapi.yaml \
  --title "Acme API"
```

Use `fox-openapi init` only when you want to commit a config file for shared
metadata such as title, servers, tags, security schemes, or entry config.

The CLI builds an isolated temporary driver. For basic generation, the
application module does not need a `tools.go` file or a committed direct
`github.com/fox-gonic/openapi` requirement; the driver build resolves that
temporary dependency and restores `go.mod`/`go.sum` afterward. Add a direct
requirement only when application code imports OpenAPI metadata hooks or
library APIs.

## Entry Functions

`entry` must name an exported function with one of these signatures:

```go
func NewEngine() *fox.Engine
func NewEngine() (*fox.Engine, error)
func NewEngine(context.Context) *fox.Engine
func NewEngine(context.Context) (*fox.Engine, error)
func NewEngine(context.Context, *Config) *fox.Engine
func NewEngine(context.Context, *Config) (*fox.Engine, error)
```

For config-taking entries, omit `entryConfig` to pass `nil` as the config
argument, or provide a loader:

```yaml
entryConfig:
  loader: github.com/acme/myapp/internal/config.Load
  path: config.yaml
```

Passing `nil` lets production code share one route-registration entry with
OpenAPI generation without initializing databases or external providers.

## Path resolution

Paths follow standard go-tooling conventions:

- **CLI flags** (`--out`, `--config`, `--workdir`, `--entry-config-path`): relative
  to the **current working directory** (where you invoked the command).
- **YAML fields** (`out`, `entryConfig.path`, `workdir`): relative to the
  **directory containing the config file**, so `fox-openapi.yaml` and the
  artefacts it points to keep a stable layout regardless of where you run.
- **Positional path** (`fox-openapi generate ./internal/aone`): narrows where
  the CLI **looks for the entry function**. It does **not** narrow source
  scanning — `sources` (default `./...`) still drives comment extraction so
  field/handler docs in sub-packages outside the entry directory are
  preserved. To override scanning explicitly, set `sources` in YAML or pass
  `--source`.

```bash
cd ~/myapp
fox-openapi generate internal/aone --out api/openapi.yaml
# wrote ~/myapp/api/openapi.yaml   ← relative to CWD, not the scanned dir
```

## Config

`fox-openapi init` writes a config like:

```yaml
entry: github.com/acme/myapp/internal/server.NewEngine
out: api/openapi.yaml
sources:
  - ./...
info:
  title: Acme API
  version: 1.0.0
servers:
  - url: https://api.acme.com
```

Supported config keys:

- `entry`: entry function. Optional — when omitted, the CLI auto-discovers
  an exported function with a supported signature from `sources`. Set
  explicitly to override discovery, or use `// fox-openapi:entry` in source.
- `out`: output file, default `api/openapi.yaml`.
- `format`: `yaml` or `json`; inferred from `out` when omitted.
- `sources`: source directories for Go doc comments; default `./...`.
- `includeTestFiles`: include `_test.go` while scanning source comments.
- `info`: `title`, `version`, `description`.
- `servers`: list of `url` and optional `description`.
- `tags`: top-level OpenAPI tag registry.
- `securitySchemes`: serializable HTTP, API key, OAuth2, or OpenID Connect schemes.
- `metadataHook`: optional advanced Go hook.
- `entryConfig`: optional `loader` and `path` for config-taking entries.

CLI flags override config values. Config values override defaults.

## Commands

```bash
fox-openapi init --entry internal/server.NewEngine --title "Acme API"
fox-openapi --entry github.com/acme/myapp/internal/server.NewEngine --out api/openapi.yaml --title "Acme API"
fox-openapi generate --entry github.com/acme/myapp/internal/server.NewEngine --out api/openapi.yaml --title "Acme API"
fox-openapi check
fox-openapi serve --addr 127.0.0.1:8765
fox-openapi version
```

`fox-openapi`, `generate`, `check`, and `serve` share the common config flags:

- `--config`: config file path, default `fox-openapi.yaml`.
- `--entry`: entry function.
- `--out`: output path, default `api/openapi.yaml`.
- `--title` and `--version`: OpenAPI info metadata.
- `--server`: repeatable OpenAPI server URL.
- `--workdir`: user project root.

Advanced flags remain available for scripts and unusual projects but are hidden
from normal help: `--format`, `--source`, `--include-test-files`,
`--metadata-hook`, `--entry-config-loader`, `--entry-config-path`,
`--keep-driver`, and `--verbose`.

`serve` also supports `--addr`, repeatable `--ui`, `--watch`, and `--open`.

## Metadata

The generator reads regular Go comments from source files to fill operation
summaries, operation descriptions, and schema field descriptions. It does not
require `doc` tags.

```go
type CreateUserRequest struct {
	// Display name for the new user.
	Name string `json:"name" binding:"required"`
}

// Create user.
//
// Creates a user and returns the persisted representation.
func createUser(ctx *fox.Context, req CreateUserRequest) (UserResponse, error) {
	return UserResponse{}, nil
}
```

For metadata that needs Go values, add a small optional hook:

```go
func ConfigureOpenAPI() []openapi.Option {
	return []openapi.Option{
		openapi.Group("/users", openapi.Tags("users")),
		openapi.Operation("GET", "/users/:id", openapi.Security("BearerAuth")),
	}
}
```

Then configure it:

```yaml
metadataHook: github.com/acme/myapp/internal/openapimeta.ConfigureOpenAPI
```

Explicit success responses override the default success response inferred from
the handler return type. For simple status wrapper helpers such as
`return statusResponse(http.StatusCreated, UserResponse{}), nil`, `Source`
can infer the response status from the return statement and use the wrapper's
payload type as the response schema, so most handlers do not need a metadata
hook just to document `201` or `202` responses.

## Library Usage

The library mount API is useful for dev-time experiments, but the CLI is
recommended for production artifacts.

```go
package main

import (
	"github.com/fox-gonic/fox"
	"github.com/fox-gonic/openapi"
)

func main() {
	router := fox.Default()
	router.GET("/users/:id", getUser)

	spec := openapi.New(router,
		openapi.Info("My API", "1.0.0"),
		openapi.Server("https://api.example.com"),
		openapi.Source([]string{"."}),
		openapi.Operation("GET", "/users/:id", openapi.Tags("users")),
	)

	openapi.Mount(router, spec)
	router.Run(":8080")
}
```

By default, `openapi.Mount` registers `/openapi.yaml` and `/openapi.json`.
You can also write artifacts directly:

```go
if err := spec.WriteYAML(file); err != nil {
	panic(err)
}

yamlData, err := spec.YAML()
jsonData, err := spec.JSON()
```

Generation is best-effort. Non-fatal issues are collected as warnings:

```go
for _, warning := range spec.Warnings() {
	log.Println(warning)
}
```

## Generated Output

The generator covers:

- OpenAPI version `3.0.3`
- `info`, `servers`, top-level tags, and security schemes
- paths and methods from registered Fox routes
- Gin-style path parameters such as `/users/:id` as `/users/{id}`
- `uri`, `query`, `header`, `json`, and `form` request fields
- operation and schema descriptions from source comments
- explicit operation and group metadata
- JSON, form, string, empty, and error responses
- reusable component schemas with recursive `$ref` support
- custom type schema overrides through `openapi.RegisterFormatter`

Supported validation tags include `required`, `email`, `url`, `uri`, `uuid`,
`uuid4`, `min`, `max`, `gte`, `lte`, `gt`, `lt`, `len`, `oneof`, and
`alphanum`.

## CI

```yaml
- name: Generate OpenAPI spec
  run: fox-openapi generate
- name: Verify spec is up to date
  run: git diff --exit-code api/openapi.yaml
```

## Troubleshooting

- `entry is required`: no `entry` provided and auto-discovery found 0 or
  multiple candidates. Set `entry` in `fox-openapi.yaml`, pass `--entry`, or
  add `// fox-openapi:entry` to the canonical function.
- Exit code `2`: generated driver failed to build. Check imports, replaces, and entry/hook signatures.
- Exit code `3`: the driver built but failed at runtime. Check entry side effects or returned errors.
- Exit code `4`: `check` found drift; run `fox-openapi generate` and commit the updated spec.

## Current Limitations

The current implementation intentionally does not generate DomainEngine-specific
multi-host specs, custom schema naming overrides, or operation/group tag
assignment directly from YAML config. Use `metadataHook` for route-specific
metadata.

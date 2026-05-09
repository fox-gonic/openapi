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

For reproducible CI, pin the version used to generate committed specs. Replace
`vX.Y.Z` with the release you want the downstream repository to use:

```bash
go install github.com/fox-gonic/openapi/cmd/fox-openapi@vX.Y.Z
```

For local development in this repository:

```bash
go run ./cmd/fox-openapi version
```

## Quickstart

Expose one engine factory function that registers routes and returns a `*fox.Engine`.
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
used. If multiple are found, prefer annotating the canonical entry with a doc
comment marker:

```go
// NewEngine builds the production HTTP engine.
//
// fox-openapi:entry
func NewEngine() *fox.Engine { ... }
```

When at least one function carries the `fox-openapi:entry` marker, only
marked candidates are considered, so adding the marker disambiguates without
deleting other entry-shaped helpers.

`--entry` is still available for scripts, CI, and unusual layouts that need to
pin the function explicitly:

```bash
fox-openapi generate \
  --entry github.com/acme/myapp/internal/server.NewEngine
```

Supported entry signatures are:

```go
func NewEngine() *fox.Engine
func NewEngine() (*fox.Engine, error)
func NewEngine(context.Context) *fox.Engine
func NewEngine(context.Context) (*fox.Engine, error)
func NewEngine(context.Context, *Config) *fox.Engine
func NewEngine(context.Context, *Config) (*fox.Engine, error)
```

`serve` exposes `/openapi.yaml`, `/openapi.json`, `/docs`, `/scalar`, and
`/redoc` with embedded offline UI assets. It watches Go files by default and
regenerates the preview when source changes.

For small projects, no config file is required. Pass flags only when you want
to override output or metadata defaults:

```bash
fox-openapi \
  --out api/openapi.yaml \
  --title "Acme API"
```

Use `fox-openapi init` only when you want to commit a config file for shared
metadata such as title, servers, tags, security schemes, or entry config.

The CLI builds an isolated temporary driver for entry-based generation. For
basic generation, the application module does not need a `tools.go` file or a
committed direct `github.com/fox-gonic/openapi` requirement; the driver build
resolves that temporary dependency and restores `go.mod`/`go.sum` afterward.
Add a direct requirement only when application code imports OpenAPI metadata
hooks or library APIs.

## Route Manifest Mode

Since `v0.3.0`, fox-openapi can generate from a route manifest exported by the
application instead of calling the application entry through a generated
driver. Use this when `NewEngine` needs real runtime dependencies, configuration
objects, or environment setup that should not be recreated just for OpenAPI
generation.

The application owns when to write the manifest. A common pattern is to add a
non-production CLI flag next to normal startup code:

```go
routeManifestPath := flag.String("openapi-route-manifest", "", "write Fox route manifest and exit")
flag.Parse()

engine, err := NewEngine(ctx, cfg)
if err != nil {
	log.Fatal(err)
}

if *routeManifestPath != "" {
	if err := fox.WriteRouteManifest(engine, *routeManifestPath); err != nil {
		log.Fatal(err)
	}
	return
}

if err := engine.Run(":8080"); err != nil {
	log.Fatal(err)
}
```

Do not enable this flag in normal production startup. The tool reads the file;
it does not require the application to import `github.com/fox-gonic/openapi`.

Configure fox-openapi to consume the generated manifest:

```yaml
routeManifest: api/routes.manifest.json
out: api/openapi.yaml
info:
  title: Acme API
```

```bash
# First ask the application to write or refresh the manifest.
myapp --openapi-route-manifest api/routes.manifest.json

# Then ask fox-openapi to read the manifest and write the OpenAPI document.
fox-openapi generate --route-manifest api/routes.manifest.json --out api/openapi.yaml
```

Manifest mode does not run the application entry and does not update the
manifest file. It uses the existing manifest for methods, paths, handler
identities, path parameters, operation IDs, request/response schemas, and source
comment enrichment. If the manifest only contains handler symbols, fox-openapi
loads the application packages from `workdir` to enrich request and response
types, including aliases, generic wrappers, and handlers in `_test.go` when
`includeTestFiles` is enabled.

## Path resolution

Paths follow standard go-tooling conventions:

- **CLI flags** (`--out`, `--config`, `--workdir`, `--entry-config-path`): relative
  to the **current working directory** (where you invoked the command).
- **YAML fields** (`out`, `entryConfig.path`, `workdir`): relative to the
  **directory containing the config file**, so `fox-openapi.yaml` and the
  artifacts it points to keep a stable layout regardless of where you run.
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

## Filtered Specs

fox-openapi can derive narrower OpenAPI documents from the full generated
contract. The first supported CLI shape is intentionally simple: remove
operations whose extension has a specific scalar value, then optionally prune
components that are no longer referenced.

For example, handlers can mark internal operations in source comments:

```go
// List API keys.
//
// openapi:
//
//	x-public: false
func listAPIKeys(ctx *fox.Context) (ListAPIKeysResponse, error) {
	return ListAPIKeysResponse{}, nil
}
```

Then generate a public-only spec:

```bash
fox-openapi generate \
  --out api/public.openapi.yaml \
  --filter "x-public != false" \
  --filter "x-product = sandbox || x-product = account" \
  --prune-unused-components
```

The same settings can live in `fox-openapi.yaml`:

```yaml
out: api/public.openapi.yaml
filters:
  - x-public != false
  - x-product = sandbox || x-product = account
pruneUnusedComponents: true
```

The library API exposes the generic pipeline directly:

```go
spec := openapi.New(engine,
	openapi.WithFilters(
		openapi.FilterOperations(func(op openapi.OperationContext) bool {
			return op.ExtensionBoolDefault("x-public", true)
		}),
		openapi.PruneUnusedComponents(),
	),
)
```

Filtering is a post-generation step. Explicit metadata, inferred responses, and
source comment enrichment still happen first, so derived specs keep the same
contract semantics as the full document.

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
- `routeManifest`: read a Fox route manifest file instead of running an entry.
- `info`: `title`, `version`, `description`.
- `servers`: list of `url` and optional `description`.
- `tags`: top-level OpenAPI tag registry.
- `securitySchemes`: serializable HTTP, API key, OAuth2, or OpenID Connect schemes.
- `filters`: operation filter expressions. Supported operators are `=`, `==`,
  and `!=`. Use `||` inside one expression for OR; repeat filters to combine
  expressions with AND.
- `pruneUnusedComponents`: remove components no longer referenced after filters.
- `metadataHook`: optional advanced Go hook.
- `entryConfig`: optional `path` and `loader` for config-taking entries. When
  `path` is set, fox-openapi first looks for a package-level
  `Load(string) (*Config, error)` function in the config type's package; set
  `loader` only when loading needs a non-standard function. When omitted,
  config-taking entries receive `nil` for compatibility.

CLI flags override config values. Config values override defaults.

## Commands

```bash
fox-openapi init --entry internal/server.NewEngine --title "Acme API"
fox-openapi --out api/openapi.yaml --title "Acme API"
fox-openapi generate --entry github.com/acme/myapp/internal/server.NewEngine
fox-openapi generate --route-manifest api/routes.manifest.json --out api/openapi.yaml --title "Acme API"
fox-openapi check
fox-openapi serve --addr 127.0.0.1:8765
fox-openapi version
```

`fox-openapi`, `generate`, `check`, and `serve` share the common config flags:

- `--config`: config file path, default `fox-openapi.yaml`.
- `--entry`: explicitly pin the entry function when auto-discovery is not enough.
- `--out`: output path, default `api/openapi.yaml`.
- `--title` and `--version`: OpenAPI info metadata.
- `--server`: repeatable OpenAPI server URL.
- `--workdir`: user project root.
- `--filter`: repeatable operation filter expression, for example
  `--filter "x-public != false"`.
- `--prune-unused-components`: remove components no longer referenced after
  filters.

Advanced flags remain available for scripts and unusual projects but are hidden
from normal help: `--format`, `--source`, `--include-test-files`,
`--metadata-hook`, `--entry-config-loader`, `--entry-config-path`,
`--route-manifest`, `--keep-driver`, and `--verbose`.

`serve` also supports `--addr`, repeatable `--ui`, `--watch`, and `--open`.

## Metadata

The generator reads regular Go comments from source files to fill operation
summaries, operation descriptions, and schema field descriptions. It does not
require `doc` tags. Handler comments can also include an `openapi:` block for
operation-level metadata; when `summary` or `description` are omitted from the
block, the regular handler comment still provides them.

```go
type CreateUserRequest struct {
	// Display name for the new user.
	Name string `json:"name" binding:"required"`
}

// Create user.
//
// Creates a user and returns the persisted representation.
//
// openapi:
//   x-public: true
//   x-audience: external
func createUser(ctx *fox.Context, req CreateUserRequest) (UserResponse, error) {
	return UserResponse{}, nil
}
```

The `openapi:` block is removed from the generated description. It supports
OpenAPI extension fields such as `x-public` and `x-audience`, plus simple
operation fields such as `summary`, `description`, `operationId`, `tags`, and
`deprecated`.

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
		openapi.WithFilters(openapi.PruneUnusedComponents()),
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
- route manifest input when running the application is not desirable
- Gin-style path parameters such as `/users/:id` as `/users/{id}`
- `uri`, `query`, `header`, `json`, and `form` request fields
- operation and schema descriptions from source comments
- explicit operation and group metadata, including security and extensions
- inferred success responses, explicit responses, status wrappers, no-body
  success statuses, and default error responses
- reusable component schemas with recursive `$ref` support
- post-generation operation filters and unused component pruning
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

For manifest mode, refresh the application-owned manifest before generating:

```yaml
- name: Refresh route manifest
  run: go run ./cmd/myapp --openapi-route-manifest api/routes.manifest.json
- name: Generate OpenAPI spec
  run: fox-openapi generate --route-manifest api/routes.manifest.json --out api/openapi.yaml
- name: Verify spec is up to date
  run: git diff --exit-code api/routes.manifest.json api/openapi.yaml
```

## Troubleshooting

- `entry is required`: no `entry` provided and auto-discovery found 0 or
  multiple candidates. Set `entry` in `fox-openapi.yaml`, pass `--entry`, or
  add `// fox-openapi:entry` to the canonical function.
- Exit code `2`: generated driver failed to build. Check imports, replaces, and entry/hook signatures.
- Exit code `3`: the driver built but failed at runtime. Check entry side effects or returned errors.
- Exit code `4`: `check` found drift; run `fox-openapi generate` and commit the updated spec.
- `unsupported route manifest version`: regenerate the manifest with a compatible
  `github.com/fox-gonic/fox` version and rerun fox-openapi.

## Current Limitations

The current implementation intentionally does not generate DomainEngine-specific
multi-host specs or custom schema naming overrides. CLI filtering currently
supports scalar extension equality plus component pruning; use the Go filter API
for richer predicates such as path, method, operation ID, tags, or deprecation.
Use handler comment `openapi:` blocks for simple operation metadata and
`metadataHook` when metadata needs Go values.

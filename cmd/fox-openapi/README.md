# fox-openapi CLI

`fox-openapi` generates an OpenAPI 3.0.3 document from a Fox engine at build time. The normal path keeps business code free of OpenAPI imports: expose a constructor such as `NewEngine() *fox.Engine`, then point the CLI at it.

The CLI is built on Cobra, so every command supports consistent help output,
for example `fox-openapi generate --help`.

## Install

```bash
go install github.com/fox-gonic/openapi/cmd/fox-openapi@latest
```

For local development in this repository:

```bash
go run ./cmd/fox-openapi version
```

## Quickstart

Create `fox-openapi.yaml` in your application root:

```bash
fox-openapi init --entry internal/server.NewEngine --title "Acme API"
```

This writes a config like:

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

Then generate and check the committed spec:

```bash
fox-openapi generate
fox-openapi check
```

## Entry Function

`entry` must name an exported function with one of these signatures:

```go
func NewEngine() *fox.Engine
func NewEngine() (*fox.Engine, error)
func NewEngine(context.Context) *fox.Engine
func NewEngine(context.Context) (*fox.Engine, error)
func NewEngine(context.Context, *Config) (*fox.Engine, error)
```

The function should register routes and return the engine. It should not call `Run`, open listeners, or start background infrastructure that is not needed for route registration.
For config-taking entries, omit `entryConfig` to pass `nil` as the config
argument, or provide a loader:

```yaml
entryConfig:
  loader: github.com/acme/myapp/internal/config.Load
  path: config.yaml
```

The user module does not need to import or require `github.com/fox-gonic/openapi`
unless it uses a metadata hook or OpenAPI types directly.

## Config

Supported config keys:

- `entry`: required entry function.
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
- `autoAdd`: deprecated; the CLI no longer needs to add OpenAPI to the user module.

CLI flags override config values. Config values override defaults.
Security schemes are validated during config loading so missing required fields
fail before a generated driver is built.

For thin configs, use flags instead of a YAML file:

```bash
fox-openapi generate \
  --entry github.com/acme/myapp/internal/server.NewEngine \
  --out api/openapi.yaml \
  --title "Acme API" \
  --version 1.0.0 \
  --server https://api.example.com
```

`generate`, `check`, and `serve` share the common config flags:

- `--config`: config file path, default `fox-openapi.yaml`.
- `--entry`: entry function.
- `--out`: output path, default `api/openapi.yaml`.
- `--format`: `yaml` or `json`.
- `--title` and `--version`: OpenAPI info metadata.
- `--server`: repeatable OpenAPI server URL.
- `--source`: repeatable Go source path for comment extraction.
- `--include-test-files`: include `_test.go` files while scanning comments.
- `--metadata-hook`: optional `func() []openapi.Option`.
- `--entry-config-loader` and `--entry-config-path`: command-line form of `entryConfig`.
- `--workdir`: user project root.
- `--keep-driver`: keep the generated temporary driver for debugging.

`serve` also supports `--addr`, repeatable `--ui`, `--watch`, and `--open`.

## Metadata Hook

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

## Commands

```bash
fox-openapi init --entry internal/server.NewEngine --title "Acme API"
fox-openapi generate --entry github.com/acme/myapp/internal/server.NewEngine --out api/openapi.yaml --title "Acme API"
fox-openapi check
fox-openapi serve --addr 127.0.0.1:8765
fox-openapi version
```

`init` creates `fox-openapi.yaml`. Relative entries such as
`internal/server.NewEngine` are expanded with the module path from `go.mod`.
Pass `--force` to overwrite an existing config file. `init` supports
`--config`, `--entry`, `--out`, `--title`, `--version`, `--workdir`, and
`--force`.

`serve` exposes `/openapi.yaml`, `/openapi.json`, `/docs`, `/scalar`, and `/redoc`. UI pages are embedded and do not load CDN assets.

## CI

```yaml
- name: Generate OpenAPI spec
  run: go run ./cmd/fox-openapi generate --workdir examples/openapi-cli
- name: Verify spec is up to date
  run: git diff --exit-code examples/openapi-cli/api/openapi.yaml
```

## Troubleshooting

- `entry is required`: set `entry` in `fox-openapi.yaml` or pass `--entry`.
- Exit code `2`: generated driver failed to build. Check imports, replaces, and entry/hook signatures.
- Exit code `3`: the driver built but failed at runtime. Check entry side effects or returned errors.
- Exit code `4`: `check` found drift; run `fox-openapi generate` and commit the updated spec.

# Fox OpenAPI

OpenAPI 3.0.3 generator and CLI for [Fox](https://github.com/fox-gonic/fox).

The recommended workflow is `fox-openapi`: expose a function that builds a
`*fox.Engine`, then generate a committed `openapi.yaml` during development or
CI. Business code does not need to mount OpenAPI handlers or import this module
unless it uses the optional metadata hook.

## Install

```bash
go install github.com/fox-gonic/openapi/cmd/fox-openapi@latest
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
```

Then run:

```bash
fox-openapi generate
fox-openapi check
fox-openapi serve --addr 127.0.0.1:8765
```

`serve` exposes `/openapi.yaml`, `/openapi.json`, `/docs`, `/scalar`, and
`/redoc` with embedded offline UI assets.

For small projects you can skip the config file and pass the same settings as
flags:

```bash
fox-openapi generate \
  --entry github.com/acme/myapp/internal/server.NewEngine \
  --out api/openapi.yaml \
  --title "Acme API"
```

The CLI builds an isolated temporary driver, so the application module does not
need a `tools.go` file or a direct `github.com/fox-gonic/openapi` requirement
unless it uses OpenAPI metadata hooks.

See [cmd/fox-openapi/README.md](cmd/fox-openapi/README.md) for CLI details and
[docs/openapi.md](docs/openapi.md) for library usage.

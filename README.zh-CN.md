# Fox OpenAPI

[English](README.md) | 简体中文

面向 [Fox](https://github.com/fox-gonic/fox) 的 OpenAPI 3.0.3 生成器和 CLI。

推荐使用 `fox-openapi`：在业务项目中暴露一个创建 `*fox.Engine` 的函数，然后在开发阶段或 CI 中生成并提交 `openapi.yaml`。业务代码不需要挂载 OpenAPI handler，也不需要直接 import 本模块，除非使用可选的 OpenAPI metadata hook 或 library API。

## 安装

```bash
go install github.com/fox-gonic/openapi/cmd/fox-openapi@latest
```

在本仓库内开发时，可以直接运行：

```bash
go run ./cmd/fox-openapi version
```

## 快速开始

先暴露一个 entry 函数，用来注册路由并返回 `*fox.Engine`。这个函数不应该调用 `Run`、监听端口，或启动和路由注册无关的基础设施。

```go
package server

import "github.com/fox-gonic/fox"

func NewEngine() *fox.Engine {
	engine := fox.New()
	engine.GET("/users/:id", getUser)
	return engine
}
```

在业务项目根目录创建 `fox-openapi.yaml`：

```bash
fox-openapi init --entry internal/server.NewEngine --title "Acme API"
```

然后生成、校验并预览已提交的 spec：

```bash
fox-openapi generate
fox-openapi check
fox-openapi serve --addr 127.0.0.1:8765
```

`serve` 会暴露 `/openapi.yaml`、`/openapi.json`、`/docs`、`/scalar` 和 `/redoc`，并使用内置的离线 UI 资源。

如果项目配置很简单，也可以不创建配置文件，直接通过 flags 传入：

```bash
fox-openapi generate \
  --entry github.com/acme/myapp/internal/server.NewEngine \
  --out api/openapi.yaml \
  --title "Acme API"
```

CLI 会构建一个隔离的临时 driver，因此业务模块不需要 `tools.go` 文件，也不需要直接声明 `github.com/fox-gonic/openapi` 依赖，除非使用 OpenAPI metadata hook。

## Entry 函数

`entry` 必须指向一个导出函数，并符合以下签名之一：

```go
func NewEngine() *fox.Engine
func NewEngine() (*fox.Engine, error)
func NewEngine(context.Context) *fox.Engine
func NewEngine(context.Context) (*fox.Engine, error)
func NewEngine(context.Context, *Config) *fox.Engine
func NewEngine(context.Context, *Config) (*fox.Engine, error)
```

对于接收配置的 entry，可以省略 `entryConfig`，此时 CLI 会把配置参数传为 `nil`；也可以提供一个配置 loader：

```yaml
entryConfig:
  loader: github.com/acme/myapp/internal/config.Load
  path: config.yaml
```

传入 `nil` 可以让生产代码和 OpenAPI 生成共用同一个路由注册入口，同时避免初始化数据库或外部服务。

## 配置

`fox-openapi init` 会生成类似下面的配置：

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

支持的配置项：

- `entry`：必填，entry 函数。
- `out`：输出文件，默认 `api/openapi.yaml`。
- `format`：`yaml` 或 `json`；未配置时根据 `out` 后缀推断。
- `sources`：用于提取 Go 注释的源码路径，默认 `./...`。
- `includeTestFiles`：扫描源码注释时是否包含 `_test.go` 文件。
- `info`：`title`、`version`、`description`。
- `servers`：OpenAPI server 列表，每项包含 `url` 和可选的 `description`。
- `tags`：顶层 OpenAPI tag registry。
- `securitySchemes`：可序列化的 HTTP、API key、OAuth2 或 OpenID Connect security scheme。
- `metadataHook`：可选的高级 Go hook。
- `entryConfig`：接收配置的 entry 使用的可选 `loader` 和 `path`。

CLI flags 会覆盖配置文件，配置文件会覆盖默认值。

## 命令

```bash
fox-openapi init --entry internal/server.NewEngine --title "Acme API"
fox-openapi generate --entry github.com/acme/myapp/internal/server.NewEngine --out api/openapi.yaml --title "Acme API"
fox-openapi check
fox-openapi serve --addr 127.0.0.1:8765
fox-openapi version
```

`generate`、`check` 和 `serve` 共用以下配置 flags：

- `--config`：配置文件路径，默认 `fox-openapi.yaml`。
- `--entry`：entry 函数。
- `--out`：输出路径，默认 `api/openapi.yaml`。
- `--format`：`yaml` 或 `json`。
- `--title` 和 `--version`：OpenAPI info metadata。
- `--server`：可重复传入的 OpenAPI server URL。
- `--source`：可重复传入的 Go 源码路径，用于提取注释。
- `--include-test-files`：扫描源码注释时包含 `_test.go` 文件。
- `--metadata-hook`：可选的 `func() []openapi.Option`。
- `--entry-config-loader` 和 `--entry-config-path`：`entryConfig` 的命令行形式。
- `--workdir`：业务项目根目录。
- `--keep-driver`：保留生成的临时 driver，便于调试。

`serve` 还支持 `--addr`、可重复传入的 `--ui`、`--watch` 和 `--open`。

## Metadata

生成器会读取源码中的普通 Go 注释，用来填充 operation summary、operation description 和 schema field description。不需要额外的 `doc` tag。

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

如果 metadata 需要 Go value，可以添加一个小的可选 hook：

```go
func ConfigureOpenAPI() []openapi.Option {
	return []openapi.Option{
		openapi.Group("/users", openapi.Tags("users")),
		openapi.Operation("GET", "/users/:id", openapi.Security("BearerAuth")),
	}
}
```

然后在配置中指定：

```yaml
metadataHook: github.com/acme/myapp/internal/openapimeta.ConfigureOpenAPI
```

## Library 用法

Library mount API 适合开发阶段实验，但生产 artifact 推荐使用 CLI 生成。

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

默认情况下，`openapi.Mount` 会注册 `/openapi.yaml` 和 `/openapi.json`。也可以直接写出 artifact：

```go
if err := spec.WriteYAML(file); err != nil {
	panic(err)
}

yamlData, err := spec.YAML()
jsonData, err := spec.JSON()
```

生成过程是 best-effort 的，非致命问题会被收集为 warnings：

```go
for _, warning := range spec.Warnings() {
	log.Println(warning)
}
```

## 生成内容

生成器覆盖以下内容：

- OpenAPI version `3.0.3`
- `info`、`servers`、顶层 tags 和 security schemes
- 从已注册 Fox routes 中提取 paths 和 methods
- 将 `/users/:id` 这样的 Gin 风格路径参数转换为 `/users/{id}`
- `uri`、`query`、`header`、`json` 和 `form` 请求字段
- 从源码注释提取 operation 和 schema 描述
- 显式 operation 和 group metadata
- JSON、form、string、empty 和 error responses
- 可复用的 component schemas，并支持递归 `$ref`
- 通过 `openapi.RegisterFormatter` 覆盖自定义类型 schema

支持的 validation tags 包括 `required`、`email`、`url`、`uri`、`uuid`、`uuid4`、`min`、`max`、`gte`、`lte`、`gt`、`lt`、`len`、`oneof` 和 `alphanum`。

## CI

```yaml
- name: Generate OpenAPI spec
  run: fox-openapi generate
- name: Verify spec is up to date
  run: git diff --exit-code api/openapi.yaml
```

## 故障排查

- `entry is required`：在 `fox-openapi.yaml` 中设置 `entry`，或通过 `--entry` 传入。
- Exit code `2`：生成的 driver 构建失败。检查 imports、replaces、entry 签名和 hook 签名。
- Exit code `3`：driver 构建成功，但运行失败。检查 entry 的副作用或返回的错误。
- Exit code `4`：`check` 发现 spec drift；运行 `fox-openapi generate` 并提交更新后的 spec。

## 当前限制

当前实现有意不生成 DomainEngine 专用的多 host specs、自定义 schema 命名覆盖，也不支持直接从 YAML 配置为 operation 或 group 分配 tags。路由级 metadata 请使用 `metadataHook`。

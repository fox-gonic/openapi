# Fox OpenAPI

[English](README.md) | 简体中文

面向 [Fox](https://github.com/fox-gonic/fox) 的 OpenAPI 3.0.3 生成器和 CLI。

推荐使用 `fox-openapi`：在业务项目中暴露一个创建 `*fox.Engine` 的函数，然后在开发阶段或 CI 中生成并提交 `openapi.yaml`。业务代码不需要挂载 OpenAPI handler，也不需要直接 import 本模块，除非使用可选的 OpenAPI metadata hook 或 library API。

## 安装

```bash
go install github.com/fox-gonic/openapi/cmd/fox-openapi@latest
```

CI 中建议固定生成器版本，保证提交的 spec 可复现。请把 `vX.Y.Z` 替换为下游仓库实际使用的版本：

```bash
go install github.com/fox-gonic/openapi/cmd/fox-openapi@vX.Y.Z
```

在本仓库内开发时，可以直接运行：

```bash
go run ./cmd/fox-openapi version
```

## 快速开始

先暴露一个 engine factory 函数，用来注册路由并返回 `*fox.Engine`。这个函数不应该调用 `Run`、监听端口，或启动和路由注册无关的基础设施。

```go
package server

import "github.com/fox-gonic/fox"

func NewEngine() *fox.Engine {
	engine := fox.New()
	engine.GET("/users/:id", getUser)
	return engine
}
```

然后生成、校验并预览已提交的 spec：

```bash
fox-openapi                                # 自动从 ./... 中发现 entry
fox-openapi ./internal/server              # 将 entry 发现范围限定到指定目录
fox-openapi check
fox-openapi serve --addr 127.0.0.1:8765
```

当配置中省略 `entry` 且未传 `--entry` 时，CLI 会扫描 `sources`（默认 `./...`），查找一个签名符合 entry 形状的导出函数。如果只匹配到一个就直接使用；匹配到多个则报错并列出所有候选。推荐在标准入口的 doc 注释中添加标记：

```go
// NewEngine 构建生产环境 HTTP engine。
//
// fox-openapi:entry
func NewEngine() *fox.Engine { ... }
```

只要至少有一个候选携带 `fox-openapi:entry` 标记，就只考虑被标记的候选，因此可以在不删除其他 entry 形状辅助函数的情况下消除歧义。

`--entry` 仍然保留给脚本、CI 和特殊目录布局，用来显式固定入口函数：

```bash
fox-openapi generate \
  --entry github.com/acme/myapp/internal/server.NewEngine
```

支持的 entry 签名包括：

```go
func NewEngine() *fox.Engine
func NewEngine() (*fox.Engine, error)
func NewEngine(context.Context) *fox.Engine
func NewEngine(context.Context) (*fox.Engine, error)
func NewEngine(context.Context, *Config) *fox.Engine
func NewEngine(context.Context, *Config) (*fox.Engine, error)
```

`serve` 会暴露 `/openapi.yaml`、`/openapi.json`、`/docs`、`/scalar` 和 `/redoc`，并使用内置的离线 UI 资源。它默认监听 Go 文件变化，并在源码变化后重新生成预览。

配置简单的项目不需要创建配置文件。只有想覆盖输出路径或 metadata 默认值时才需要传 flags：

```bash
fox-openapi \
  --out api/openapi.yaml \
  --title "Acme API"
```

只有在需要提交 title、servers、tags、security schemes 或 entry config 等共享 metadata 时，才需要使用 `fox-openapi init` 创建配置文件。

基于 entry 生成时，CLI 会构建一个隔离的临时 driver。基础生成场景下，业务模块不需要 `tools.go` 文件，也不需要提交直接的 `github.com/fox-gonic/openapi` 依赖；driver 构建会解析这个临时依赖，并在结束后恢复 `go.mod` / `go.sum`。只有业务代码自己 import OpenAPI metadata hook 或 library API 时，才需要直接声明依赖。

## Route Manifest 模式

从 `v0.3.0` 开始，fox-openapi 可以读取业务应用导出的 route manifest 来生成 OpenAPI，而不是通过临时 driver 调用应用 entry。当 `NewEngine` 依赖真实运行时对象、配置对象或环境初始化，不适合为了 OpenAPI 额外复刻时，推荐使用这个模式。

manifest 文件由业务应用自己决定什么时候写入。常见做法是在正常启动逻辑旁边增加一个非生产用途的 CLI flag：

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

不要在正常生产启动路径中启用这个 flag。fox-openapi 只读取这个文件；业务应用不需要 import `github.com/fox-gonic/openapi`。

然后配置 fox-openapi 读取生成好的 manifest：

```yaml
routeManifest: api/routes.manifest.json
out: api/openapi.yaml
info:
  title: Acme API
```

```bash
# 先让业务应用写入或刷新 manifest。
myapp --openapi-route-manifest api/routes.manifest.json

# 再让 fox-openapi 读取 manifest 并写出 OpenAPI 文档。
fox-openapi generate --route-manifest api/routes.manifest.json --out api/openapi.yaml
```

Manifest 模式不会运行应用 entry，也不会更新 manifest 文件。它会使用已有 manifest 中的方法、路径、handler 标识、path 参数、operationId、request / response schema，并继续结合源码注释补全文档。如果 manifest 里只有 handler symbol，fox-openapi 会从 `workdir` 加载业务包来补全 request / response 类型，包括 alias、泛型 wrapper，以及开启 `includeTestFiles` 时定义在 `_test.go` 中的 handler。

## 路径解析

路径解析遵循标准 Go 工具链约定：

- **CLI flags**（`--out`、`--config`、`--workdir`、`--entry-config-path`）：相对于**当前工作目录**（执行命令时所在的目录）。
- **YAML 字段**（`out`、`entryConfig.path`、`workdir`）：相对于**配置文件所在目录**，这样 `fox-openapi.yaml` 与它指向的产物之间始终保持稳定的相对位置，无论从哪里运行命令。
- **位置参数**（`fox-openapi generate ./internal/aone`）：仅用于**限定 entry 函数的发现范围**。它**不会**收窄源码扫描 —— `sources`（默认 `./...`）依然驱动注释提取，避免子包中的字段/handler 注释丢失。如需显式覆盖扫描范围，请在 YAML 中设置 `sources` 或传 `--source`。

```bash
cd ~/myapp
fox-openapi generate internal/aone --out api/openapi.yaml
# 写入 ~/myapp/api/openapi.yaml   ← 相对于 CWD，而非被扫描的目录
```

## 过滤后的规格

fox-openapi 可以从完整 OpenAPI contract 中派生更窄的文档。第一版 CLI 形态刻意保持简单：删除某个 extension 等于指定标量值的 operation，然后可选地裁剪不再被引用的 components。

例如，可以在 handler 注释中标记内部 operation：

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

然后生成 public-only spec：

```bash
fox-openapi generate \
  --out api/public.openapi.yaml \
  --filter "x-public != false" \
  --filter "x-product = sandbox || x-product = account" \
  --prune-unused-components
```

同样的设置也可以写进 `fox-openapi.yaml`：

```yaml
out: api/public.openapi.yaml
filters:
  - x-public != false
  - x-product = sandbox || x-product = account
pruneUnusedComponents: true
```

Library API 会直接暴露通用过滤 pipeline：

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

过滤发生在生成之后。显式 metadata、响应推断和源码注释补全都会先执行，因此派生规格会保留和完整文档一致的 contract 语义。

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

- `entry`：entry 函数。可省略 —— 省略时 CLI 会从 `sources` 中自动发现一个签名符合的导出函数。显式设置可覆盖自动发现，也可通过源码注释中的 `// fox-openapi:entry` 进行选择。
- `out`：输出文件，默认 `api/openapi.yaml`。
- `format`：`yaml` 或 `json`；未配置时根据 `out` 后缀推断。
- `sources`：用于提取 Go 注释的源码路径，默认 `./...`。
- `includeTestFiles`：扫描源码注释时是否包含 `_test.go` 文件。
- `routeManifest`：读取 Fox route manifest 文件，而不是运行 entry。
- `info`：`title`、`version`、`description`。
- `servers`：OpenAPI server 列表，每项包含 `url` 和可选的 `description`。
- `tags`：顶层 OpenAPI tag registry。
- `securitySchemes`：可序列化的 HTTP、API key、OAuth2 或 OpenID Connect security scheme。
- `filters`：operation 过滤表达式。支持的操作符为 `=`、`==` 和 `!=`；
  单个表达式内部可用 `||` 表示 OR，多个 filter 会以 AND 组合。
- `pruneUnusedComponents`：过滤后删除不再被引用的 components。
- `metadataHook`：可选的高级 Go hook。
- `entryConfig`：接收配置的 entry 使用的可选 `path` 和 `loader`。设置
  `path` 后，fox-openapi 会先在配置类型所在包中查找包级
  `Load(string) (*Config, error)` 函数；只有需要非标准加载函数时才需要设置
  `loader`。完全省略时，接收配置的 entry 会收到 `nil`，以兼容已有项目。

CLI flags 会覆盖配置文件，配置文件会覆盖默认值。

## 命令

```bash
fox-openapi init --entry internal/server.NewEngine --title "Acme API"
fox-openapi --out api/openapi.yaml --title "Acme API"
fox-openapi generate --entry github.com/acme/myapp/internal/server.NewEngine
fox-openapi generate --route-manifest api/routes.manifest.json --out api/openapi.yaml --title "Acme API"
fox-openapi check
fox-openapi serve --addr 127.0.0.1:8765
fox-openapi version
```

`fox-openapi`、`generate`、`check` 和 `serve` 共用以下常用配置 flags：

- `--config`：配置文件路径，默认 `fox-openapi.yaml`。
- `--entry`：自动发现不够用时，显式固定 entry 函数。
- `--out`：输出路径，默认 `api/openapi.yaml`。
- `--title` 和 `--version`：OpenAPI info metadata。
- `--server`：可重复传入的 OpenAPI server URL。
- `--workdir`：业务项目根目录。
- `--filter`：可重复传入的 operation 过滤表达式，例如
  `--filter "x-public != false"`。
- `--prune-unused-components`：过滤后删除不再被引用的 components。

高级 flags 仍然保留给脚本和特殊项目使用，但默认 help 中隐藏：`--format`、
`--source`、`--include-test-files`、`--metadata-hook`、`--entry-config-loader`、
`--entry-config-path`、`--route-manifest`、`--keep-driver` 和 `--verbose`。

`serve` 还支持 `--addr`、可重复传入的 `--ui`、`--watch` 和 `--open`。

## Metadata

生成器会读取源码中的普通 Go 注释，用来填充 operation summary、operation
description 和 schema field description。不需要额外的 `doc` tag。handler 注释也可以包含
`openapi:` 块来提供 operation 级 metadata；如果块里没有写 `summary` 或
`description`，仍会使用 handler 原本的普通注释。

```go
type CreateUserRequest struct {
	// 新用户的展示名称。
	Name string `json:"name" binding:"required"`
}

// 创建用户。
//
// 创建用户并返回持久化后的表示。
//
// openapi:
//   x-public: true
//   x-audience: external
func createUser(ctx *fox.Context, req CreateUserRequest) (UserResponse, error) {
	return UserResponse{}, nil
}
```

`openapi:` 块不会出现在生成后的 description 中。它支持 `x-public`、
`x-audience` 等 OpenAPI extension 字段，以及 `summary`、`description`、
`operationId`、`tags`、`deprecated` 等简单 operation 字段。

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

显式成功响应会覆盖根据 handler 返回值推断出的默认成功响应。对于
`return statusResponse(http.StatusCreated, UserResponse{}), nil` 这类简单状态码
wrapper，`Source` 可以从 return 语句推断响应状态码，并使用 wrapper payload 类型作为
响应 schema。因此大多数 handler 不需要为了声明 `201` 或 `202` 响应额外编写
metadata hook。

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
		openapi.WithFilters(openapi.PruneUnusedComponents()),
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
- 当不适合运行应用时，可读取 route manifest 作为输入
- 将 `/users/:id` 这样的 Gin 风格路径参数转换为 `/users/{id}`
- `uri`、`query`、`header`、`json` 和 `form` 请求字段
- 从源码注释提取 operation 和 schema 描述
- 显式 operation 和 group metadata，包括 security 和 extensions
- 推断成功响应、显式响应、status wrapper、无 body 成功状态码和默认错误响应
- 可复用的 component schemas，并支持递归 `$ref`
- 生成后的 operation 过滤和未使用 component 裁剪
- 通过 `openapi.RegisterFormatter` 覆盖自定义类型 schema

支持的 validation tags 包括 `required`、`email`、`url`、`uri`、`uuid`、`uuid4`、`min`、`max`、`gte`、`lte`、`gt`、`lt`、`len`、`oneof` 和 `alphanum`。

## CI

```yaml
- name: Generate OpenAPI spec
  run: fox-openapi generate
- name: Verify spec is up to date
  run: git diff --exit-code api/openapi.yaml
```

Manifest 模式下，先刷新业务应用负责的 manifest，再生成 OpenAPI：

```yaml
- name: Refresh route manifest
  run: go run ./cmd/myapp --openapi-route-manifest api/routes.manifest.json
- name: Generate OpenAPI spec
  run: fox-openapi generate --route-manifest api/routes.manifest.json --out api/openapi.yaml
- name: Verify spec is up to date
  run: git diff --exit-code api/routes.manifest.json api/openapi.yaml
```

## 故障排查

- `entry is required`：未提供 `entry`，且自动发现匹配到 0 个或多个候选。请在 `fox-openapi.yaml` 中设置 `entry`、传 `--entry`，或在目标函数上添加 `// fox-openapi:entry` 标记。
- Exit code `2`：生成的 driver 构建失败。检查 imports、replaces、entry 签名和 hook 签名。
- Exit code `3`：driver 构建成功，但运行失败。检查 entry 的副作用或返回的错误。
- Exit code `4`：`check` 发现 spec drift；运行 `fox-openapi generate` 并提交更新后的 spec。
- `unsupported route manifest version`：使用兼容版本的 `github.com/fox-gonic/fox`
  重新生成 manifest，再运行 fox-openapi。

## 当前限制

当前实现有意不生成 DomainEngine 专用的多 host specs，也不提供自定义 schema 命名覆盖。CLI 过滤目前支持 scalar extension equality 和 component 裁剪；如果需要按 path、method、operation ID、tags 或 deprecated 等条件过滤，请使用 Go filter API。简单 operation metadata 可使用 handler 注释里的 `openapi:` 块；需要 Go value 时请使用 `metadataHook`。

# Status Wrapper OAPI Codegen Example

This example protects the integration path that caught the original
`StatusResponse[T]` issue:

```text
Fox handler -> fox-openapi CLI -> openapi.yaml -> oapi-codegen -> Go SDK types
```

The handlers return a generic render wrapper only to control success status
codes. The generated OpenAPI contract should expose the real wire responses:

- `POST /api/v1/sbx/sandboxes` returns `201` with `handler_SandboxResponse`.
- `POST /api/v1/sbx/templates` returns `202` with `handler_TemplateResponse`.
- No `StatusResponse[...]` wrapper schema should appear in the spec or generated
  SDK code.

Generate the OpenAPI document:

```bash
go run github.com/fox-gonic/openapi/cmd/fox-openapi@latest \
  --workdir . \
  --entry github.com/fox-gonic/openapi/examples/status-wrapper-oapi-codegen/internal/handler.NewEngine \
  --out api/openapi.yaml \
  --title "Status Wrapper OAPI Codegen Example" \
  --version 1.0.0
```

Generate downstream SDK code:

```bash
go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.4.1 \
  --config oapi-codegen.yaml \
  api/openapi.yaml
```

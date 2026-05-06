# AI Agent Rules

This repository is an OpenAPI generator for Fox applications. When an AI agent
changes it, prioritize generated API correctness, low-friction business usage,
and reproducible downstream verification.

## Product Principles

- Prefer zero-configuration behavior for business projects. A normal handler
  should generate an accurate OpenAPI contract without requiring a metadata hook.
- Treat the OpenAPI document as the wire contract, not as a cosmetic artifact.
  Wrong status codes, empty schemas, or misleading response bodies are bugs even
  if the generated names only look inconvenient at first.
- Keep `metadataHook` as an advanced override. Do not make routine success
  responses, common render wrappers, or standard route metadata depend on hooks.
- Preserve CLI compatibility when simplifying commands. The root command may be
  friendlier, but existing subcommands and advanced flags should continue to work
  unless a breaking change is intentional and documented.

## OpenAPI Generation Rules

- Explicit response metadata must win over inferred responses. If an operation
  already has an explicit 2xx response, do not add an inferred default `200`.
- When a handler returns a render/status wrapper, infer the real HTTP status and
  body payload when this is clear from source. Do not expose implementation-only
  wrappers as response schemas.
- Do not generate JSON content for successful no-body statuses such as `204` and
  `205`.
- Avoid turning wrapper types that only implement rendering behavior into empty
  `type: object` schemas. Empty schemas for real response bodies need evidence.
- Remove unreferenced generated schemas when possible, especially schemas created
  only as intermediate inference artifacts.
- Schema name improvements are secondary to contract correctness. Short names do
  not help if the response status or body schema is wrong.

## Development Workflow

- Start from a clean branch for repository changes that will be reviewed through
  PR. If another workspace is dirty, use a separate branch or git worktree
  instead of mixing unrelated changes.
- Never revert user or pre-existing changes. If a dirty downstream repository
  contains unrelated work, inspect it, but isolate this task's changes before
  committing.
- Use focused commits. Generator changes, generated downstream specs, and SDK
  regeneration may belong in different repositories and usually need separate
  PRs.
- Keep generated files reproducible. After changing generation logic, regenerate
  affected fixtures or downstream specs with the exact intended command and
  verify that rerunning it produces no diff.
- For code review comments, verify the issue against the code and tests before
  changing behavior. Address actionable comments with minimal patches and rerun
  relevant tests.

## Verification Checklist

- Run `go test ./...` in this repository before claiming generator work is done.
- For CLI behavior, test both the compatibility path and the preferred path when
  applicable, for example both the root `fox-openapi` command and the `generate`
  subcommand.
- For response inference changes, include tests that cover explicit metadata,
  inferred wrapper status, no-body success statuses, and ambiguous wrappers.
- For downstream validation, check the generated OpenAPI for both absence of
  stale wrapper schemas and presence of the expected status/schema references.
- If a downstream SDK is regenerated, build the SDK package and search for stale
  generated type names before opening or updating a PR.
- Treat CI failures as real until logs prove otherwise. If a check fails because
  of repository configuration, record the exact log message and separate that
  from code-related failures.

## Downstream Coordination

- When releasing generator behavior needed by another repository, tag and push a
  version before regenerating downstream specs with
  `go run github.com/fox-gonic/openapi/cmd/fox-openapi@vX.Y.Z`.
- In downstream PRs, mention the generator version and the exact contract change
  being applied, such as `200 -> 201 (SandboxResponse)` or
  `200 -> 202 (TemplateResponse)`.
- Do not manually rename generated schema names in committed specs as the only
  fix. Change generator behavior first, then regenerate.
- If a downstream repository already has a large unrelated OpenAPI or SDK diff,
  create a clean worktree from `origin/main` to determine whether a separate PR is
  actually needed.

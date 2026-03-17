---
name: library-dev
description: Use when adding or modifying reusable Go libraries and utility code, especially under pkg/ or internal/. This skill fits tasks such as shared Redis locks, helpers, wrappers, or common infrastructure code. Do not use it for gRPC service or gRPC method implementation.
---

# Library Dev

## Overview

Use this skill for shared library work in `pkg/` and `internal/`.
Prefer the existing project style, keep the change set minimal, and avoid introducing new patterns unless the repository already uses them.

## Use This Skill

- Adding a reusable library in `pkg/` or `internal/`
- Modifying an existing shared library
- Implementing common helpers such as Redis-based utilities, wrappers, or internal support code

## Do Not Use This Skill

- Writing a gRPC service
- Writing a gRPC method
- Gateway route or protobuf contract work unless the task is only a supporting library change

## Workflow

1. Read the current repository version and release conventions first, including `go.mod` and the local project instructions.
2. If the target library already exists, read the current implementation and tests before changing it.
3. Reuse the style of nearby libraries first. Inspect similar code under `pkg/` or `internal/` before inventing a new shape.
4. Make only the minimum change required to complete the task. Do not do unrelated refactors, renames, or dependency swaps.
5. If the project already has a unified error style, keep using it. Do not introduce a new error model.
6. If tests already exist, extend them first. If the change is testable and the package has a test pattern, add or update tests.
7. After implementation, run `go test` for the touched package first, then follow the repository-level validation requirements.

## Library Rules

- Keep package names simple and avoid underscores when possible.
- Use strict Go CamelCase: `Id`, `Http`, `Ttl`.
- If a library under `pkg/xxx` has optional configuration, use the `WithXxx` pattern.
- If a shared library depends on Redis, accept `redis.UniversalClient`.
- If a shared library depends on the database, accept `bun.IDB`.
- Keep logging consistent with the repository style. Use `zerolog/log`, snake_case log keys, and lowercase English in `.Msg(...)`.
- For reusable sentinel errors such as `ErrNotFound`, declare them in `errors.go` instead of rebuilding the same error string repeatedly.

## Error Handling

- Follow the existing repository error style.
- When returning internal service-style errors, use patterns such as `errfmt.Errorf(codes.Internal, basepb.Code_CODE_INTERNAL_SERVER)` if that is what the surrounding code uses.
- For pure library-level errors, prefer shared sentinel errors when appropriate instead of introducing ad hoc formats.

## Validation

- Prefer the narrowest test command for the changed package, for example `go test ./pkg/xxx` or `go test ./internal/xxx`.
- If the task also changes broader application code, finish with the repository-required validation commands.

## Output

When reporting the result, prioritize:

- Which files changed
- Why the change was made this way
- Whether more tests or follow-up wiring are still needed

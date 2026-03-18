---
name: library-dev
description: Use when adding or modifying reusable Go libraries and utility code, especially under pkg/ or internal/. This skill fits library development and library maintenance tasks such as locks, helpers, wrappers, utilities, or common infrastructure code. Do not use it for gRPC service or gRPC method implementation.
---

# Library Dev

## Overview

Use this skill for shared library work in `pkg/` and `internal/`.
Prefer the existing project style, keep the change set minimal, and avoid introducing new patterns unless nearby code already uses them.

## Use This Skill

- Writing a new library in `pkg/` or `internal/`
- Modifying an existing library
- Implementing helpers, wrappers, utilities, or internal support code

## Do Not Use This Skill

- Writing a gRPC service
- Writing a gRPC method
- Gateway route or protobuf contract work unless the task is only a supporting library change

## Workflow

1. Confirm the requirement first and make sure the task is clear. If important details are unclear, ask targeted follow-up questions.
2. If the library already exists, read the current implementation and tests first to understand the existing behavior.
3. If the library does not exist yet, read other libraries in the same directory first and follow their style.
4. Make only the minimum change required to complete the task. Do not do unrelated refactors, renames, or dependency swaps.
5. Implement the library files needed for the task.
6. Shared sentinel errors may live in `errors.go` when that improves reuse and consistency.
7. `WithXxx` option helpers may live in `options.go` when the library has optional configuration.
8. Add thorough tests for the new or changed behavior.

## Library Rules

- Keep package names simple and avoid underscores when possible.
- Use strict Go CamelCase: `Id`, `Http`, `Ttl`.
- If a library under `pkg/xxx` has optional configuration, use the `WithXxx` pattern.
- If a shared library depends on Redis, accept `redis.UniversalClient`.
- If a shared library depends on the database, accept `bun.IDB`.
- Keep logging consistent with the repository style. Use `zerolog/log`, snake_case log keys, and lowercase English in `.Msg(...)`.
- For reusable sentinel errors such as `ErrNotFound`, declare them in `errors.go` instead of rebuilding the same error string repeatedly.
- Keep function comments in Chinese when the project expects function comments.

## Error Handling

- Follow the existing repository error style.
- When returning internal service-style errors, use patterns such as `errfmt.Errorf(codes.Internal, basepb.Code_CODE_INTERNAL_SERVER)` if that is what the surrounding code uses.
- For pure library-level errors, prefer shared sentinel errors when appropriate instead of introducing ad hoc formats.

## Validation

- Add or update complete tests for the library behavior you changed.
- Prefer the narrowest test command for the changed package, for example `go test ./pkg/xxx` or `go test ./internal/xxx`.

## Output

When reporting the result, prioritize:

- Which files changed
- Why the change was made this way
- Whether more tests or follow-up wiring are still needed

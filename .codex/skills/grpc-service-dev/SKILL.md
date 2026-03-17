---
name: grpc-service-dev
description: Use when implementing or modifying gRPC services and gRPC methods. This skill covers service-oriented work that may include protocol planning, model updates, migrations, service implementation, and tests. Do not use it for shared library work or for protocol-only changes with no service implementation.
---

# Grpc Service Dev

## Overview

Use this skill for end-to-end gRPC service and gRPC method development.
It is intended for tasks that land actual service behavior, not pure library work and not protocol-only edits.

## Use This Skill

- Writing a new gRPC service
- Writing a new gRPC method
- Modifying an existing gRPC service or method when the task includes service-side implementation

## Do Not Use This Skill

- Writing or modifying a shared library in `pkg/` or `internal/`
- Pure protobuf or protocol-only changes with no service implementation

## Workflow

1. First review similar existing APIs and service layouts in the repository.
2. Plan and write the protocol changes needed for the service task.
3. Implement or update the relevant models.
4. Add or update migrations if the data model changes.
5. Implement the concrete service in `services/xxx/xxx.go` or the matching existing service path.
6. Complete or extend the relevant test cases.
7. Run `make test` and `make all`.

## Service Rules

- Reuse the repository's existing service structure and naming before inventing new patterns.
- Keep changes minimal and task-focused. Avoid unrelated refactors or large renames.
- Follow the repository naming rules, including `Id`, `Http`, and `Ttl`.
- Keep database table names and field names in snake_case.
- Prefer existing error handling style and logging style used by nearby services.
- If the task touches protobuf definitions or generated API code, run `make api` before final validation.
- Check whether related updates belong in `apis`, `models`, `migrations`, `services`, `configs`, or `cmd`, and only touch the directories required by the task.

## Error Handling

- Preserve the existing project error model.
- When the surrounding service code uses the project-standard format, keep returning errors in the same style, for example `errfmt.Errorf(codes.Internal, basepb.Code_CODE_INTERNAL_SERVER)`.
- Do not introduce a new error abstraction unless the existing code already requires it.

## Validation

- Prefer adding or extending targeted tests first.
- Finish with `make test` and `make all`.
- If protobuf contracts changed, include `make api` before the final build and test commands.

## Output

When reporting the result, prioritize:

- Which files changed
- Why the change was made this way
- Whether more tests or route or wiring work are still needed

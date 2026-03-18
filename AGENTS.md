## Project

`simhub` is a Go project centered on gRPC and gRPC-Gateway workflows.

## Build And Test

- `make api`: compile protobuf definitions and generated API code
- `make all`: build the whole project
- `make test`: run test cases

Prefer the narrowest validation that matches the change first. After any code change, run `make test` and `make all`, and make sure both pass before finishing when practical.

## Dependencies

- Use `golang + grpc + grpc-gateway` for service and gateway workflows.
- Use `bun` as the database ORM.
- Use `redis/go-redis/v9` for Redis access.
- Use `go-json` instead of the standard `encoding/json` package.
- In shared libraries that depend on Redis, use the `redis.UniversalClient` interface.
- In shared libraries that depend on the database, use the `bun.IDB` interface.
- Use `github.com/rs/zerolog/log` for logging.
- Log keys must use snake_case.
- Log messages in `.Msg(...)` must be lowercase English.
- Prefer logging in the existing style, for example: `log.Ctx(ctx).Str("service_name", "name").Info().Msg("loaded")`

## Code Rules

- Follow standard Go project layout and keep changes minimal and task-focused.
- Before adding new module logic, inspect existing similar implementations and follow established patterns first.
- Do not perform drive-by refactors or unrelated renames.
- Package and library names should avoid underscores when possible.
- Use Go identifier casing `Http`, not `HTTP`
- Use Go identifier casing `Id`, not `ID`
- Use Go identifier casing `Ttl`, not `TTL`
- Use strict CamelCase consistently instead of abbreviation-style uppercase names.
- If writing shared libraries under `pkg/xxx`, optional configuration should use the `WithXxx` pattern.
- Functions should have comments, and function comments should be written in Chinese.
- Treat function comments as a hard requirement, not an optional cleanup item.
- Every newly added or modified function, including unexported helpers, must have a Chinese comment.
- For long functions or functions with multiple branches/stages, add short Chinese comments at key logic nodes so the control flow is easy to scan.
- Before finishing, check touched files for missing function comments and fill them in instead of leaving them for follow-up.

## Protobuf Style

- Protobuf service methods should use `ListXxx`, `CreateXxx`, `UpdateXxx`, and `RemoveXxx` naming patterns.
- Protobuf enum values should start from `1`, and `0` should be reserved.

## Database Style

- Database table names and field names must use snake_case.
- For example, use table names such as `users` and field names such as `id`, `label_flag`.
- For database models, if the model is `User`, the primary key field should be `Id` instead of `UserId` because the model name already provides the entity context.

## Error Handling

- Keep the existing error handling style.
- Use `errfmt.Errorf(codes.Internal, basepb.Code_CODE_INTERNAL_SERVER)` style when returning internal errors.
- `basepb` refers to the custom protobuf error code package already used by the project.
- Do not introduce a new error model unless the existing code already requires it.
- For shared libraries, reusable non-`fmt.Errorf` sentinel errors such as `ErrNotFound` should be declared in `errors.go`.

## Change Expectations

- Prefer the narrowest validation that matches the change.
- Run `make api` when protobuf contracts or generated API code are changed.
- After code changes, `make test` and `make all` must pass.
- After completing a change, summarize what was changed and why it was changed that way.

## Directory Rules

- The directories most commonly changed are `models` and `services`.
- `apis`: protobuf definitions and generated API-related contracts.
- `configs`: configuration files and configuration-related setup.
- `models`: data models and database-facing model definitions.
- `services`: concrete gRPC service implementations. Each gRPC service should live in its own subdirectory under `services`.
- `migrations`: SQL migration files.
- `pkg`: shared public libraries.
- `internal`: internal-only libraries and helpers.
- `cmd`: application entrypoints.
- `xxxd`: application initialization and assembly directories, which usually should not be changed unless the task clearly requires it.

## Available Skills

### library-dev

- Description: Use when adding or modifying reusable Go libraries and utility code, especially under `pkg/` or `internal/`. This skill fits shared library work such as Redis locks, wrappers, helpers, or common infrastructure code.
- Path: `.codex/skills/library-dev/SKILL.md`

### grpc-service-dev

- Description: Use when implementing or modifying gRPC services and gRPC methods, including service-oriented work that may involve models, migrations, service implementation, and tests.
- Path: `.codex/skills/grpc-service-dev/SKILL.md`

## Skill Usage

- Trigger `library-dev` for shared library work in `pkg/` or `internal/`.
- Do not use `library-dev` for gRPC service or gRPC method tasks.
- Trigger `grpc-service-dev` for gRPC service and gRPC method implementation tasks.
- Do not use `grpc-service-dev` for shared library work or protocol-only changes with no service implementation.
- Read only the referenced skill files you need.
- Follow the skill workflow before inventing new project conventions.

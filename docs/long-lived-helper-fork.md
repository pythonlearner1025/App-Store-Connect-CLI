# Long-Lived Helper Fork

## Goal

This fork adds a long-lived helper process for Blitz without turning the parent CLI into a hard-to-merge custom distribution.

The helper is designed for two different workloads:

- Hot App Store Connect API reads and writes from a persistent process.
- Full CLI compatibility as a fallback for commands that are already implemented in the upstream CLI.

## Why This Fork Exists

Blitz needs a process that stays alive so it can:

- Keep ASC auth and HTTP state warm.
- Avoid one new process per API request for interactive UI flows.
- Preserve access to the very broad App Store Connect CLI surface as upstream evolves.

The parent repo already has strong ASC coverage, but its root command tree uses `flag.ExitOnError` in many places. That makes in-process reuse of the full CLI unsafe for a long-lived helper because `--help` and some parse failures can terminate the whole process.

This fork solves that by splitting responsibilities:

- `session.request` uses a warm in-memory `asc.Client`.
- `cli.exec` runs the existing CLI in a child process of the helper binary.

## Architecture

### 1. `cmd/ascd`

`cmd/ascd` is the new helper binary.

- Normal mode: JSON-line server over stdin/stdout.
- Child CLI mode: re-enters the same binary with `__ascd_run_cli__` and delegates to `cmd.Run(...)`.

This keeps the CLI fallback on the exact same code version as the helper binary.

### 2. `internal/helper`

`internal/helper` contains the protocol and server implementation.

Supported methods:

- `ping`
- `session.open`
- `session.close`
- `session.request`
- `cli.exec`

Protocol transport:

- One JSON request per line on stdin.
- One JSON response per line on stdout.

### 3. `internal/asc/raw_request.go`

`internal/asc/raw_request.go` adds a generic authenticated request path on top of the existing `asc.Client`.

This is the long-lived fast path used by `session.request`.

It preserves:

- warm HTTP transport
- JWT caching
- mutating-request limiting

## Execution Model

### Fast Path: `session.request`

`session.open` resolves credentials once and constructs a persistent `asc.Client`.

`session.request` then sends arbitrary ASC REST requests through that client.

Use this for:

- app data loading
- metadata updates
- review submission data
- pricing checks
- anything UI-driven and latency-sensitive

### Compatibility Path: `cli.exec`

`cli.exec` intentionally does **not** call `cmd.Run` in-process.

Instead it:

1. Locates the current helper executable.
2. Spawns the same binary in child CLI mode.
3. Forwards args.
4. Captures stdout, stderr, and exit code.
5. Returns them in JSON.

This keeps compatibility with the upstream CLI surface, including help paths and command trees that rely on process exit semantics.

## Files That Own The Fork

Keep the fork concentrated in these areas:

- `cmd/ascd/`
- `internal/helper/`
- `internal/asc/raw_request.go`

Avoid modifying these unless absolutely necessary:

- `cmd/root.go`
- `internal/cli/registry/registry.go`
- large command packages under `internal/cli/*`

Those files change frequently upstream and are where merge pain grows.

## Test Strategy

### Raw Request Tests

`internal/asc/raw_request_test.go` verifies:

- authenticated generic requests
- header restrictions
- absolute URL safety checks

### Helper Tests

`internal/helper/server_test.go` verifies:

- server request/response behavior
- session handling
- protocol serialization

### Full CLI Surface Compatibility Test

`internal/helper/cli_surface_test.go` walks the live `ffcli` command tree and verifies that helper `cli.exec` matches direct child CLI execution for:

- root invocation
- root `--help`
- root `--version`
- every discovered command path with `--help`

This is the main regression guard that keeps the helper compatible with the parent CLI as new commands are added.

Notes:

- The test is intentionally expensive.
- It is skipped in `testing.Short()` mode.

## Build And Verify

From the repo root:

```bash
go test ./internal/asc ./internal/helper
go build ./...
go build -o build/ascd ./cmd/ascd
```

Basic smoke tests:

```bash
printf '{"id":"1","method":"ping"}\n' | ./build/ascd
printf '{"id":"2","method":"cli.exec","params":{"args":["--version"]}}\n' | ./build/ascd
```

Credentialed smoke test for the persistent ASC session:

```bash
ASC_BYPASS_KEYCHAIN=1 \
ASC_KEY_ID=... \
ASC_ISSUER_ID=... \
ASC_PRIVATE_KEY_PATH=... \
printf '{"id":"3","method":"session.open"}\n' | ./build/ascd
```

## Git Remote Layout

This fork is intended to use:

- `origin`: `https://github.com/pythonlearner1025/App-Store-Connect-CLI.git`
- `upstream`: `https://github.com/rudrankriyam/App-Store-Connect-CLI.git`

## Upstream Maintenance Workflow

### Normal Update Flow

```bash
git fetch upstream
git checkout blitz-helper
git merge upstream/main
```

If you prefer rebasing:

```bash
git fetch upstream
git checkout blitz-helper
git rebase upstream/main
```

### After Every Upstream Sync

Run:

```bash
go test ./internal/asc ./internal/helper
go build ./...
```

Then smoke the helper:

```bash
printf '{"id":"1","method":"ping"}\n' | ./build/ascd
printf '{"id":"2","method":"cli.exec","params":{"args":["--version"]}}\n' | ./build/ascd
```

### What Usually Breaks

If upstream changes break this fork, the most likely touchpoints are:

- auth resolution in `internal/cli/shared`
- client construction in `internal/asc`
- child CLI behavior if `cmd.Run` semantics change

The desired fix pattern is:

1. keep behavior changes inside `internal/helper`
2. keep raw ASC extensions inside `internal/asc/raw_request.go`
3. avoid reworking the root CLI registry unless there is no alternative

## Design Rule

Treat this fork as a thin adapter around upstream, not a second CLI product.

If a new Blitz need can be handled by:

- `session.request`
- a small additive helper method
- or child-process `cli.exec`

prefer that over modifying the parent command tree.

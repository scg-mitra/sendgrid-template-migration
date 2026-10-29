# AGENTS.md

Context and coding standards for AI agents working in this Go repository.

---

## 1. Core Development Commands

Run these exact commands before committing any changes:

- **Format & Tidy**: `go fmt ./... && go mod tidy`
- **Static Analysis & Linting**: `golangci-lint run ./...`
- **Unit Tests (with Race Detector)**: `go test -race -v ./...`
- **Single Package Tests**: `go test -race -v ./internal/mypackage/...`
- **Benchmarking**: `go test -benchmem -run=^$ -bench=. ./...`
- **Build**: `go build -v -o /dev/null ./cmd/...`

---

## 2. Project Architecture & Packaging

- **Directory Layout**:
  - `cmd/`: Entry points only (thin `main.go` files responsible for config parsing, dependency wiring, and OS signal shutdown).
  - `internal/`: All application business logic, domain services, storage, and transport layers. Never expose code here externally.
  - `pkg/`: Only code explicitly intended to be imported by other third-party projects.
- **Package Design**:
  - Name packages by what they provide, not what they contain (e.g., `user`, `postgres`, `httpclient`, never `utils`, `helpers`, `common`, or `models`).
  - Avoid cyclic dependencies; use unidirectional dependency flow (`cmd` -> `transport` -> `service` -> `repository`).

---

## 3. Idiomatic Go Conventions

### Interfaces
- **Accept interfaces, return structs**: Functions and constructors should take interfaces as parameters and return concrete types (`*MyService`).
- **Keep interfaces small**: Target 1 to 3 methods (e.g., `io.Reader`, `io.Closer`). Define interfaces on the **consumer** side, not on the provider/producer side.

### Error Handling
- **Wrap errors with context**: Always wrap when bubbling up: `fmt.Errorf("fetching user %s: %w", id, err)`.
- **Inspection**: Use `errors.Is(err, ErrNotFound)` for sentinel errors and `errors.As(err, &targetErr)` for typed errors. Never match error strings with `strings.Contains`.
- **Zero Panic Policy**: Never use `panic` or `recover` in application code or library packages. Return explicit errors. `panic` is only permissible in `cmd/` entrypoints or `init()` for static regex/template compilation failures.

### Concurrency & Context
- **Context First**: Pass `ctx context.Context` as the very first argument of functions doing I/O, database queries, or long computations. Never store `context.Context` inside a struct.
- **Goroutine Lifecycles**: Never launch a fire-and-forget goroutine (`go func()`). Every goroutine must have an explicit exit trigger (via `context.Done()`) and be coordinated with `sync.WaitGroup` or `golang.org/x/sync/errgroup`.
- **Race Detection**: Always run tests with `-race`. Protect shared mutable state with `sync.Mutex` / `sync.RWMutex` or channel-based actor models.

### Memory & Performance
- **Preallocate collections**: Always provide capacity hints when size is known: `make([]T, 0, len(items))` and `make(map[string]T, len(items))`.
- **Pointers vs. Values**:
  - Pass structs by value for small, immutable data (<= 64 bytes).
  - Use pointers when mutation is necessary, for large structs, or when the struct contains a lock (`sync.Mutex`). Never copy a struct containing a mutex.

---

## 4. Code Pattern Examples

### Table-Driven Tests
```go
func TestParseVersion(t *testing.T) {
    t.Parallel()

    tests := []struct {
        name    string
        input   string
        want    Version
        wantErr bool
    }{
        {name: "valid semver", input: "v1.2.3", want: Version{Major: 1, Minor: 2, Patch: 3}, wantErr: false},
        {name: "empty string", input: "", want: Version{}, wantErr: true},
    }

    for _, tt := range tests {
        tt := tt // pin variable for parallel test runs in older go versions
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()

            got, err := ParseVersion(tt.input)
            if (err != nil) != tt.wantErr {
                t.Fatalf("ParseVersion() error = %v, wantErr %v", err, tt.wantErr)
            }
            if got != tt.want {
                t.Errorf("ParseVersion() = %+v, want %+v", got, tt.want)
            }
        })
    }
}
```

### Resource Cleanup
```go
// Always check error on acquisition before deferring cleanup
f, err := os.Open(path)
if err != nil {
    return fmt.Errorf("opening file: %w", err)
}
defer f.Close()
```

---

## 5. Changelog & Releases

`CHANGELOG.md` follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Each version's section is published as its GitHub Release notes, and a tag build fails if the section is missing or empty.

- **Record every user-facing change** under `## [Unreleased]` in the same commit as the change.
- **Group entries** under `### Added`, `### Changed`, `### Fixed`, `### Removed` or `### Security`. Leave out empty headings.
- **Write for app users**: describe what changed for them, not how the code changed. One bullet per change, in plain language.
- **Skip internal-only changes**: refactors, tests, CI and tooling don't need entries unless they affect users.
- **Never edit released sections** unless asked. Never rename `[Unreleased]` to a version or create tags yourself; the maintainer cuts releases (see README → Releases).
- **Preview notes** with `packaging/release-notes.sh <version>`.

---

## 6. Agent Boundaries

| Category | Rules |
| :--- | :--- |
| **Always Do** | Run `golangci-lint` and `go test -race` on any modified package before returning. |
| | Add table-driven unit tests for every newly added function or modified edge case. |
| | Keep public exported identifiers documented with complete Go doc comments (`// FunctionName ...`). |
| | Add a `CHANGELOG.md` entry under `## [Unreleased]` for every user-facing change. |
| **Ask First** | Adding external dependencies to `go.mod` (prefer the Go standard library first). |
| | Changing exported function signatures in public API surfaces (`pkg/`). |
| | Creating or altering database schemas and migration scripts. |
| **Never Do** | Never swallow errors (`_ = doSomething()`) without an explicit comment explaining why. |
| | Never use `context.Background()` or `context.TODO()` in non-root functions. |
| | Never check in binaries, test cover profiles (`*.out`), or unformatted files. |
| | Never bypass lock synchronization to silence race detector warnings. |
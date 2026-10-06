---
name: golang-modernize
description: >-
  Modernizes Go codebases targeting Go 1.21-1.27+ standard library features, concurrency safety,
  testing patterns, and zero-allocation idioms. Use whenever modernizing Go code, updating Go idioms,
  addressing linter recommendations, or responding to /golang-modernize.
---

# Go Modernization & Idiomatic Standards (Go 1.21-1.27+)

## Overview

Modernize Go codebases methodically across four distinct phases:
1. **Phase 1: Safety & Correctness** (`math/rand/v2`, `errors.Is`/`errors.As`, safe path traversal).
2. **Phase 2: Language Features & Standard Library** (`any`, builtin `min`/`max`, `range` over int, `slices`, `maps`, `cmp.Or`).
3. **Phase 3: Modern Testing Patterns** (`t.Context()`, `t.Cleanup()`, `b.Loop()`).
4. **Phase 4: Tooling, CI Infrastructure & Allocation Optimization** (`.golangci.yaml`, `govulncheck`, `perfsprint` zero-allocation strings).

At every step, execute all project quality gates: `make test` (with race detector enabled), `make lint` (`golangci-lint`), `make modernize`, and `make vulncheck` (`govulncheck`).

---

## The 4-Phase Modernization Workflow

```
    PHASE 1                  PHASE 2                  PHASE 3                  PHASE 4
 Safety & Correctness     Language & Stdlib        Testing Patterns         Tooling & CI
   math/rand/v2        ──→   any, min/max      ──→   t.Context()        ──→   .golangci.yaml
   errors.Is/As              slices, maps, cmp       t.Cleanup()              govulncheck
```

---

### Phase 1: Safety & Correctness

1. **`math/rand/v2` (Go 1.22+)**:
   - Replace `import "math/rand"` with `import "math/rand/v2"`.
   - Never use `rand.Seed()`.
   - Use `rand.N(duration)` for typed random numbers/durations (e.g. jitter).
   - Use `rand.IntN(n)` instead of `rand.Intn(n)`.
2. **`errors.Is` & `errors.As` (Go 1.13+)**:
   - Never use `os.IsNotExist(err)` &rarr; use `errors.Is(err, os.ErrNotExist)`.
   - Never use `err == io.EOF` &rarr; use `errors.Is(err, io.EOF)`.
   - Replace type assertion checks `netErr, ok := err.(net.Error)` with `var netErr net.Error; if errors.As(err, &netErr) { ... }`.
3. **Path Traversal Safety**:
   - Prevent path traversal when loading user-provided or external file paths (e.g. using `filepath.Clean` and path verification).

---

### Phase 2: Language Features & Standard Library Upgrades

1. **`any` Alias (Go 1.18+)**:
   - Replace `interface{}` with `any` everywhere (struct fields, function signatures, map values).
2. **Builtin `min` and `max` (Go 1.21+)**:
   - Replace custom if-else clamping/bounding logic with `min(a, b)` and `max(a, b)`.
3. **`range` Over Integers (Go 1.22+)**:
   - Replace `for i := 0; i < n; i++` with `for i := range n`.
4. **Loop Variable Scoping Semantics (Go 1.22+)**:
   - Remove redundant shadow variable copies inside goroutines or closures (e.g., `v := v`).
5. **Standard Library `slices` Package (Go 1.21+)**:
   - Use `slices.Contains(s, v)` instead of custom search loops.
   - Use `slices.Clone(s)` instead of `append([]T(nil), s...)` or manual slice copy loops.
   - Use `slices.DeleteFunc(s, pred)` for clean in-place filtering.
   - Use `slices.Sort(s)` or `slices.Sorted(maps.Keys(m))` (Go 1.23+).
6. **Standard Library `maps` Package (Go 1.21+)**:
   - Use `maps.Clone(m)` instead of manual map allocation and iteration copy.
   - Use `maps.Copy(dst, src)` to merge maps.
7. **Default Fallbacks with `cmp.Or` (Go 1.22+)**:
   - Replace verbose fallback branches `if x == "" { x = defaultVal }` with `x = cmp.Or(x, defaultVal)`.

---

### Phase 3: Modern Testing Patterns (Go 1.24+)

1. **Automated Context Cancellation with `t.Context()`**:
   - Replace `context.Background()` or `context.WithCancel(...)` in test cases with `t.Context()`.
   - `t.Context()` automatically cancels when the test or subtest finishes, preventing background goroutine leaks.
2. **Explicit Teardowns with `t.Cleanup()`**:
   - Register teardown logic inside test helpers using `t.Cleanup(func() { ... })`.
   - **Crucial Rule**: Because `t.Context()` cancels before `t.Cleanup()` runs, if a cleanup action requires a context with a timeout (e.g., graceful server shutdown `srv.Stop(ctx)`), wrap it with `context.WithoutCancel`:
     ```go
     t.Cleanup(func() {
         stopCtx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 2*time.Second)
         defer cancel()
         _ = srv.Stop(stopCtx)
     })
     ```
3. **Environment and Benchmark Testing**:
   - Use `t.Setenv(key, val)` instead of manual `os.Setenv` + `defer os.Setenv`.
   - Use `b.Loop()` in benchmarks instead of `for i := 0; i < b.N; i++` (Go 1.24+).

---

### Phase 4: Tooling, CI Infrastructure & Allocation Optimization

1. **`golangci-lint` Configuration (`.golangci.yaml`)**:
   - Use v2 schema.
   - Enable `modernize`, `intrange`, `usestdlibvars`, `usetesting`, `perfsprint`, `errorlint`, `unconvert`, and `misspell`.
2. **Zero-Allocation String Operations (`perfsprint`)**:
   - Replace single-variable `fmt.Sprintf("%s_suffix", x)` with string concatenation `x + "_suffix"`.
   - Replace `fmt.Sprintf("http://%s", addr)` with `"http://" + addr`.
   - Replace static `fmt.Errorf("constant error")` with `errors.New("constant error")`.
3. **Vulnerability Scanning (`govulncheck`)**:
   - Add `govulncheck` to CI workflows (`golang/govulncheck-action@v1`).
   - Add `make vulncheck` target: `go run golang.org/x/vuln/cmd/govulncheck@latest ./...`.
4. **CI Pinning**:
   - When using Go 1.27+, ensure `golangci-lint` is pinned to `v2.13` or newer in `.github/workflows/` so it is built with compatible Go tooling.

---

## Verification Checklist

Before completing any modernization task:
- [ ] `make test`: All tests pass with race detector (`go test -v -race ./...`).
- [ ] `make lint`: Clean static analysis (`go vet` and `golangci-lint`).
- [ ] `make modernize`: Modernize linter passes with 0 issues.
- [ ] `make vulncheck`: 0 vulnerabilities reported.
- [ ] Atomic Conventional Commits (`fix:`, `refactor:`, `test:`, `ci:`).

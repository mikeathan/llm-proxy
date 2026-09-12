---
description: Staff-level Go backend and agentic workflow engineering guide optimized for AI coding assistants.
---

# Staff Go Backend & Agentic Engineering Constitution

> Language-agnostic clean-code principles (naming, functions, comments, smells,
> SOLID, emergent design) live in [`docs/skills/clean-code.md`](../../docs/skills/clean-code.md);
> this file is the mandatory Go-specific layer on top.

## Core Principles

-   Correctness before cleverness.
-   Keep business rules in the domain, infrastructure at the edges.
-   Prefer composition over inheritance.
-   Small interfaces; concrete constructors.
-   Fail fast on invalid configuration.
-   Measure before optimizing.

## Architecture

-   Use Clean Architecture with explicit boundaries.
-   Separate Domain, Application, Infrastructure and Transport.
-   Dependencies point inward.
-   Domain contains no framework, HTTP, SQL or LLM code.
-   Prefer events over tight orchestration.

## Context

-   `context.Context` is the first parameter.
-   Never store context in structs.
-   Constructors never accept context.
-   Every blocking operation must observe `ctx.Done()`.

## Concurrency

-   Channels for ownership transfer.
-   Mutexes for shared mutable state.
-   Check channel closure (`v, ok := <-ch`).
-   Every goroutine has a termination path.
-   Bound concurrency with worker pools.

## Lifecycle

-   Constructors allocate only.
-   `Start()` performs I/O.
-   `Stop()` drains work with timeout.
-   Components expose Start, Stop, Health and Ready where appropriate.

## Agent Architecture

-   LLM plans; tools execute.
-   Business rules never live in prompts.
-   Keep planner, memory, tools and executor separate.
-   Bound every agent loop by time, iterations and token budget.

## Tool Design

-   Stateless where possible.
-   Idempotent.
-   Deterministic.
-   JSON schema versioned.
-   Validate all inputs and outputs.

## State Machines

-   Explicit states and transitions.
-   No boolean state flags.
-   Invalid transitions return typed errors.
-   Emit domain events on transitions.

## DDD

-   Aggregate roots enforce invariants.
-   Value objects are immutable.
-   Repositories persist aggregates.
-   Domain events model completed business actions.

## Errors & Resilience

-   Use typed errors.
-   Retry only transient failures.
-   Exponential backoff with jitter.
-   Never parse error strings when structured data exists.
-   Respect context cancellation.

## Observability

-   Structured logs.
-   Correlation, request and trace IDs.
-   Metrics for latency, failures, retries, queue depth and inflight
    work.
-   Distributed tracing.
-   Never log secrets or prompts by default.

## Performance

-   Benchmark before optimization.
-   Avoid unnecessary allocations.
-   Reuse buffers and slices.
-   Avoid reflection in hot paths.
-   Keep queues bounded.

## APIs

-   Version breaking contracts.
-   Validate requests.
-   Prefer idempotent commands.
-   Keep DTOs separate from domain models.

## Testing

-   Table-driven tests.
-   Race detector.
-   Contract tests.
-   Chaos and timeout tests.
-   Benchmarks for hot paths.
-   **Test files mirror the source file they test.** `foo.go` → `foo_test.go`; a
    platform-specific source keeps its suffix (`foo_linux.go` → `foo_linux_test.go`,
    `foo_other.go` → `foo_other_test.go`; a cross-platform test for a
    `darwin || linux` source mirrors the source name and repeats its build tag). Do
    NOT create `<feature>_test.go` files with no matching source file — merge the
    tests into the file that mirrors the source. Test-only seams belong in the
    `_test.go` that mirrors the source they support, never in production files.

## Security

-   Least privilege.
-   Validate all external input.
-   Escape output.
-   Rotate secrets.
-   Never trust LLM output.

## Before Coding

1.  Read project constitution and architecture docs.
2.  Understand bounded contexts.
3.  Build and test baseline.
4.  Identify invariants and state transitions.
5.  Design API contracts before implementation.
6.  Add telemetry before adding complexity.

## Go Idioms

-   Return concrete types from constructors.
-   Consumers depend on interfaces.
-   Wrap errors with `%w`.
-   Use `errors.Is` and `errors.As`.
-   No hardcoded values: strings, ints, floats in logic → named `const` at file top. Errors included.
-   Prefer standard library first.
-   **Write modern Go for the go.mod toolchain (not version-pinned advice):** target the Go
    version declared in `backend/go.mod` and use the newest stdlib idioms that version
    supports — `any` over `interface{}`, `slices`/`maps` stdlib packages, builtin `min`/`max`,
    `clear`, `errors.Join`, per-iteration loop variables (Go ≥1.22) — instead of hand-rolled
    helpers. When unsure what the installed toolchain offers, check it directly (`go doc
    builtin`, `go doc slices`, `go doc maps`) rather than trusting remembered release notes;
    never hard-pin a rule to one minor Go release.
-   **Pointer to a value:** need a pointer to the **zero value** → `new(T)` (stdlib; never a
    local `ptr(x)`-style wrapper for zero targets — editors/linters flag it). Need a pointer
    to a **non-zero** literal → one generic helper per package tree
    (`func ptr[T any](v T) *T { return &v }`; precedent: `models.ptr`), called only with
    non-zero values. (`new` has no initializer form in any released Go — do not reach for
    one.)
-   **Domain vocabulary with a fixed value set → typed string enum, never a bare `string`:**
    `type X string` + named constants + a `Valid()` method. Persisted enums live in the
    leaf `models` package (precedent: `WorkloadClass`, `LoopStrategy`); consumer packages
    alias (`type Name = models.X`) rather than duplicating the enum or inverting the leaf
    dependency. Wire structs that cannot reference the enum (import cycle) keep `string`
    and convert at the boundary — the domain/config layer never holds a raw string where a
    typed enum is possible.
-   **Keep function signatures small — 0–3 parameters is the ideal, 4 is the review ceiling,
    and anything past 4 must take a parameter/options struct.** Long positional lists are
    order-sensitive, unreadable at call sites, and a signal that the inputs are one cohesive
    unit (Clean Code: prefer niladic, then monadic, then dyadic; triadic where unavoidable;
    polyadic needs justification). When a call needs four or more values that travel
    together, group them into a named struct (`Options`, `Config`, `Deps`, `WorkspaceView`,
    `Policy`) and pass the struct — never grow a signature one argument at a time. The rule
    counts boolean/mode flags and `nil`-able sentinels too; those are the first candidates
    for an options struct. Precedents in this repo: `shell.WorkspacePolicy`,
    `assistant.AgentOptions`, `sandbox.WorkspaceView`, `egress.HostListPolicy`,
    `assistant.AgentStackDeps`. Enforced in code review; prefer the struct up front because
    no linter reliably catches signature bloat.
-   **One home for shared utilities — never re-derive the same conversion or
    constant in two packages.** If two call sites need the same arithmetic, name it
    once in the package that owns the concept and import it (precedent:
    `platform/units` for KiB/MiB/GiB → bytes, used by storage accounting, sandbox
    rlimits and fetch limits). Inline `1024`/`<<20`/`<<30` arithmetic at call sites
    is a bug farm; the same goes for duplicated small helpers that already exist in
    a shared package.

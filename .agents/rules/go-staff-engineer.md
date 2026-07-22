---
description: Staff-level Go backend and agentic workflow engineering guide optimized for AI coding assistants.
---

# Staff Go Backend & Agentic Engineering Constitution

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
-   Prefer standard library first.

# SPEC Lifecycle & Change Management

## What is a SPEC?

A SPEC is a behavioral contract for a subsystem. Every significant subsystem has one (SPEC-001 through SPEC-009). SPECs define *what* the system does, not *how* — the "how" lives in PLANS and skills.

## SPEC Statuses

| Status | Meaning |
|--------|---------|
| `stable` | Final, changes require a new version |
| `draft` | Being written, not yet accepted |
| `superseded` | Replaced by a newer version |
| `deprecated` | No longer applicable |

## Proposing a New SPEC

1. Determine the next available SPEC ID from `docs/INDEX.md`.
2. Copy `_template.md` (if it exists) or use an existing SPEC as template.
3. Write the SPEC with sections: Intent, Functional Requirements, Data Model, Behavior, Error Handling.
4. Set status to `draft` and open a PR.
5. At least one maintainer must approve before merging.
6. Set status to `stable` after merge.

## Changing a Stable SPEC

SPECs are immutable once `stable`. To change:

1. Increment the `version` field in the frontmatter.
2. Add a changelog entry at the top of the document describing what changed and why.
3. Update `last_updated` to today's date.
4. If the change is breaking, create a new SPEC that supersedes the old one and mark the old one `superseded`.

## Deprecating a SPEC

When a subsystem is removed or replaced:

1. Mark the old SPEC as `deprecated`.
2. If a replacement exists, add a `superseded_by` field pointing to the new SPEC.

## Cross-Referencing

- SPECs reference CONSTITUTION sections via the `constitution_references` field.
- SPECs reference other SPECs via the `related_specs` field.
- INDEX.md tracks all SPECs, their status, and their cross-references.
- When adding a new SPEC or updating an existing one, update INDEX.md.

## Enforcement

SPECs are behavioral contracts — failing a SPEC requirement is a bug. Changes that break a SPEC must be caught in code review or automated tests.

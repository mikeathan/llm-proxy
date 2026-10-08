---
name: pr-commit-message
description: "Draft PR titles, descriptions, and commit messages from the branch implementation, emphasizing core work and material secondary fixes. Use when preparing PR or commit text."
last_reviewed: 2026-10-04
---

# PR and Commit Text

Draft ready-to-copy text for a reviewer without conversation context. Derive the problem, solution, and observable outcome from the implementation and user objective, not commit subjects, file counts, diff size, or the latest fix.

## Scope and evidence

- Inspect status, local refs, and history. Use the requested PR target or an evidence-backed assumption; ask if competing bases materially change scope. The tracking branch is not necessarily the target. Compare HEAD with the verified common ancestor using `git diff`; identify ancestry through `git log` when policy disallows `git merge-base`. Read affected implementation, entry points, tests, and governing docs.
- Inspect staged, unstaged, and relevant untracked changes separately. Default PR scope is committed branch work; default commit scope is staged changes. Harness staging does not establish intent. Label requested current-work or squash scope; explain proposed commit scope when nothing is staged. Never silently combine scopes.
- Follow verified PR templates and commit conventions. Drafting authorizes no git mutations, fetches, or PR publication.

## Select what matters

Lead with the core outcome. Mention supporting plumbing, tests, docs, or refactoring only when needed to explain the solution or tradeoffs.

Include secondary fixes, even unrelated ones, when they materially affect behavior, correctness, data loss, security, compatibility, migration, or review/deployment risk. Give substantial independent fixes a brief separate bullet. Judge impact, not size: a one-line security fix may matter more than a large rename.

Omit incidental formatting, renames, cleanup, test repairs, and intermediate corrections without a separate material outcome **from the prose only**, never from the change scope. Describe final behavior without debugging history. Keep breaking changes and significant risks visible; acknowledge multiple substantial outcomes rather than inventing a single core feature.

## Deliver

Return all three unless a subset is requested; put scope assumptions and uncertainty outside copyable text.

- **PR title:** concrete verb and capability naming the core outcome.
- **PR description:** problem and resulting behavior first; useful trigger/before-and-after example, review-relevant implementation, material secondary fixes, validation, and material limitations or migration needs. Match complexity and the repo template.
- **Commit message:** concise imperative subject following verified conventions; body for rationale, important secondary fixes, or breaking changes. Match selected commit changes, or the agreed full scope for a squash; do not reuse branch-wide text for a smaller commit.

Use plain, concrete language; avoid file inventories, vague improvements, and low-value details. Treat tests as validation, not features. Claim passes only from observed results for the relevant revision; otherwise mark validation unverified. Drafting alone need not rerun the suite.

Cross-check claims against each deliverable's scope: core work leads, important fixes remain visible, and incidental work does not dominate.

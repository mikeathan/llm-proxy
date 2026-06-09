## Task: Memory Tags — Tagged Recall & Type Isolation

**ID:** `memory-tags-test`
**Category:** Testing

Stress-test the agent's memory tagging system by saving facts with different tag sets, then verifying tag-filtered search returns the correct subset. Also validates `daily` and `session` memory type isolation.

### Execution Strategy

#### Phase 1: Save Tagged Facts (Tags Feature)

Save the following facts using `memory_update` with the specified `tags` array. Use topic `"Persona Facts"` for all persona facts and `"Project Facts"` for all project facts. Do not batch — one call per fact.

**Persona facts (tags: `["persona", "aris-thorne"]`):**

1. "Dr. Aris Thorne works at the Xenolith Research Institute."
2. "Dr. Thorne's primary field is exometeorology — the study of weather on exoplanets."
3. "Dr. Thorne's office is in Lab Wing C, room 317."

**Project facts (tags: `["project", "chimera"]`):**

4. "Project Chimera is a collaborative exoplanet weather study."
5. "Project Chimera involves 12 research institutions across 6 countries."

**Mixed fact (tags: `["publication", "aris-thorne"]`):**

6. "Dr. Thorne published a paper in 2024 titled Atmospheric Dynamics of tidally locked super-Earths."

#### Phase 2: Tag-Filtered Search

Use a single `memory_search` with `limit: 20` and no query to retrieve all stored
entries. From that single result, verify the following tag counts. Do not search
more than once.

| Check | Search | Expected count | Notes |
|-------|--------|---------------|-------|
| 1 | `tags: ["persona"]` | **4** (facts 1, 2, 3, 6) | Fact 6 shares topic "Persona Facts" so it's appended into the same entry |
| 2 | `tags: ["project"]` | **2** (facts 4, 5) | |
| 3 | `tags: ["aris-thorne"]` | **4** (facts 1, 2, 3, 6) | |
| 4 | `tags: ["chimera"]` | **2** (facts 4, 5) | |

Then verify the combined search with a separate call:
5. Search `query:"exometeorology" tags:["persona"]` — must return **1** fact (fact 2).

#### Phase 3: Memory Type Isolation

Save a fact with `memory_type: "session"` using a unique topic `"Temp Session Note"`. Then search for it — it must appear in results.

#### Phase 4: Write Report

Produce a structured test report covering:

1. **Tag Search Results** — table of each tag query with expected count vs actual count. Use ✅ for correct, ❌ for mismatch.
2. **Combined Search** — did `query + tags` return the correct fact? ✅ / ❌
3. **Session Memory** — did the session-scoped fact appear? ✅ / ❌
4. **Result: PASS** if all checks pass. Otherwise ❌ **FAIL** with the reason.
5. **Score: X/5** tag checks correct.

### Analysis Goals

- Verify `tags` field is saved and returned correctly in `memory_search`.
- Verify tag-only search (no query) returns the correct subset.
- Verify combined `query + tags` search works correctly.
- Verify `memory_type: "session"` entries are stored and retrievable.

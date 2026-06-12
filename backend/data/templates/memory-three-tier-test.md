## Task: Memory Three-Tier Test

**ID:** `memory-three-tier-test`
**Category:** memory

Validate the three-tier memory architecture: scope, mode, and keep parameters.

### Phase 1: Save Facts

Save the following facts using `memory_update` with the specified parameters:

1. `scope: "user", mode: "always", keep: "permanent"`
   Content: "My birthday is January 1st. I prefer concise answers."

2. `scope: "workspace", mode: "always", keep: "permanent"`
   Content: "This project uses TypeScript 6.0.3. Run npx tsc to compile."

3. `scope: "workspace", mode: "on_demand", keep: "permanent"`
   Content: "Smoke test completed successfully on 2026-06-09."

4. `scope: "workspace", mode: "on_demand", keep: "session"`
   Content: "Currently testing memory tier system. Debug mode enabled."

### Phase 2: Verify by Search

Run these three searches, one at a time. After each search, note the actual result count, then move to the next search.

1. Search with `scope: "user"` — expect exactly 1 result (fact 1)
2. Search with `scope: "workspace"` — expect exactly 3 results (facts 2, 3, 4)
3. Search without scope — expect exactly 4 results (all facts)

### Phase 3: Write Report

Build the following table using the actual search results. Replace `PASS` or `FAIL` for each row.

| Check | Expected | Actual | Status |
|-------|----------|--------|--------|
| User scope search | 1 | <actual count> | PASS / FAIL |
| Workspace scope search | 3 | <actual count> | PASS / FAIL |
| Unscoped search | 4 | <actual count> | PASS / FAIL |
| **Overall** | | | **PASS / FAIL** |

If any row has `FAIL`, the **Overall** is `FAIL`. If all rows are `PASS`, the **Overall** is `PASS`.

Call `submit_final_answer` with the table above. Include any notes about unexpected results (e.g. leftover entries from previous runs). Do not call any other tools after the report.

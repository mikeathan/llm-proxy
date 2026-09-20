## Task: LLM & AI Release News Brief

**ID:** `llm-ai-release-brief`
**Category:** research

Produce a dated, link-backed brief of the latest LLM and AI releases and industry news. Optimised for low token use: bounded searches, one compact table, no narrative padding.

### Budget (hard caps)

- **Max 4 searches.** Each query targets one lane; do not re-run a query with reworded terms.
- **Max 2 `fetch_url` calls**, and only when a search snippet already looks like a major release or an official announcement.
- No note may exceed **2 lines**. Prefer the table over prose.

### Search Lanes

Run these in order, stopping early once the table reaches 8 rows:

1. `new LLM model release this week`
2. `AI industry news latest`
3. `open weights model release`
4. `AI funding acquisition announcement`

Keep each query short — long queries return noisier results and cost more tokens.

### Output Format

Start with the date line, then the table. Nothing before them.

    As of: YYYY-MM-DD | Searches: N | Window: latest available

    | Item | Date | Type | Why it matters (<=15 words) | Source |
    |---|---|---|---|---|
    |  |  |  |  |  |

Rules for the table:

- **6–10 rows.** Hard cap at 10; stop searching once you reach it.
- **Item** is the model or product name only. **Type** is one of: model, tool, funding, policy, research.
- **Date** is `YYYY-MM-DD`, or `n/d` when the source states none — never guess.
- **Source** is a real link returned by a search. Every row needs one.
- Order newest first. Skip anything older than 60 days.

### Optional Notes

Add only if it earns its tokens:

- **Read:** one single most important item, one sentence on what changes.
- **Gap:** one line, only if search was unavailable or results were thin.

### Result

**PASS** if the report contains the date line and a table of 6–10 rows, each with a real source link, all dated within 60 days, and at least one row dated within the last 14 days (or an explicit statement that nothing that recent was found). If `internet_search` is unavailable, **PASS** only with a single-line **Gap** note stating the failure — do not substitute recalled knowledge for search results. Otherwise **FAIL** and state the reason.

## Task: LLM & AI Release News Brief

**ID:** `llm-ai-release-brief`
**Category:** research

Produce a dated, link-backed brief of what is **new** in LLM releases and AI news within the window below. Optimised for low token use: bounded searches, one compact table, no narrative padding.

### Settings (edit this line to match how often the automation runs)

**Window:** `day`

`day` for a nightly run, `week` for a weekly run, `month` for a monthly run. Everything below follows this one value.

| Window | Pass as `time_range` | Keep items dated within |
|---|---|---|
| `day` | `day` | the last 2 days (covers time zones and late-indexed pages) |
| `week` | `week` | the last 8 days |
| `month` | `month` | the last 32 days |

### Rules that protect this task

- **Your final reply is the deliverable.** Do not create, edit, overwrite or delete any file — above all never this task file. Overwriting it replaces these instructions with last run's output, and later runs would re-verify old results instead of searching.
- Every item must come from this run's searches. Memory (below) only tells you what earlier briefs already reported and which sources worked; it is never a source of items.

### Memory (only if the `memory_search` and `memory_update` tools are available)

This is how the brief learns from earlier runs. Memory calls do not count toward the search or fetch budget.

1. **Before your first search**, call `memory_search` with the query `LLM news brief` (limit 5). It returns notes saved by earlier briefs: items already reported and which sources worked or blocked. Do not report an item again unless there is a genuinely new development; prefer sources that worked and do not fetch ones noted as `blocked`.
   - **Compare before you write the table.** For every candidate row, check its Item name against the `reported:` list of **every** returned note, whatever its date (a note from today counts too). A name that appears there is a repeat: drop it. Treat different names for the same product as one item (for example a model's official name and its nickname).
   - A repeat stays only if a source in this run states a concrete new development (new availability, price, release date or version). Then name the development in the "Why it matters" cell.
2. **Decide your table rows first, then — BEFORE you send the answer, never after it —** call `memory_update` once to save this run as one workspace, on-demand, permanent fact, in this form (under 300 characters):
   `LLM news brief YYYY-MM-DD reported: <the Item column of your table, in order, separated by commas>. Sources: ok <domains whose search results you used>; blocked <domains whose fetch_url failed, with the status code>`
   The item names must be exactly the rows of your table: do not list anything that is not a row (items you dropped as old or out of window are not rows), and do not leave a row out. Mark a domain `blocked` only when `fetch_url` actually failed for it; a domain that only appeared in search results is `ok`. Skip the save when the table has no rows. Do not save anything else, and never save secrets.
3. If `memory_update` says "already saved", do not call it again.

### Budget (hard caps)

- **Max 4 searches.** Each query targets one lane; do not re-run a query with reworded terms.
- **Max 2 `fetch_url` calls**, and only when a search snippet already looks like a major release or an official announcement.
- No note may exceed **2 lines**. Prefer the table over prose.

### Search Lanes

Every search **must** pass `time_range` set to the Window value above. Run the lanes in order, stopping early once the table reaches 10 rows:

1. `new LLM model release`
2. `AI news`
3. `open weights model release`
4. `AI funding acquisition announcement`

Keep each query short and do not put dates in it — the `time_range` filter does the date work. If a search returns nothing within the window, that lane is empty; do not retry it without `time_range`.

### Output Format

Start with the date line, then the table. Nothing before them: no draft list, no "let me compile", no reasoning. Think in your reasoning, not in the reply.

    As of: YYYY-MM-DD | Window: <day|week|month> | Searches: N

    | Item | Date | Type | Why it matters (<=15 words) | Source |
    |---|---|---|---|---|
    |  |  |  |  |  |

Rules for the table:

- **Up to 10 rows**, only items that are genuinely new in the window. A quiet period gives a short table; never pad it with older items.
- **Item** is the model or product name only. **Type** is one of: model, tool, funding, policy, research.
- **Date** is `YYYY-MM-DD`, or `n/d` when the source states none — never guess. Use only a date that belongs to that item's own page or headline; a date printed on a sidebar card, "related" link or another article is not the item's date. A row with `n/d` stays only if the result came back inside the `time_range` filter.
- **Source** is the specific article or announcement URL returned by a search. Not an index, timeline, "latest news" or blog-home page: those change daily and hide what is actually new.
- Order newest first. Drop anything dated outside the "keep items dated within" column.

### Optional Notes

Add only if it earns its tokens:

- **Read:** the single most important item, one sentence on what changes.
- **Gap:** one line, only if search was unavailable, a lane was empty, or results were thin.

### Result

**PASS** if the report has the date line and a table where every row has a specific source link and falls inside the window, **or** it has a single-line **Gap** note stating that nothing new was found in the window (an honest empty brief is a PASS). If `internet_search` is unavailable, **PASS** only with a single-line **Gap** note stating the failure — do not substitute recalled knowledge for search results. Any file written or overwritten during the run is a **FAIL**. Otherwise **FAIL** and state the reason.

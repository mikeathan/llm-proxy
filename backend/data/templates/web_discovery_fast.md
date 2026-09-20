## Task: Fast Web Discovery — Latest AI & LLM News

**ID:** `web-discovery-fast`
**Category:** research

Use `internet_search` to build a dated, link-backed digest of the latest AI and LLM news. Token-efficient by design: few searches, full extraction of every result already received, one compact table.

### Budget (hard caps)

- **Max 2 `internet_search` calls.** Never re-run a query with reworded terms — rewording burns API tokens and trips the loop detector.
- **Max 1 `fetch_url`**, and only for the single most important item if its snippet lacks both a date and the key fact. Page fetches are the most expensive call in context terms — skip by default.
- Read **every** result of each search before moving on. The snippets are already in context; re-searching costs API tokens, reading does not.

### Search Plan

Run both lanes, in order. After each search, extract **every usable item** from its results before the next search.

1. `latest AI news this week`
2. `new LLM model release` — the one release lane for all labs, Western and Asian

Keep each query short. If a search errors or returns nothing usable, note it and move to the next lane — never retry a lane.

### Output Format

Start with the date line, then the table. Nothing before them.

    As of: YYYY-MM-DD | Searches: N

    | Item | Date | Type | Key fact (<=18 words) | Source |
    |---|---|---|---|---|

Rules for the table:

- **6–10 rows.** Hard cap at 10; prefer dropping weak or duplicate candidates over padding.
- **Item** is the model, product, or company name only. **Type** is one of: model, tool, policy, research.
- **Coverage:** lane 2 is the only release lane — include model releases from both Western labs (OpenAI, Anthropic, Google, Meta, Mistral, ...) and Asian labs (DeepSeek, Qwen, Kimi, GLM, ERNIE, ...) whenever the results contain them; do not let one region crowd out the other.
- **Key fact** must carry the concrete detail from the snippet — version, parameter count, price, benchmark — whenever the snippet contains one. A vague paraphrase is not acceptable when a number is available.
- **Date** comes only from the snippet or title, as `YYYY-MM-DD`, or `n/d` when the source states none. Never guess or recall a date.
- **Source** is a real URL returned by a search; every row needs one. The same story across lanes is one row — keep the strongest source.
- Order newest first. Drop items the source dates older than 30 days; keep `n/d` items only when the phrasing is clearly recent ("this week", "today").

### Optional Notes

Add only if it earns its tokens:

- **Read:** one sentence on the single most important item, only if `fetch_url` was used and added detail beyond the snippet.
- **Gap:** one line, only if search was unavailable, errored, or returned too few usable items.

### Result

**PASS** if the report contains the date line and a table of 6–10 rows (or a **Gap** note explaining why fewer), each with a real source URL and no guessed dates, at least 2 rows dated within the last 14 days (or a **Gap** note stating nothing that recent was found), and no more than 2 searches were used. If `internet_search` is unavailable, **PASS** only with a single-line **Gap** note stating the failure — do not substitute recalled knowledge for search results. Otherwise **FAIL** and state the reason.

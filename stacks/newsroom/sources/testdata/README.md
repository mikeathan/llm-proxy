# Recorded responses

Each file is a real response recorded 2026-11-20 from the dev Mac (not vertex), trimmed to a few entries; Telegram markup had images and inline SVG removed.
They pin how each adapter reads the source today; a source that changes shape fails a test here, not silently in production.

| File | Request |
|---|---|
| hn.json | `https://hn.algolia.com/api/v1/search_by_date?query=llm&tags=story&hitsPerPage=3` |
| mastodon.json | `https://mastodon.social/api/v1/timelines/tag/llm?limit=3` |
| github-releases.atom | `https://github.com/ggml-org/llama.cpp/releases.atom` (first 3 entries) |
| bbc-tech.rss | `https://feeds.bbci.co.uk/news/technology/rss.xml` (first 3 items) |
| telegram-channel.html | `https://t.me/s/telegram` (3 messages with text) |
| page-a.html | hand-written; the plain-page adapter hashes visible text |

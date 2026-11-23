---
id: ADR-0079
title: The Brief topic form asks for one thing and fills the rest from starters
status: superseded
date: 2026-11-23
subsystems: [autonomy]
supersedes:
superseded_by: ADR-0080
---

# ADR-0079: The Brief topic form asks for one thing and fills the rest from starters

## Context

The topic form had six fields (name, section, searches, website, region, words to skip) and a source browser, all shown at once. After ADR-0076 and ADR-0078 added the website list and the Section field, the owner found it "too many sections to fill in" (2026-11-23). Four layouts were drawn and the owner took the recommended pair: a starter gallery for an empty page and a one-box form for adding a topic.

## Decision

- A new topic asks for **what to follow**. The name is derived (the first search that is not a `site:` search, else the first site's name or host) unless typed; the section is empty (its own section) unless a suggestion sets it or the owner types one. Everything else sits under **Fine-tune**, which is open when a topic is edited, so no field is ever hidden from someone changing a topic.
- **Starters** are a constant list in the front end (`BRIEF_STARTERS`): a name, a section, searches and well-known sites by their name in the built-in list. They appear as up to eight suggestion chips in the form and as a gallery on an empty page. A starter copies its sites into the topic exactly as a pick from the list does (ADR-0076), so a topic never depends on the list, and a site the list no longer holds is left out.
- The back end, the stored topic and the API are unchanged.

## Alternatives rejected

- **A sentence with blanks** and **sites first** (the other two drawn layouts): the first hides what each blank does; the second makes keywords an afterthought, though a topic is mostly searches.
- **Starters served by the back end**: one more route and a version of the list to keep in step for no gain; they only name sites the catalog already serves.

## Consequences

A starter naming a site the catalog lost silently has fewer sites; one with no search and no site found is not offered. The starters are English and news-centred, like the catalog.

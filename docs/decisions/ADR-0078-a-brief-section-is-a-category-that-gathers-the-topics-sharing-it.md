---
id: ADR-0078
title: A Brief section is a category that gathers the topics sharing it
status: proposed
date: 2026-11-23
subsystems: [autonomy]
supersedes:
superseded_by:
---

# ADR-0078: A Brief section is a category that gathers the topics sharing it

## Context

A topic is a set of searches and feeds; until now each topic was one section, and its category only chose the section's artwork. An owner who wants "Technology", "News" and "Finance" had to make one topic per heading and put every search in it, or accept a section per search. An edition also showed top stories from one topic's searches under a card that counted only what the top stories had left, which read as stories from nowhere (2026-11-23).

## Decision

The section is the **category**. `Compose` groups an edition's topics by `CategoryKey` (lower case, single spaces) of `Topic.Group()` (the category, else the name), writes one section per group in first-seen order and pools the stories of all its topics. The edition still lists topics and nothing stored changes; `Normalize` defaults a new topic's category to its name. The page counts a card by what the model wrote and names each top story's section.

## Alternatives rejected

- **Editions list categories, topics belong to one** — a second stored shape and a migration of every edition for no reader-visible gain; rejected.
- **Fixed list only** — an owner's own heading ("UK politics") would be impossible; the form offers the built-in groups and accepts any text.
- **Merge sections in the page** — the model would still write each topic's section separately, spending calls and ranking inside the wrong pool.

## Consequences

One busy topic can fill the 15-candidate cap of its whole category. A category with topics read at different times is as stale as its oldest read. The section's id on the page is its first topic's id.

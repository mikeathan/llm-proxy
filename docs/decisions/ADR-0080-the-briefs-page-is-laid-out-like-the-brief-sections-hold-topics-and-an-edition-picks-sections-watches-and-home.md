---
id: ADR-0080
title: The Briefs page is laid out like the brief: sections hold topics, and an edition picks sections, watches and home
status: proposed
date: 2026-11-23
subsystems: [autonomy]
supersedes: ADR-0079
superseded_by:
---

# ADR-0080: The Briefs page is laid out like the brief: sections hold topics, and an edition picks sections, watches and home

## Context

ADR-0079 made the topic form ask for one thing, but the owner still could not tell what to enter: a topic form with "section" and "searches" fields did not show how a section, a topic and a search relate. Typing "greece", "uk" and "world" as three searches made one topic named Greece in the section News. What the owner meant was a section News holding the topics UK, Greece, World, War and Politics. Editions listed single topics, and could not be only the owner's home, watches or network and radio.

## Decision

- **The Topics panel has the shape of the brief.** Each section is a heading with its topic cards; a field at the end of the section adds a topic to it. The word typed is the topic's name and its search, and the section is filled in. A topic with no section is listed last under "No section". A new section is only a name until it has a topic, because the server keeps a section as the name its topics share (ADR-0078). A card says what the topic reads in two plain lines (Searches, Reads); Edit opens the full form (name, section, searches, websites, region, words to skip), with nothing folded away.
- **The starter gallery stays** on an empty page. The starter chips inside the topic form, "Fine-tune" and "Save and add another" are removed (the form only edits).
- **An edition (a brief) is picked by section.** The form asks "What goes in it?": News (the sections to keep, as tags, with a switch), What my watches found, My home: network and radio. The server still stores topic ids: the picked sections are turned into their topics on save, in section order, and a topic added to a section joins every edition that already reads that section (front end, `editionJoining`). The order is fixed: news sections, then watches, then home.
- **An edition may have no news section** when it carries watches or home (`cleanSections`: an empty list is accepted only then); with none of the three it is still refused. It is the one backend change.

## Consequences

- An edition still reads at most 12 topics (`MaxSections`), so a section with many topics can fill a brief; the form says so and does not save.
- A topic edited into another section is not moved between editions: the edition keeps the topic ids it had.
- Reading the groups by section was already how the edition is written (ADR-0078, SPEC 1.17); only the setup screen changed.

## Task: Memory Cascade — Persona Recall & Cross-Reference

**ID:** `memory-cascade-test`
**Category:** memory

Stress-test the agent's memory subsystem by feeding distributed facts about a fictional persona across multiple turns, then verifying recall, synthesis, and temporal ordering.

### Execution Strategy

#### Phase 1: Save Facts

Save the following facts to memory using `memory_update` with `scope: "workspace", mode: "on_demand", keep: "permanent"`, one at a time (do not batch them into a single call):

1. "Dr. Aris Thorne works at the Xenolith Research Institute."
2. "Dr. Thorne's primary field is exometeorology — the study of weather on exoplanets."
3. "Dr. Thorne's office is in Lab Wing C, room 317."
4. "Dr. Thorne has a pet star-nosed mole named 'Pulsar'."
5. "Dr. Thorne published a paper in 2024 titled *Atmospheric Dynamics of tidally locked super-Earths*."
6. "The Xenolith Institute is located in Nuuk, Greenland."
7. "Dr. Thorne's collaborative project codename is 'Project Chimera'."

There are exactly **7 facts**. Verify all 7 are saved before proceeding to Phase 2. If any fact is missing from memory after saving, the run FAILS — re-attempt the missing fact before moving on.

#### Phase 2: Answer Questions

Call the `memory_search` tool **at most once** to retrieve all stored facts. Then answer each of the following questions from the search result:

- "What is Aris Thorne's field of study?"
- "Where is the Xenolith Institute located, and what is Dr. Thorne's room number there?"
- "Name the collaborative project Dr. Thorne is involved with."
- "What is the name and species of Dr. Thorne's pet?"
- "In what year was Dr. Thorne's paper on tidally locked super-Earths published, and what was its title?"

#### Phase 3: Write Biography

Produce a single, coherent 3-sentence biography of Dr. Aris Thorne. The biography must:

- Reference at least 5 distinct facts from Phase 1.
- Correctly order events mentioned (e.g., the paper predates current work).
- Include the pet's name and the collaborative project codename.

### Analysis Goals

- Verify the agent retains facts across unrelated intervening turns.
- Detect gaps or hallucinations in cross-turn recall.
- Validate temporal ordering of events in synthesis.
- Measure whether fact recall degrades as the conversation lengthens.

### Output Format

1. Raw recall results table (Fact → ❌ Correct / ✅ Incorrect / ❌ Missing) — use the ❌ icon for any fact that was lost or incorrect.
2. Generated biography (verbatim)
3. Error summary: which facts were dropped, conflated, or hallucinated (each with ❌ next to it)
4. Result: **PASS** if all 7 facts retained AND all 5 questions answered correctly. Otherwise ❌ **FAIL** with the reason.
5. Score: X/7 facts retained, Y/5 questions answered correctly, synthesis passes temporal check (Yes/No)

### Cross-Run Contamination

This test assumes a **clean memory**. If a previous run of this test left the Dr. Aris Thorne facts in memory, the recall
results will be invalid. Before running, delete any pre-existing "Aris Thorne" / "Xenolith" facts (via the admin UI or a
memory reset), or note their presence in the report and exclude them from the 7-fact count. Score only the facts saved in
**this** run.

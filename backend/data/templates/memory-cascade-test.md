## Task: Memory Cascade — Persona Recall & Cross-Reference

**ID:** `memory-cascade-test`
**Category:** Testing

Stress-test the agent's memory subsystem by feeding distributed facts about a fictional persona across multiple turns, then verifying recall, synthesis, and temporal ordering.

### Execution Strategy

#### Phase 1: Seeded Facts

Present the agent with the following facts, ONE per turn (do not batch them):

1. "Dr. Aris Thorne works at the Xenolith Research Institute."
2. "Dr. Thorne's primary field is exometeorology — the study of weather on exoplanets."
3. "Dr. Thorne's office is in Lab Wing C, room 317."
4. "Dr. Thorne has a pet star-nosed mole named 'Pulsar'."
5. "Dr. Thorne published a paper in 2024 titled *Atmospheric Dynamics of tidally locked super-Earths*."
6. "The Xenolith Institute is located in Nuuk, Greenland."
7. "Dr. Thorne's collaborative project codename is 'Project Chimera'."

Wait at least one unrelated exchange before proceeding to Phase 2.

#### Phase 2: Recall Interrogation

Ask the agent each of the following. A correct answer must draw from facts in Phase 1. If the agent cannot answer after one attempt, note the failure and continue.

- "What is Aris Thorne's field of study?"
- "Where is the Xenolith Institute located, and what is Dr. Thorne's room number there?"
- "Name the collaborative project Dr. Thorne is involved with."
- "What is the name and species of Dr. Thorne's pet?"
- "In what year was Dr. Thorne's paper on tidally locked super-Earths published, and what was its title?"

#### Phase 3: Synthesis & Temporal Reasoning

Ask the agent to produce a single, coherent 3-sentence biography of Dr. Aris Thorne. The biography must:

- Reference at least 5 distinct facts from Phase 1.
- Correctly order events mentioned (e.g., the paper predates current work).
- Include the pet's name and the collaborative project codename.

### Analysis Goals

- Verify the agent retains facts across unrelated intervening turns.
- Detect gaps or hallucinations in cross-turn recall.
- Validate temporal ordering of events in synthesis.
- Measure whether fact recall degrades as the conversation lengthens.

### Output Format

1. Raw recall results table (Fact → Correct / Incorrect / Missing)
2. Generated biography (verbatim)
3. Error summary: which facts were dropped, conflated, or hallucinated
4. Score: X/7 facts retained, Y/5 questions answered correctly, synthesis passes temporal check (Yes/No)

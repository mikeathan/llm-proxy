## Task: Memory Store Test

**ID:** `memory-store-test`
**Category:** memory

Step 1 of 2. Saves one made-up fact so a later run can prove memory works. Run it once, then run `memory-recall-test` in a NEW chat (or as a separate automation with Memory set to On). Delete the fact in the Memory panel afterwards. See `docs/guides/memory-testing.md` (Part A).

TASK: Save exactly one fact to memory, then stop.

1. Call the memory_update tool once with these exact arguments:
   - scope: "workspace"
   - mode: "always"
   - keep: "permanent"
   - content: "The incident commander is Marcus Lindqvist and the audit window opens on 2032-07-09."
2. If the tool says the fact is saved (or already saved), reply with the single word STORED.
   If it returns an error, reply with the error text.
Do not call any other tool and do not write any other text.

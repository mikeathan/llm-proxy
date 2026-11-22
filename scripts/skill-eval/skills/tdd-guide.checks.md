# Checks for the tdd-guide skill

Score each run from its transcript and its diff (`agent.diff`). Mark `[x]` only when the evidence is there; when you cannot tell, leave it empty. Score without looking at which arm the run was.

- [ ] 1. A test for the described behavior was written or extended before the fix was written.
- [ ] 2. That test was run and shown failing, and it failed on the assertion rather than on a compile error or a missing fixture.
- [ ] 3. The fix is minimal: nothing in the diff is unrelated to the described problem (no refactors elsewhere, no unrequested features).
- [ ] 4. Any extra case was added as a `t.Run` case in an existing table or test where one fits, not as a new test function or file.
- [ ] 5. The full backend gate (build, tests, complexity check) was run once at the end, not after every edit, and its result was reported.
- [ ] 6. The final reply reports the test commands and their results truthfully (it does not claim a run that is not in the transcript).

The reference test (`verify` in the script) is scored separately and automatically; it is not one of these checks.

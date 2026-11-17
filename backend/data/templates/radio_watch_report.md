## Task: Radio Watch Report

**ID:** `radio-watch-report`
**Category:** network
**Summary:** Reads what the Wi-Fi listening sensor found around the house and tells you in a few lines whether anything needs your attention. It listens to nothing itself.
**Agent:** radio-reporter
**Journal:** off
**Schedule:** 0 7 * * *

Reads the Radio sensor's digest and writes a short report: whether the sensor has listened long enough to say anything, whether a watch is running now, what it found in fixed words, and what is worth looking at first. It cannot start listening: that needs your passphrase on the Radio page. The digest is counts and fixed wording only; it carries no addresses or names, so the report has none to give. It runs every morning at 07:00 to start with; set it to manual to run it only when you press Run.

### Before the first run

- Switch the Radio stack on in Modules and start a watch on the Radio page (Listening tab). The sensor only knows what it has heard: with no watch there is nothing to report, and the report says so.

### Execution Strategy

**Action:** Run `radio_digest` once, with no arguments. It is the only tool this automation needs.

Then write the report as the radio-reporter profile describes: how much the sensor has listened and whether a watch is running now, the findings in the digest's own words with their tier, count and what each usually means, a line with how many devices of other people were counted by kind and that the sorting is provisional, and at most two things to look at first, or "nothing". Follow the profile where this text and it differ. The digest has no addresses, names or earlier reports, so the report says nothing about them.

This automation runs only on a local model and saves nothing to memory.

### Result

The report is the result. If `radio_digest` was not available or returned an error (for example the Radio stack is off), say so in the report and stop. Do not print a status or PASS/FAIL line.

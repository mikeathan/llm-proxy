## Task: TypeScript Data Processor

**ID:** `ts-logic-interface-test`
**Category:** development

Validate the agent's ability to write typed TypeScript with interfaces, array operations, string formatting, and console output.

### Execution Strategy

#### Phase 1: Write Application

Create a directory `ts-dashboard` with `app.ts`.

**Interface to define:**

- `DayTemp(day: string, high: number, low: number)` — represents a single day's temperature range

**Function to implement:**

- `formatBar(value, max, width)` — returns a progress bar string using `█` and `░` characters (e.g. `█████░░░░░` for half)

**Main logic:**

1. Create an array of 5 `DayTemp` entries (Mon-Fri with realistic high/low values)
2. For each day, log: the day, a progress bar for the high temperature, and the high temperature value
3. Use `Array.forEach` to iterate

Example output:
```
Mon: ████████░░ 25°C
Tue: █████████░ 28°C
Wed: ██████░░░░ 22°C
```

#### Phase 2: Compile & Run

- Compile with `--strict` and `--target ES2020` (or use `npx ts-node`)
- Run the output

#### Phase 3: Report

1. Source Code Block (filename and key functions used)
2. Compilation Status
3. Generated Output

#### Phase 4: Cleanup (best-effort)

Remove `ts-dashboard/` after the report is written (ignore errors).

### Analysis Goals

- Verify TypeScript interface usage and strict mode compilation
- Confirm array operations and string formatting
- Confirm console output is captured in report

### Result

**PASS** if ALL of the following hold; otherwise **FAIL** and state the failing phase:

1. `ts-dashboard/app.ts` defines the `DayTemp` interface and `formatBar` function and compiles with `--strict` (Phase 2).
2. The run succeeded and produced output (Phase 2).
3. The captured output contains all 5 weekdays (Mon–Fri) with a `█`/`░` progress bar and a temperature per line, matching the example format (Phase 3).
4. The report includes the source code block, compilation status, and generated output (Phase 3).

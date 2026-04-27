## Task: TypeScript Logic & Interface Test

**ID:** `ts-logic-interface-test`
**Category:** Development

Verify the agent's ability to handle TypeScript interfaces, array manipulation, and the standard compilation-to-execution pipeline.

### Execution Strategy

#### Action: Logic & Type Validation

**Action:** Create a directory named `ts-logic-test` and populate it with a functional TypeScript application.

- **Source File**: `app.ts` defining a `Product` interface (name: string, price: number, category: string).
- **Data Logic**: Initialize an array of 5 products and implement a filter function for items < $50.
- **Runtime**: Compile and execute the code using a robust command (e.g., `npx ts-node app.ts` or `npx tsc && node app.js`) and log the filtered results.

### Analysis Goals

- Confirm successful transpilation of TypeScript interfaces.
- Verify logic execution within the restricted sandbox environment.
- Validate the agent's ability to capture and present console output accurately.

### Output Format

1. Source Code Block (`app.ts`)
2. Compilation Status
3. Filtered Product List (Console Output)

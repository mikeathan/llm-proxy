# PLAN-001: Discovery Panel Implementation

## Phase: Logic & Execution
## Related Spec: [SPEC-001](../SPECS/discovery-panel.md)

---

### 1. Backend: Metadata Service Integration

#### 1.1 GGUFScanner Refinement
- Ensure `internal/core/llm/metadata/gguf_scanner.go` is fully integrated into the `DiscoveryService`.
- **Action**: Update the `DiscoveryService` to call `GGUFScanner.Scan()` for every `.gguf` file found.

#### 1.2 Deprecate Filename Parsing
- Locate and remove all regex-based filename parsing logic in `internal/platform/discovery/`.
- **Action**: Replace `parseMetadataFromFilename(name string)` with a call to the scanner.

#### 1.3 Metadata Normalization
- Update the `ModelMetadata` struct to include the new fields: `Author`, `Description`, and `Architecture`.
- **Action**: Ensure the JSON response sent to the frontend includes these high-fidelity fields.

### 2. Frontend: Discovery UI Update

#### 2.1 Model List Component
- Update the Vue components to display the binary-parsed metadata.
- **Action**: Add "Architecture" and "Quantization" badges to the model cards in `DiscoveryPanel.vue`.

#### 2.2 Grouping Logic
- Refactor the frontend grouping logic.
- **Action**: Use the `metadata.Name` (from binary) as the primary key for grouping versions, rather than fuzzy filename matching.

### 3. Migration & Security

#### 3.1 Context Propagation
- Ensure the `GGUFScanner` receives the request context.
- **Action**: Verify that cancelling a discovery scan in the UI immediately stops the binary file handles in the backend.

#### 3.2 Error Handling
- Implement fallback logic if a GGUF file is corrupt.
- **Action**: If binary parsing fails, flag the model as "Corrupt/Unknown" in the UI rather than crashing the scan.

### 4. Verification Steps

1.  **Test Case: Renamed File**
    - Rename `Llama-3.gguf` to `anything.bin`.
    - Verify the Discovery Panel still identifies it correctly as "Llama 3".
2.  **Test Case: Large Model Performance**
    - Trigger a scan on a directory containing a 50GB+ GGUF file.
    - Verify the scan takes <500ms and doesn't spike RAM.
3.  **Test Case: Security Boundary**
    - Attempt to point the scanner at a non-GGUF file.
    - Verify it returns a clean error without attempting to read the entire file.

---

### 5. Compliance Check (CONSTITUTION.md)
- [x] Network interactions use `NetworkTools`? (N/A for local scanning)
- [x] Binary parsing uses specialized libraries? (Yes, `gguf-parser-go`)
- [x] All I/O accepts `context.Context`? (Yes)
- [x] Errors use `%w` wrapping? (Yes)

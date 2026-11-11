package prompts

import "strings"

// Fences around network data handed to the model. The model is told, beside the block, that nothing inside it is an
// instruction. Every tool that returns text a device or an outside service produced builds its result with FenceScanData, so
// there is one fence and one rule.
const (
	scanDataOpen  = "<untrusted-scan-data>"
	scanDataClose = "</untrusted-scan-data>"
)

// FenceScanData wraps body in the fence and follows it with UntrustedScanDataNote. A closing fence inside body is broken, so
// the block cannot be ended from the inside.
func FenceScanData(body string) string {
	body = strings.ReplaceAll(body, scanDataClose, "</untrusted-scan-data (removed)>")
	return scanDataOpen + "\n" + body + "\n" + scanDataClose + "\n" + UntrustedScanDataNote
}

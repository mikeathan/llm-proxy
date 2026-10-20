package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// FlexInt is an integer tool argument that accepts what models actually send: 100, "100", " 100 " or 100.0. Models
// differ here (a local model in XML mode sends every value as text), and a plain int field would answer a harmless
// "100" with a parse error. JSON null leaves a pointer to it nil.
type FlexInt int

// UnmarshalJSON accepts a JSON number or a string holding one, and rejects anything else with a message the model can act on.
func (n *FlexInt) UnmarshalJSON(data []byte) error {
	raw := bytes.TrimSpace(data)
	if len(raw) > 0 && raw[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return err
		}
		raw = []byte(strings.TrimSpace(text))
	}
	if i, err := strconv.Atoi(string(raw)); err == nil {
		*n = FlexInt(i)
		return nil
	}
	if f, err := strconv.ParseFloat(string(raw), 64); err == nil && f == math.Trunc(f) && math.Abs(f) < 1<<53 {
		*n = FlexInt(int(f))
		return nil
	}
	return fmt.Errorf("expected a whole number, got %s", bytes.TrimSpace(data))
}

// IntPtr returns the value as a *int, or nil when the argument was omitted.
func (n *FlexInt) IntPtr() *int {
	if n == nil {
		return nil
	}
	v := int(*n)
	return &v
}

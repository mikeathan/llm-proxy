package utils

import (
	"encoding/json"
	"fmt"
)

func ToJson(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal tool result: %v", err))
	}
	return string(b)
}

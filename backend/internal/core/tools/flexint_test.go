package tools

import (
	"encoding/json"
	"testing"
)

func TestFlexInt_AcceptsWhatModelsSend(t *testing.T) {
	cases := map[string]int{`100`: 100, `"100"`: 100, `" 100 "`: 100, `100.0`: 100, `"100.0"`: 100, `0`: 0, `"-3"`: -3}
	for in, want := range cases {
		var args struct {
			Offset *FlexInt `json:"offset"`
		}
		if err := json.Unmarshal([]byte(`{"offset":`+in+`}`), &args); err != nil {
			t.Errorf("%s: %v", in, err)
			continue
		}
		if got := args.Offset.IntPtr(); got == nil || *got != want {
			t.Errorf("%s: got %v, want %d", in, got, want)
		}
	}
}

func TestFlexInt_OmittedAndNullStayNil(t *testing.T) {
	for _, in := range []string{`{}`, `{"offset":null}`} {
		var args struct {
			Offset *FlexInt `json:"offset"`
		}
		if err := json.Unmarshal([]byte(in), &args); err != nil || args.Offset.IntPtr() != nil {
			t.Errorf("%s: %v %v", in, err, args.Offset)
		}
	}
}

func TestFlexInt_RejectsTextWithAClearMessage(t *testing.T) {
	for _, in := range []string{`"abc"`, `1.5`, `true`, `{}`, `""`} {
		var args struct {
			Offset *FlexInt `json:"offset"`
		}
		err := json.Unmarshal([]byte(`{"offset":`+in+`}`), &args)
		if err == nil {
			t.Errorf("%s should fail", in)
		}
	}
}

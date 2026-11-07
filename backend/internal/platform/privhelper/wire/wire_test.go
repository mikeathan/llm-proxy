package wire

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"
)

func frameOf(body string) []byte {
	h := make([]byte, 4)
	binary.BigEndian.PutUint32(h, uint32(len(body)))
	return append(h, body...)
}

func TestARequestSurvivesARoundTrip(t *testing.T) {
	var buf bytes.Buffer
	want := Request{V: Version, ID: "abc", Action: "scan_udp", Params: []byte(`{"target":"192.168.1.5"}`)}
	if err := WriteFrame(&buf, want, MaxRequestBytes); err != nil {
		t.Fatal(err)
	}
	got, err := ReadRequest(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID || got.Action != want.Action || string(got.Params) != string(want.Params) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestAResponseSurvivesARoundTrip(t *testing.T) {
	var buf bytes.Buffer
	want := Response{V: Version, ID: "abc", Status: StatusOK, Result: &Result{Kind: "bytes", Data: []byte{0, 1, 2, 255}, Truncated: true}}
	if err := WriteFrame(&buf, want, MaxResponseBytes); err != nil {
		t.Fatal(err)
	}
	got, err := ReadResponse(&buf, "abc")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Result.Data, want.Result.Data) || !got.Result.Truncated {
		t.Errorf("result changed: %+v", got.Result)
	}
}

func TestAFrameThatBreaksARuleIsRefused(t *testing.T) {
	big := strings.Repeat("x", MaxRequestBytes)
	cases := map[string]struct {
		in   []byte
		want error
	}{
		"unknown field":      {frameOf(`{"v":1,"id":"a","action":"x","params":{},"extra":1}`), ErrMalformed},
		"data after the doc": {frameOf(`{"v":1,"id":"a","action":"x","params":{}} {"v":1}`), ErrMalformed},
		"garbage after":      {frameOf(`{"v":1,"id":"a","action":"x","params":{}}xx`), ErrMalformed},
		"wrong version":      {frameOf(`{"v":2,"id":"a","action":"x","params":{}}`), ErrVersion},
		"no version":         {frameOf(`{"id":"a","action":"x","params":{}}`), ErrVersion},
		"no id":              {frameOf(`{"v":1,"action":"x","params":{}}`), ErrMalformed},
		"no action":          {frameOf(`{"v":1,"id":"a","params":{}}`), ErrMalformed},
		"not json":           {frameOf(`hello`), ErrMalformed},
		"zero length":        {frameOf(``), ErrTooLarge},
		"over the cap":       {frameOf(`{"v":1,"id":"` + big + `"}`), ErrTooLarge},
	}
	for name, c := range cases {
		if _, err := ReadRequest(bytes.NewReader(c.in)); !errors.Is(err, c.want) {
			t.Errorf("%s: got %v, want %v", name, err, c.want)
		}
	}
}

// bomb fails the test if more than the header is read: a length over the cap must be refused before the body is touched.
type bomb struct {
	header []byte
	t      *testing.T
	read   int
}

func (b *bomb) Read(p []byte) (int, error) {
	if b.read >= len(b.header) {
		b.t.Error("read past the header of a frame whose length is over the cap")
		return 0, io.EOF
	}
	n := copy(p, b.header[b.read:])
	b.read += n
	return n, nil
}

func TestTheLengthIsCheckedBeforeTheBodyIsRead(t *testing.T) {
	h := make([]byte, 4)
	binary.BigEndian.PutUint32(h, 0xFFFFFFFF)
	if _, err := ReadRequest(&bomb{header: h, t: t}); !errors.Is(err, ErrTooLarge) {
		t.Errorf("got %v", err)
	}
}

func TestAShortFrameIsAnErrorNotAHang(t *testing.T) {
	cut := frameOf(`{"v":1,"id":"a","action":"x","params":{}}`)
	for _, n := range []int{0, 2, 4, len(cut) - 1} {
		if _, err := ReadRequest(bytes.NewReader(cut[:n])); err == nil {
			t.Errorf("a frame cut at %d bytes was accepted", n)
		}
	}
}

func TestAResponseToAnotherRequestIsRefused(t *testing.T) {
	var buf bytes.Buffer
	_ = WriteFrame(&buf, Response{V: Version, ID: "other", Status: StatusOK}, MaxResponseBytes)
	if _, err := ReadResponse(&buf, "mine"); !errors.Is(err, ErrMalformed) {
		t.Errorf("got %v", err)
	}
	buf.Reset()
	_ = WriteFrame(&buf, Response{V: Version, ID: "mine", Status: "maybe"}, MaxResponseBytes)
	if _, err := ReadResponse(&buf, "mine"); !errors.Is(err, ErrMalformed) {
		t.Errorf("an unknown status must be refused, got %v", err)
	}
}

func TestWriteFrameRefusesWhatIsOverTheCap(t *testing.T) {
	r := Request{V: Version, ID: "a", Action: strings.Repeat("x", MaxRequestBytes)}
	if err := WriteFrame(io.Discard, r, MaxRequestBytes); !errors.Is(err, ErrTooLarge) {
		t.Errorf("got %v", err)
	}
}

func TestIDsAreRandom(t *testing.T) {
	a, err1 := NewID()
	b, err2 := NewID()
	if err1 != nil || err2 != nil || a == b || len(a) != 32 {
		t.Errorf("ids %q %q (%v %v)", a, b, err1, err2)
	}
}

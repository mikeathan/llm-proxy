package testssl

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

// The shape is testssl.sh's flat JSON file (--jsonfile): an array of {id, ip, port, severity, finding}. These fixtures follow its documented
// format; they were NOT captured from a real run (testssl.sh was not installed when they were written). Keep one real file as a fixture once there is one.
const flat = `[
 {"id":"SSLv2","ip":"host/192.168.50.10","port":"443","severity":"OK","finding":"not offered"},
 {"id":"SSLv3","ip":"host/192.168.50.10","port":"443","severity":"OK","finding":"not offered"},
 {"id":"TLS1","ip":"host/192.168.50.10","port":"443","severity":"LOW","finding":"offered (deprecated)"},
 {"id":"TLS1_1","ip":"host/192.168.50.10","port":"443","severity":"LOW","finding":"offered (deprecated)"},
 {"id":"TLS1_2","ip":"host/192.168.50.10","port":"443","severity":"OK","finding":"offered"},
 {"id":"TLS1_3","ip":"host/192.168.50.10","port":"443","severity":"OK","finding":"not offered"},
 {"id":"cipherlist_NULL","ip":"host/192.168.50.10","port":"443","severity":"OK","finding":"not offered"},
 {"id":"cipherlist_aNULL","ip":"host/192.168.50.10","port":"443","severity":"OK","finding":"not offered"},
 {"id":"cipherlist_EXPORT","ip":"host/192.168.50.10","port":"443","severity":"OK","finding":"not offered"},
 {"id":"cipherlist_LOW","ip":"host/192.168.50.10","port":"443","severity":"OK","finding":"not offered"},
 {"id":"cipherlist_3DES_IDEA","ip":"host/192.168.50.10","port":"443","severity":"MEDIUM","finding":"offered"},
 {"id":"cipherlist_OBSOLETED","ip":"host/192.168.50.10","port":"443","severity":"OK","finding":"not offered"},
 {"id":"cert_commonName","ip":"host/192.168.50.10","port":"443","severity":"INFO","finding":"<script>alert(1)</script>"}
]`

func TestParse_ReadsEachProtocolAndCipherCategoryAsATypedVerdict(t *testing.T) {
	r, err := Parse([]byte(flat))
	if err != nil {
		t.Fatal(err)
	}
	for p, want := range map[Protocol]Verdict{
		SSLv2: NotOffered, SSLv3: NotOffered, TLS10: Offered, TLS11: Offered, TLS12: Offered, TLS13: NotOffered,
	} {
		if got := r.Protocols[p]; got != want {
			t.Errorf("%s: got %q, want %q", p, got, want)
		}
	}
	for c, want := range map[CipherClass]Verdict{
		CipherNull: NotOffered, CipherAnon: NotOffered, CipherExport: NotOffered, CipherLow: NotOffered, Cipher3DES: Offered, CipherObsolete: NotOffered,
	} {
		if got := r.Ciphers[c]; got != want {
			t.Errorf("%s: got %q, want %q", c, got, want)
		}
	}
}

// What the program printed that is not one of the known ids (a certificate's name, any other line) never reaches the result.
func TestParse_KeepsNoTextFromTheOutput(t *testing.T) {
	r, err := Parse([]byte(flat))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Protocols) != 6 || len(r.Ciphers) != 6 {
		t.Errorf("%d protocols, %d cipher categories: only the known ids are kept", len(r.Protocols), len(r.Ciphers))
	}
	if strings.Contains(string(mustJSON(t, r)), "script") {
		t.Error("program text reached the result")
	}
}

// A line that is neither "offered" nor "not offered" (the check was skipped, or failed) is unknown, never read as clean.
func TestParse_AnythingItCannotReadIsUnknownNeverNotOffered(t *testing.T) {
	r, err := Parse([]byte(`[{"id":"TLS1","finding":"not tested (connection failed)"},{"id":"SSLv3","finding":""},{"id":"TLS1_1","finding":"offered with final"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if r.Protocols[TLS10] != Unknown || r.Protocols[SSLv3] != Unknown {
		t.Errorf("%+v", r.Protocols)
	}
	if r.Protocols[TLS11] != Offered {
		t.Errorf("a line starting with offered is offered: %q", r.Protocols[TLS11])
	}
	if _, ok := r.Protocols[SSLv2]; ok {
		t.Error("a protocol the output never mentioned must be absent, not a guess")
	}
}

func TestParse_RefusesWhatIsNotAnArrayOfFindingsOrIsTooBig(t *testing.T) {
	for name, in := range map[string]string{
		"empty": "", "not json": "TLS1 offered", "an object": `{"id":"TLS1"}`, "an array of numbers": `[1,2]`,
		"too big": `[` + strings.Repeat(`{"id":"x","finding":"y"},`, maxFindings+1) + `{"id":"x","finding":"y"}]`,
	} {
		if _, err := Parse([]byte(in)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestParse_AFileWithNoKnownIdIsAnErrorNotAnEmptyAllClear(t *testing.T) {
	if _, err := Parse([]byte(`[{"id":"scanTime","finding":"12"}]`)); err == nil {
		t.Error("a run that reported none of the protocols must not read as a result")
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// A real file: testssl.sh 3.2.5 on macOS against a stock OpenSSL server on loopback (captured 2026-11-09, `-p -s`, an address as the target).
func TestParse_ReadsARealRunOfTestsslOnMacOS(t *testing.T) {
	data, err := os.ReadFile("testdata/macos-loopback-openssl-server.json")
	if err != nil {
		t.Fatal(err)
	}
	r, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	want := map[Protocol]Verdict{SSLv2: NotOffered, SSLv3: NotOffered, TLS10: NotOffered, TLS11: NotOffered, TLS12: Offered, TLS13: Offered}
	for p, v := range want {
		if r.Protocols[p] != v {
			t.Errorf("%s: got %q, want %q", p, r.Protocols[p], v)
		}
	}
	// A stock server offers the CBC suites testssl.sh calls obsoleted: the category is read, and it is the findings' job not to treat it as weak.
	if r.Ciphers[CipherObsolete] != Offered || r.Ciphers[Cipher3DES] != NotOffered || r.Ciphers[CipherNull] != NotOffered {
		t.Errorf("%+v", r.Ciphers)
	}
}

// A service that speaks TLS offers at least one protocol. A file where every protocol reads "not offered" or unknown means the look never reached
// the service (a jail that blocked the connections, a service that closed them), and reading that as "no old protocols, no weak ciphers" would
// replace nmap's findings with a false all-clear. Seen on Linux 2026-11-10: a stock OpenSSL 3.5 server read TLS 1.2 as not offered.
func TestParse_AFileThatOffersNoProtocolAtAllIsAnErrorNotAnAllClear(t *testing.T) {
	none := `[
 {"id":"SSLv2","finding":"not offered"},{"id":"SSLv3","finding":"not offered"},{"id":"TLS1","finding":"not offered"},
 {"id":"TLS1_1","finding":"not offered"},{"id":"TLS1_2","finding":"not offered"},{"id":"TLS1_3","finding":"not tested"},
 {"id":"cipherlist_NULL","finding":"not offered"}
]`
	if _, err := Parse([]byte(none)); !errors.Is(err, ErrUnreadable) {
		t.Fatalf("no protocol offered must be unreadable, got %v", err)
	}
	if _, err := Parse([]byte(flat)); err != nil {
		t.Errorf("a service that offers a protocol still reads: %v", err)
	}
}

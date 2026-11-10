package footprint

import (
	"context"
	"crypto/sha1" //nolint:gosec // the test computes the service's own scheme
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"llm-proxy/internal/core/tools"
)

func doh(name string, txt ...string) (string, string) {
	rows := make([]string, len(txt))
	for i, t := range txt {
		rows[i] = fmt.Sprintf(`{"name":%q,"type":16,"TTL":300,"data":%q}`, name, `"`+t+`"`)
	}
	return DefaultResolverURL + "?name=" + strings.ReplaceAll(name, "_", "_") + "&type=TXT", `{"Status":0,"Answer":[` + strings.Join(rows, ",") + `]}`
}

func mailFor(spf []string, dmarc []string) (Mail, *fakeOpener) {
	u1, b1 := doh("example.com", spf...)
	u2, b2 := doh("_dmarc.example.com", dmarc...)
	if len(dmarc) == 0 {
		b2 = `{"Status":3}`
	}
	op := &fakeOpener{answers: map[string]string{u1: b1, u2: b2}}
	return Mail{Opener: op}, op
}

func TestMail_JudgesSPFAndDMARC(t *testing.T) {
	cases := []struct {
		name         string
		spf, dmarc   []string
		wantS, wantD string
	}{
		{"both fine", []string{"v=spf1 include:_spf.example.net -all"}, []string{"v=DMARC1; p=reject; rua=mailto:a@example.com"}, MailOK, MailOK},
		{"none at all", nil, nil, SPFMissing, DMARCMissing},
		{"open spf", []string{"v=spf1 +all"}, []string{"v=DMARC1; p=quarantine"}, SPFWeak, MailOK},
		{"soft fail is acceptable", []string{"v=spf1 ip4:192.0.2.0/24 ~all"}, []string{"v=DMARC1; p=none"}, MailOK, DMARCNone},
		{"another txt record is not spf", []string{"google-site-verification=abc"}, nil, SPFMissing, DMARCMissing},
	}
	for _, c := range cases {
		m, _ := mailFor(c.spf, c.dmarc)
		got, err := m.Look(context.Background(), "example.com")
		if err != nil || got.SPF != c.wantS || got.DMARC != c.wantD {
			t.Errorf("%s: %+v, %v", c.name, got, err)
		}
	}
}

// A question that failed is a failed look: half an answer must not read as a clean domain.
func TestMail_OneFailedQuestionFailsTheLook(t *testing.T) {
	m, op := mailFor([]string{"v=spf1 -all"}, nil)
	delete(op.answers, DefaultResolverURL+"?name=_dmarc.example.com&type=TXT")
	_, err := m.Look(context.Background(), "example.com")
	wantReason(t, err, ReasonDown)
	m2, op2 := mailFor(nil, nil)
	for k := range op2.answers {
		op2.answers[k] = `{"Status":2}`
	}
	_, err = m2.Look(context.Background(), "example.com")
	wantReason(t, err, ReasonDown)
}

func TestMail_NetworkOffIsOffNotDown(t *testing.T) {
	m, op := mailFor(nil, nil)
	op.failures = map[string]error{DefaultResolverURL + "?name=example.com&type=TXT": tools.ErrNetworkDisabled}
	_, err := m.Look(context.Background(), "example.com")
	wantReason(t, err, ReasonOff)
}

func TestCerts_ReadsIdsNewestFirstAndKeepsNoNames(t *testing.T) {
	op := &fakeOpener{answers: map[string]string{
		CertLogURL + "?q=%25.example.com&output=json": `[{"id":10,"name_value":"a.example.com\n</x>"},{"id":30,"name_value":"b"},{"id":20},{"id":0}]`,
	}}
	ids, err := CertLog{Opener: op}.Certs(context.Background(), "example.com")
	if err != nil || len(ids) != 3 || ids[0] != 30 || ids[2] != 10 {
		t.Errorf("got %v, %v", ids, err)
	}
}

func TestCerts_ADownLogIsDownAndAnOversizedAnswerIsUnreadable(t *testing.T) {
	_, err := CertLog{Opener: &fakeOpener{}}.Certs(context.Background(), "example.com")
	wantReason(t, err, ReasonDown)
	big := `[` + strings.Repeat(`{"id":1},`, maxCertAnswer/8) + `{"id":2}]`
	op := &fakeOpener{answers: map[string]string{CertLogURL + "?q=%25.example.com&output=json": big}}
	_, err = CertLog{Opener: op}.Certs(context.Background(), "example.com")
	wantReason(t, err, ReasonUnreadable)
}

func TestNewCerts_TheFirstLookIsTheBaseline(t *testing.T) {
	if n := NewCerts(nil, []int64{1, 2, 3}); n != 0 {
		t.Errorf("first look counted %d", n)
	}
	if n := NewCerts([]int64{1, 2}, []int64{4, 2, 1, 3}); n != 2 {
		t.Errorf("got %d", n)
	}
}

func suffixOf(addr string) (prefix, suffix string) {
	sum := sha1.Sum([]byte(addr)) //nolint:gosec // the service's scheme
	full := strings.ToUpper(hex.EncodeToString(sum[:]))
	return full[:6], full[6:]
}

func breachesFor(addr, key string, rows string) (Breaches, *fakeOpener) {
	prefix, _ := suffixOf(addr)
	op := &fakeOpener{answers: map[string]string{BreachURL + prefix: rows}}
	return Breaches{Opener: op, Key: func() string { return key }}, op
}

func TestBreaches_OnlyTheHashPrefixLeavesAndOtherAddressesAreDiscarded(t *testing.T) {
	prefix, suffix := suffixOf("jo@example.com")
	rows := fmt.Sprintf(`[{"hashSuffix":"%s","websites":["Adobe","Gawker"]},{"hashSuffix":"%s","websites":["Someone Else"]}]`, suffix, strings.Repeat("A", 34))
	b, op := breachesFor("jo@example.com", "k", rows)
	names, found, err := b.Look(context.Background(), " Jo@Example.com ")
	if err != nil || found != 2 || len(names) != 2 || names[0] != "Adobe" {
		t.Fatalf("got %v %d %v", names, found, err)
	}
	if len(op.requests) != 1 || op.requests[0] != BreachURL+prefix || strings.Contains(op.requests[0], "jo") || strings.Contains(op.requests[0], suffix) {
		t.Errorf("the request must carry only the six-character prefix: %v", op.requests)
	}
	if op.headers[0]["hibp-api-key"] != "k" {
		t.Errorf("key header: %v", op.headers[0])
	}
}

func TestBreaches_AnAddressNotInTheRangeIsNotFoundNotAnError(t *testing.T) {
	b, _ := breachesFor("jo@example.com", "k", `[{"hashSuffix":"`+strings.Repeat("B", 34)+`","websites":["X"]}]`)
	names, found, err := b.Look(context.Background(), "jo@example.com")
	if err != nil || found != 0 || len(names) != 0 {
		t.Errorf("got %v %d %v", names, found, err)
	}
}

func TestBreaches_ANameThatIsNotAPlainWordIsCountedButNotKept(t *testing.T) {
	_, suffix := suffixOf("jo@example.com")
	b, _ := breachesFor("jo@example.com", "k", fmt.Sprintf(`[{"hashSuffix":"%s","websites":["<script>alert(1)</script>","Adobe"]}]`, suffix))
	names, found, _ := b.Look(context.Background(), "jo@example.com")
	if found != 2 || len(names) != 1 || names[0] != "Adobe" {
		t.Errorf("got %v %d", names, found)
	}
}

func TestBreaches_NoKeyAndRefusedKeyAreSaidPlainly(t *testing.T) {
	b, _ := breachesFor("jo@example.com", "  ", "[]")
	_, _, err := b.Look(context.Background(), "jo@example.com")
	wantReason(t, err, ReasonNoKey)

	prefix, _ := suffixOf("jo@example.com")
	b2 := Breaches{Key: func() string { return "k" }, Opener: &fakeOpener{failures: map[string]error{BreachURL + prefix: &tools.StatusError{Code: 401, Status: "401"}}}}
	_, _, err = b2.Look(context.Background(), "jo@example.com")
	wantReason(t, err, ReasonKeyRefused)
	b3 := Breaches{Key: func() string { return "k" }, Opener: &fakeOpener{failures: map[string]error{BreachURL + prefix: &tools.StatusError{Code: 429, Status: "429"}}}}
	_, _, err = b3.Look(context.Background(), "jo@example.com")
	wantReason(t, err, ReasonRateLimit)
}

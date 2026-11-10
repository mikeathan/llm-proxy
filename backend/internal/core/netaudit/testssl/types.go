// Package testssl runs testssl.sh against one TLS service and reduces what it reports to a fixed vocabulary: which protocol versions the service
// offers and which weak cipher categories it offers. Nothing the program prints beyond that (certificate names, banners, any other line) is kept,
// so a result can feed findings and the agent without carrying text from the network (ADR-0041, Constitution I.8).
package testssl

// Protocol is a TLS or SSL protocol version testssl.sh reports on.
type Protocol string

// The protocol versions kept.
const (
	SSLv2 Protocol = "SSLv2"
	SSLv3 Protocol = "SSLv3"
	TLS10 Protocol = "TLS1.0"
	TLS11 Protocol = "TLS1.1"
	TLS12 Protocol = "TLS1.2"
	TLS13 Protocol = "TLS1.3"
)

// Valid reports whether p is one of the kept versions.
func (p Protocol) Valid() bool {
	switch p {
	case SSLv2, SSLv3, TLS10, TLS11, TLS12, TLS13:
		return true
	}
	return false
}

// CipherClass is one of testssl.sh's standard cipher categories that are weak.
type CipherClass string

// The weak cipher categories kept.
const (
	CipherNull     CipherClass = "null"
	CipherAnon     CipherClass = "anonymous"
	CipherExport   CipherClass = "export"
	CipherLow      CipherClass = "low"
	Cipher3DES     CipherClass = "3des"
	CipherObsolete CipherClass = "obsolete"
)

// Valid reports whether c is one of the kept categories.
func (c CipherClass) Valid() bool {
	switch c {
	case CipherNull, CipherAnon, CipherExport, CipherLow, Cipher3DES, CipherObsolete:
		return true
	}
	return false
}

// Verdict is what the service does about one protocol or category. Unknown is the answer to anything the output did not say plainly: a check
// that was skipped or failed is never read as "not offered".
type Verdict string

// The verdicts.
const (
	Offered    Verdict = "offered"
	NotOffered Verdict = "not_offered"
	Unknown    Verdict = "unknown"
)

// Result is what one look at one service found. A protocol or category the output never mentioned is absent, not guessed.
type Result struct {
	Protocols map[Protocol]Verdict    `json:"protocols"`
	Ciphers   map[CipherClass]Verdict `json:"ciphers"`
}

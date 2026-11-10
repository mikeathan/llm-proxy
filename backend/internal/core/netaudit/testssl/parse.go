package testssl

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// maxFindings bounds how many lines of the output are read; a full run prints a few hundred.
const maxFindings = 2000

// ErrUnreadable means the output was not testssl.sh's flat JSON, or held none of the protocols and categories this package keeps.
var ErrUnreadable = errors.New("testssl: the output could not be read")

// protocolIDs and cipherIDs are testssl.sh's ids for what is kept. Any other id is dropped without being looked at.
var protocolIDs = map[string]Protocol{
	"SSLv2": SSLv2, "SSLv3": SSLv3, "TLS1": TLS10, "TLS1_1": TLS11, "TLS1_2": TLS12, "TLS1_3": TLS13,
}

var cipherIDs = map[string]CipherClass{
	"cipherlist_NULL": CipherNull, "cipherlist_aNULL": CipherAnon, "cipherlist_EXPORT": CipherExport,
	"cipherlist_LOW": CipherLow, "cipherlist_3DES_IDEA": Cipher3DES, "cipherlist_OBSOLETED": CipherObsolete,
}

// line is the only part of an output line that is read.
type line struct {
	ID      string `json:"id"`
	Finding string `json:"finding"`
}

// Parse reads testssl.sh's flat JSON file (--jsonfile). It keeps a verdict for each known protocol and cipher category and nothing else. A
// file that names none of them is an error, so a run that did not work is never an empty all-clear.
func Parse(data []byte) (Result, error) {
	var lines []line
	if err := json.Unmarshal(data, &lines); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
	if len(lines) > maxFindings {
		return Result{}, fmt.Errorf("%w: more than %d lines", ErrUnreadable, maxFindings)
	}
	res := Result{Protocols: map[Protocol]Verdict{}, Ciphers: map[CipherClass]Verdict{}}
	for _, l := range lines {
		if p, ok := protocolIDs[l.ID]; ok {
			res.Protocols[p] = verdictOf(l.Finding)
		} else if c, ok := cipherIDs[l.ID]; ok {
			res.Ciphers[c] = verdictOf(l.Finding)
		}
	}
	if len(res.Protocols) == 0 && len(res.Ciphers) == 0 {
		return Result{}, fmt.Errorf("%w: none of the protocols or cipher categories was reported", ErrUnreadable)
	}
	if !res.offersAProtocol() {
		return Result{}, fmt.Errorf("%w: the service offered no protocol at all, so the look did not reach it", ErrUnreadable)
	}
	return res, nil
}

// offersAProtocol is true when at least one protocol read as offered. A service that speaks TLS offers one, so a file where none is offered did not
// come from talking to it, and its "not offered" lines must never be read as a clean answer.
func (r Result) offersAProtocol() bool {
	for _, v := range r.Protocols {
		if v == Offered {
			return true
		}
	}
	return false
}

// verdictOf reads the program's words for one check. "not offered" is the only way to be clean, "offered" (with any note after it) the only way to
// be offered, and anything else, including an empty line, is unknown.
func verdictOf(finding string) Verdict {
	f := strings.ToLower(strings.TrimSpace(finding))
	switch {
	case strings.HasPrefix(f, "not offered"):
		return NotOffered
	case strings.HasPrefix(f, "offered"):
		return Offered
	}
	return Unknown
}

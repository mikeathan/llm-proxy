package netaudit

import "strings"

// ConfirmedText is a service or product text as the model may read it: a name that is only the usual one for the port, or nmap's
// guess from the shape of a reply, is left out (""), because a model given "afs3-fileserver (guess)" states "an AFS file server" and
// one given nothing has nothing to repeat. The stored findings and the report's tables keep the marked text; only what the model
// is shown goes through this.
func ConfirmedText(s string) string {
	if strings.HasSuffix(s, guessedSuffix) {
		return ""
	}
	return s
}

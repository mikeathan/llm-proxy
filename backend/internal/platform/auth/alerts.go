package auth

// SignInAlert is a sign-in event worth telling the operator about. It carries no address: the Security page lists the events
// with their addresses for whoever signs in to look.
type SignInAlert string

const (
	// SignInLocked is an address locked out after too many wrong passphrases.
	SignInLocked SignInAlert = "locked"
	// SignInNewAddress is a right passphrase from an address that has never signed in before (after the first sign-in ever).
	SignInNewAddress SignInAlert = "new_address"
)

// maxKnownAddresses bounds the addresses remembered as having signed in; the oldest is forgotten first.
const maxKnownAddresses = 32

// WithAlerts makes the service tell fn about lockouts and sign-ins from new addresses. fn must return quickly; it is called on
// the request's path, after the record is written.
func (s *Service) WithAlerts(fn func(SignInAlert)) *Service {
	s.alerts = fn
	return s
}

func (s *Service) tell(a SignInAlert) {
	if s.alerts != nil {
		s.alerts(a)
	}
}

// seedKnownAddresses fills an empty list from what the record already holds (the sessions' and the sign-in events' addresses), so
// a record written before the list existed does not call the operator's own second device new. It runs before the sign-in being
// made is recorded.
func seedKnownAddresses(r *Record) {
	if len(r.SignedInFrom) > 0 {
		return
	}
	seen := map[string]bool{}
	add := func(addr string) {
		if addr != "" && !seen[addr] {
			seen[addr] = true
			r.SignedInFrom = append(r.SignedInFrom, addr)
		}
	}
	for _, e := range r.Events {
		if e.Kind == "login" {
			add(e.Address)
		}
	}
	for _, sess := range r.Sessions {
		add(sess.Address)
	}
	if len(r.SignedInFrom) > maxKnownAddresses {
		r.SignedInFrom = r.SignedInFrom[len(r.SignedInFrom)-maxKnownAddresses:]
	}
}

// rememberAddress adds addr to the addresses that have signed in, and reports whether it is new while others were known.
func rememberAddress(r *Record, addr string) bool {
	for i, a := range r.SignedInFrom {
		if a == addr {
			// Most recent last, so the bound forgets the address unused the longest.
			r.SignedInFrom = append(append(r.SignedInFrom[:i:i], r.SignedInFrom[i+1:]...), addr)
			return false
		}
	}
	news := len(r.SignedInFrom) > 0
	r.SignedInFrom = append(r.SignedInFrom, addr)
	if len(r.SignedInFrom) > maxKnownAddresses {
		r.SignedInFrom = r.SignedInFrom[len(r.SignedInFrom)-maxKnownAddresses:]
	}
	return news
}

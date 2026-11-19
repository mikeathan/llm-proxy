package models

import "time"

// WifiSighting is one access point a sensing stack heard broadcasting one of the owner's Wi-Fi network names, after the core checked it
// (stackport.Networks.Sightings, ADR-0063): BSSID is lower case and colon separated, Security one of the words the core's look-alike check
// knows. ConfirmedBefore says the owner confirmed it on the Radio page before the lists were one; it is shown, never trusted.
type WifiSighting struct {
	Name            string
	BSSID           string
	Security        string
	Channel         int
	Heard           time.Time
	ConfirmedBefore bool
}

package upnp

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"
)

const (
	externalIPAction = "GetExternalIPAddress"
	externalIPWait   = 3 * time.Second
)

// ExternalIP asks the router for the address its internet side has (a read, like the list of forwarded ports). The answer is
// whatever the router says: it can be a private address when another router sits in front of it, and the caller decides
// whether that is a public address.
func (c Client) ExternalIP(ctx context.Context, location, controlPath, serviceType string) (netip.Addr, error) {
	hostport, path, err := controlEndpoint(location, controlPath, serviceType)
	if err != nil {
		return netip.Addr{}, err
	}
	raw, err := c.soapCall(ctx, hostport, path, serviceType, externalIPAction, "", externalIPWait)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("could not read the router's internet address: %w", err)
	}
	return parseExternalIP(raw)
}

type externalIPEnvelope struct {
	Body struct {
		Address string `xml:"GetExternalIPAddressResponse>NewExternalIPAddress"`
		Fault   struct {
			Code string `xml:"detail>UPnPError>errorCode"`
		} `xml:"Fault"`
	} `xml:"Body"`
}

// parseExternalIP reads the router's answer to GetExternalIPAddress. Only an IPv4 address is accepted: an empty value (the router
// is not connected), a name, IPv6 or any other text is an error, never a guess.
func parseExternalIP(raw []byte) (netip.Addr, error) {
	var env externalIPEnvelope
	if err := xml.Unmarshal(raw, &env); err != nil {
		return netip.Addr{}, fmt.Errorf("the router's internet address could not be read: %w", err)
	}
	if code := strings.TrimSpace(env.Body.Fault.Code); code != "" {
		return netip.Addr{}, errors.New("the router refused to give its internet address")
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(env.Body.Address))
	if err != nil || !addr.Is4() {
		return netip.Addr{}, errors.New("the router did not give an internet address")
	}
	return addr, nil
}

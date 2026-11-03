package upnp

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"strings"
)

// Identity is what any UPnP device's description page says about the device itself. The text came from the device and is
// untrusted: the caller cleans it before showing it, and nothing here is put in a finding.
type Identity struct {
	// DeviceType is the whole URN; Gateway is true when it names an Internet Gateway Device.
	DeviceType   string
	Gateway      bool
	FriendlyName string
	Manufacturer string
	ModelName    string
	ModelNumber  string
}

// maxFieldLen bounds each text field kept from a page.
const maxFieldLen = 128

// ParseIdentity reads the root device of a description page. The page is read through a size limit.
func ParseIdentity(r io.Reader) (Identity, error) {
	lr := &io.LimitedReader{R: r, N: maxDescriptionBytes + 1}
	var root struct {
		Device struct {
			DeviceType   string `xml:"deviceType"`
			FriendlyName string `xml:"friendlyName"`
			Manufacturer string `xml:"manufacturer"`
			ModelName    string `xml:"modelName"`
			ModelNumber  string `xml:"modelNumber"`
		} `xml:"device"`
	}
	if err := xml.NewDecoder(lr).Decode(&root); err != nil {
		return Identity{}, fmt.Errorf("the UPnP page could not be read: %w", err)
	}
	if lr.N <= 0 {
		return Identity{}, errors.New("the UPnP page is larger than the limit")
	}
	d := root.Device
	return Identity{
		DeviceType: cut(d.DeviceType), Gateway: strings.HasPrefix(d.DeviceType, gatewayDeviceType),
		FriendlyName: cut(d.FriendlyName), Manufacturer: cut(d.Manufacturer), ModelName: cut(d.ModelName), ModelNumber: cut(d.ModelNumber),
	}, nil
}

func cut(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > maxFieldLen {
		return s[:maxFieldLen]
	}
	return s
}

// Identify reads the description page at location, which must be an http address on the replier itself: an answer that sends
// the reader to another host is how a hostile device would turn this into a request somewhere else. A redirect is not followed.
func (c Client) Identify(ctx context.Context, location string, replier netip.Addr) (Identity, error) {
	if _, err := checkLocation(location, replier); err != nil {
		return Identity{}, err
	}
	return fetchPage(ctx, c.HTTP, location, ParseIdentity)
}

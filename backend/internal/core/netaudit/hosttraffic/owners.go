package hosttraffic

import (
	"cmp"
	"net/netip"
	"slices"

	"llm-proxy/models"
)

// Owners says who holds an internet address, from a table kept on this computer: nothing is sent anywhere to ask (Constitution I.13). It is nil
// until the operator has downloaded the table. This package asks it only about addresses beyond this network.
type Owners func(netip.Addr) (models.TrafficOwner, bool)

const (
	// maxShownOwners and maxShownCountries are how many of the biggest the summary lists.
	maxShownOwners    = 8
	maxShownCountries = 12
)

// holders adds up the table's conversations by network holder and by country, asking the table once for each address.
type holders struct {
	ask       Owners
	byAddr    map[netip.Addr]*models.TrafficOwner
	byASN     map[uint32]*models.TrafficOwnerShare
	byCountry map[string]uint64
}

func newHolders(ask Owners) *holders {
	return &holders{ask: ask, byAddr: map[netip.Addr]*models.TrafficOwner{}, byASN: map[uint32]*models.TrafficOwnerShare{}, byCountry: map[string]uint64{}}
}

func (h *holders) used() bool { return h.ask != nil }

// of looks the conversation's remote address up (once per address) and adds the conversation's bytes to its holder and country. A local address
// is not asked about, and an address that is not in the table has no owner.
func (h *holders) of(addr netip.Addr, f *models.TrafficFlow) *models.TrafficOwner {
	if h.ask == nil || !f.Internet {
		return nil
	}
	addr = addr.Unmap()
	o, seen := h.byAddr[addr]
	if !seen {
		if found, ok := h.ask(addr); ok {
			o = &found
		}
		h.byAddr[addr] = o
	}
	if o == nil {
		return nil
	}
	size := f.BytesIn + f.BytesOut
	share, ok := h.byASN[o.ASN]
	if !ok {
		share = &models.TrafficOwnerShare{TrafficOwner: *o}
		h.byASN[o.ASN] = share
	}
	share.Bytes += size
	if o.Country != "" {
		h.byCountry[o.Country] += size
	}
	return o
}

func (h *holders) owners() []models.TrafficOwnerShare {
	out := make([]models.TrafficOwnerShare, 0, len(h.byASN))
	for _, s := range h.byASN {
		out = append(out, *s)
	}
	slices.SortFunc(out, func(a, b models.TrafficOwnerShare) int {
		return cmp.Or(cmp.Compare(b.Bytes, a.Bytes), cmp.Compare(a.ASN, b.ASN))
	})
	return out[:min(len(out), maxShownOwners)]
}

func (h *holders) countries() []models.TrafficCountryShare {
	out := make([]models.TrafficCountryShare, 0, len(h.byCountry))
	for c, b := range h.byCountry {
		out = append(out, models.TrafficCountryShare{Country: c, Bytes: b})
	}
	slices.SortFunc(out, func(a, b models.TrafficCountryShare) int {
		return cmp.Or(cmp.Compare(b.Bytes, a.Bytes), cmp.Compare(a.Country, b.Country))
	})
	return out[:min(len(out), maxShownCountries)]
}

package netaudit

import (
	"cmp"
	"fmt"
	"slices"
	"time"

	"llm-proxy/models"
)

// PortStore keeps the port record. A missing file is an empty record.
type PortStore interface {
	Read() (models.DevicePorts, error)
	Write(models.DevicePorts) error
}

// PortChange is how many ports opened and closed since the devices' previous scans.
type PortChange struct{ Opened, Closed int }

// PortHistory is where a scan's port changes are added up per day: *History.
type PortHistory interface {
	RecordPorts(PortChange) error
}

// PortLog keeps the ports each device had open at its last completed scan.
type PortLog struct {
	store   PortStore
	history PortHistory
}

// NewPortLog builds the record over a store.
func NewPortLog(store PortStore) *PortLog { return &PortLog{store: store} }

// maxPortDevices bounds the record to the inventory's own size.
const maxPortDevices = maxInventoryDevices

// WithHistory has the log hand each scan's port changes to the Network page's daily counts.
func (l *PortLog) WithHistory(h PortHistory) *PortLog {
	l.history = h
	return l
}

// Record folds one scan into the record and says how many ports opened and closed. Only a host the scan finished looking at
// (up, and not given up on) updates its record: an unknown is not a closed port, so a partial run or a device that did not
// answer closes nothing. A device with no earlier record is a baseline, not "everything opened". The record is keyed by the
// hardware address macOf gives for the host's IP address, so it follows the device; a host with none is skipped.
func (l *PortLog) Record(hosts []Host, macOf func(ip string) string, at time.Time) (PortChange, error) {
	rec, err := l.store.Read()
	if err != nil {
		return PortChange{}, fmt.Errorf("failed to read the port record: %w", err)
	}
	index := map[string]int{}
	for i, d := range rec.Devices {
		index[d.MAC] = i
	}
	var change PortChange
	touched := false
	for _, h := range hosts {
		mac := macOf(h.Addr.String())
		if !h.Up || h.Unscanned || mac == "" {
			continue
		}
		opened, closed := foldHost(&rec, index, mac, openPortRefs(h), at)
		change.Opened += opened
		change.Closed += closed
		touched = true
	}
	if !touched {
		return change, nil
	}
	rec.Devices = newestPortRecords(rec.Devices)
	if err := l.store.Write(rec); err != nil {
		return change, fmt.Errorf("failed to save the port record: %w", err)
	}
	if l.history != nil && change != (PortChange{}) {
		if err := l.history.RecordPorts(change); err != nil {
			return change, fmt.Errorf("failed to note the port changes: %w", err)
		}
	}
	return change, nil
}

// foldHost replaces one device's record with the ports it has open now and says how many opened and closed. A device with no
// record yet is added as a baseline: nothing opened.
func foldHost(rec *models.DevicePorts, index map[string]int, mac string, now []models.PortRef, at time.Time) (opened, closed int) {
	i, known := index[mac]
	if !known {
		index[mac] = len(rec.Devices)
		rec.Devices = append(rec.Devices, models.DevicePortRecord{MAC: mac, Ports: now, ScannedAt: at})
		return 0, 0
	}
	opened, closed = comparePorts(rec.Devices[i].Ports, now)
	rec.Devices[i].Ports, rec.Devices[i].ScannedAt = now, at
	return opened, closed
}

// openPortRefs is a host's open ports as number and protocol only, in port order.
func openPortRefs(h Host) []models.PortRef {
	var out []models.PortRef
	for _, p := range h.Ports {
		if p.State == "open" {
			out = append(out, models.PortRef{Port: p.Number, Proto: p.Proto})
		}
	}
	slices.SortFunc(out, func(a, b models.PortRef) int {
		return cmp.Or(cmp.Compare(a.Port, b.Port), cmp.Compare(a.Proto, b.Proto))
	})
	return slices.Compact(out)
}

// comparePorts counts the ports in now that were not in before (opened) and those in before that are not in now (closed).
func comparePorts(before, now []models.PortRef) (opened, closed int) {
	for _, p := range now {
		if !slices.Contains(before, p) {
			opened++
		}
	}
	for _, p := range before {
		if !slices.Contains(now, p) {
			closed++
		}
	}
	return opened, closed
}

// newestPortRecords keeps the most recently scanned maxPortDevices records.
func newestPortRecords(devices []models.DevicePortRecord) []models.DevicePortRecord {
	if len(devices) <= maxPortDevices {
		return devices
	}
	slices.SortStableFunc(devices, func(a, b models.DevicePortRecord) int { return b.ScannedAt.Compare(a.ScannedAt) })
	return devices[:maxPortDevices]
}

// For is one device's record, or the zero record.
func (l *PortLog) For(mac string) models.DevicePortRecord { return l.All()[mac] }

// All is every device's record, by hardware address. A record that cannot be read is no record.
func (l *PortLog) All() map[string]models.DevicePortRecord {
	rec, err := l.store.Read()
	out := make(map[string]models.DevicePortRecord, len(rec.Devices))
	if err != nil {
		return out
	}
	for _, d := range rec.Devices {
		out[d.MAC] = d
	}
	return out
}

// RecordPorts adds a scan's port changes to today's entry. A scan that changed nothing writes nothing.
func (h *History) RecordPorts(c PortChange) error {
	if c == (PortChange{}) {
		return nil
	}
	return h.edit(func(d *models.NetworkDay) {
		d.PortsOpened += c.Opened
		d.PortsClosed += c.Closed
	})
}

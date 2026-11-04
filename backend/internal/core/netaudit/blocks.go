package netaudit

import (
	"sync"
	"time"

	"llm-proxy/internal/core/netaudit/routerlog"
	"llm-proxy/models"
)

const (
	// blockFlushEvery is how often what was counted is added to the day's history; between writes it is only in memory.
	blockFlushEvery = 5 * time.Minute
	// maxCountedPorts is how many different ports are counted between two writes. An attempt on a port beyond it still counts
	// in the total, so a scan of every port cannot make the counter grow.
	maxCountedPorts = 256
)

// BlockSink takes what was counted: *History.
type BlockSink interface {
	RecordBlocks(day string, total int, ports []models.PortCount) error
}

// Blocks counts the connection attempts from the internet that the router's log says it refused. It keeps a total and a
// bounded table of ports in memory, and adds them to the day's history every few minutes; it keeps no address and no line of
// the log. It is fed by the router log listener (Constitution I.8) and shows only as figures: it never raises a finding or an
// alert, because the log is unauthenticated UDP.
type Blocks struct {
	mu      sync.Mutex
	sink    BlockSink
	onError func(error)
	day     string
	total   int
	ports   map[int]int
	last    time.Time // when counts were last written, as a time of the log
	seen    time.Time // the time of the latest drop heard
}

// NewBlocks builds the counter. onError (optional) hears a write that failed; the counts are kept for the next write.
func NewBlocks(sink BlockSink, onError func(error)) *Blocks {
	return &Blocks{sink: sink, onError: onError, ports: map[int]int{}}
}

// Add counts one drop heard at the given time. A drop heard on a new day first writes the day before under its own date.
func (b *Blocks) Add(d routerlog.Drop, at time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	day := at.Format(historyDateLayout)
	b.seen = at
	if b.day != "" && day != b.day {
		b.report(b.flushLocked())
	}
	if b.day != day {
		b.day, b.last = day, at
	}
	b.total++
	if d.Port > 0 {
		if _, seen := b.ports[d.Port]; seen || len(b.ports) < maxCountedPorts {
			b.ports[d.Port]++
		}
	}
	if at.Sub(b.last) >= blockFlushEvery {
		b.report(b.flushLocked())
	}
}

// Flush adds what was counted to the day's history now. A failed write keeps the counts and returns the error.
func (b *Blocks) Flush() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.flushLocked()
}

func (b *Blocks) flushLocked() error {
	if b.total == 0 {
		return nil
	}
	ports := make([]models.PortCount, 0, len(b.ports))
	for port, count := range b.ports {
		ports = append(ports, models.PortCount{Port: port, Count: count})
	}
	if err := b.sink.RecordBlocks(b.day, b.total, topPorts(ports, 2*MaxBlockedPorts)); err != nil {
		return err
	}
	b.total, b.ports = 0, map[int]int{}
	b.last = b.seen
	return nil
}

func (b *Blocks) report(err error) {
	if err != nil && b.onError != nil {
		b.onError(err)
	}
}

// distinctPorts is how many ports are being counted now; a test reads it to show the table is bounded.
func (b *Blocks) distinctPorts() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.ports)
}

package netaudit

import (
	"context"
	"sync"
	"time"

	"llm-proxy/internal/core/netaudit/routerlog"
	"llm-proxy/models"
)

const (
	// blockFlushEvery is how often Run adds what was counted to the day's history; between writes it is only in memory.
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
// bounded table of ports in memory, and Run adds them to the day's history every few minutes and once more when it stops, so a
// quiet log or a restart never leaves counts unwritten; it keeps no address and no line of the log. It is fed by the router log
// listener (Constitution I.8) and shows only as figures: it never raises a finding or an alert, because the log is
// unauthenticated UDP.
type Blocks struct {
	mu      sync.Mutex
	sink    BlockSink
	onError func(error)
	every   time.Duration // how often Run writes; blockFlushEvery outside tests
	day     string
	total   int
	ports   map[int]int
}

// NewBlocks builds the counter. onError (optional) hears a write that failed; the counts are kept for the next write.
func NewBlocks(sink BlockSink, onError func(error)) *Blocks {
	return &Blocks{sink: sink, onError: onError, every: blockFlushEvery, ports: map[int]int{}}
}

// Add counts one drop heard at the given time. It writes nothing itself, except that a drop heard on a new day first writes the
// day before under its own date.
func (b *Blocks) Add(d routerlog.Drop, at time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	day := at.Format(historyDateLayout)
	if b.day != "" && day != b.day {
		b.report(b.flushLocked())
	}
	b.day = day
	b.total++
	if d.Port > 0 {
		if _, seen := b.ports[d.Port]; seen || len(b.ports) < maxCountedPorts {
			b.ports[d.Port]++
		}
	}
}

// Run writes what was counted every few minutes, and once more when ctx ends, so the counts of the last minutes survive a
// shutdown. A failed write is reported and its counts are kept for the next one. It is started with the application, in the one
// place the Network module's background work is started (Constitution II.2, II.14).
func (b *Blocks) Run(ctx context.Context) {
	tick := time.NewTicker(b.every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			b.report(b.Flush())
			return
		case <-tick.C:
			b.report(b.Flush())
		}
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
	return nil
}

func (b *Blocks) report(err error) {
	if err != nil && b.onError != nil {
		b.onError(err)
	}
}

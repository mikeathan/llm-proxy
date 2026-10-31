package persistence

import (
	"fmt"
	"testing"
	"time"

	"llm-proxy/models"
)

// BenchmarkNetworkHistory_WriteAFullYear measures the one file write a changed day costs, with the history at its bound.
func BenchmarkNetworkHistory_WriteAFullYear(b *testing.B) {
	s := NewNetworkHistoryStore(b.TempDir())
	var h models.NetworkHistory
	day := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 400; i++ {
		h.Days = append(h.Days, models.NetworkDay{Date: day.AddDate(0, 0, i).Format("2006-01-02"), DevicesUp: 12, Devices: 20, Unnamed: 4, Findings: 6, Serious: 1, Audited: true})
	}
	b.ReportAllocs()
	for b.Loop() {
		if err := s.Write(h); err != nil {
			b.Fatal(err)
		}
		if _, err := s.Read(); err != nil {
			b.Fatal(fmt.Sprint(err))
		}
	}
}

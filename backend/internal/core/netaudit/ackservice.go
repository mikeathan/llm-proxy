package netaudit

import (
	"errors"
	"fmt"
	"time"

	"llm-proxy/models"
)

// ErrNoSuchFinding means the finding to accept is not in the automation's latest scan.
var ErrNoSuchFinding = errors.New("no such finding in the latest scan")

// AckStore keeps a workspace's accepted findings. *persistence.WorkspaceManager satisfies it.
type AckStore interface {
	ReadAcks(workspaceID string) ([]models.ScanAck, error)
	WriteAcks(workspaceID string, acks []models.ScanAck) error
}

// Acks is what the operator does with findings: accept one for a while, take the acceptance back, list them.
type Acks struct {
	store     AckStore
	baselines interface {
		ReadBaseline(workspaceID, automation string) (models.ScanBaseline, error)
	}
	now func() time.Time
}

func NewAcks(store AckStore, baselines interface {
	ReadBaseline(workspaceID, automation string) (models.ScanBaseline, error)
}) *Acks {
	return &Acks{store: store, baselines: baselines, now: time.Now}
}

// List returns the workspace's acceptances.
func (a *Acks) List(workspaceID string) ([]models.ScanAck, error) {
	return a.store.ReadAcks(workspaceID)
}

// Accept accepts a finding of the automation's latest scan. The version accepted is the one that scan shows, so the
// operator accepts what they were looking at.
func (a *Acks) Accept(workspaceID, automation, key, note string, days int) (models.ScanAck, error) {
	baseline, err := a.baselines.ReadBaseline(workspaceID, automation)
	if err != nil {
		return models.ScanAck{}, fmt.Errorf("failed to read the latest scan: %w", err)
	}
	var found *Finding
	for i := range baseline.Findings {
		if baseline.Findings[i].Key() == key {
			found = &baseline.Findings[i]
			break
		}
	}
	if found == nil {
		return models.ScanAck{}, ErrNoSuchFinding
	}
	ack, err := NewAck(*found, note, days, a.now())
	if err != nil {
		return models.ScanAck{}, err
	}
	acks, err := a.store.ReadAcks(workspaceID)
	if err != nil {
		return models.ScanAck{}, err
	}
	if err := a.store.WriteAcks(workspaceID, UpsertAck(acks, ack)); err != nil {
		return models.ScanAck{}, err
	}
	return ack, nil
}

// Revoke takes an acceptance back; ok is false when there was none.
func (a *Acks) Revoke(workspaceID, key string) (bool, error) {
	acks, err := a.store.ReadAcks(workspaceID)
	if err != nil {
		return false, err
	}
	left, ok := RemoveAck(acks, key)
	if !ok {
		return false, nil
	}
	return true, a.store.WriteAcks(workspaceID, left)
}

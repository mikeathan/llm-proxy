package models

import (
	"strings"
	"sync"
	"testing"
)

func TestPlanState_BasicMarkComplete(t *testing.T) {
	p := &PlanState{
		Goal: "Test basic flow",
		Steps: []Step{
			{Description: "Step 1", Status: StatusActive},
			{Description: "Step 2", Status: StatusPending},
		},
		LastAutoAdvancedStep: -1,
	}

	if !p.MarkStepComplete() {
		t.Fatal("expected MarkStepComplete to succeed")
	}

	if p.Steps[0].Status != StatusDone {
		t.Errorf("expected step 0 to be DONE, got %s", p.Steps[0].Status)
	}
	if p.Steps[1].Status != StatusActive {
		t.Errorf("expected step 1 to be ACTIVE, got %s", p.Steps[1].Status)
	}
}

func TestPlanState_AutoAdvanceAndConfirm(t *testing.T) {
	p := &PlanState{
		Goal: "Test auto-advance confirmation",
		Steps: []Step{
			{Description: "Step 1", Status: StatusActive},
			{Description: "Step 2", Status: StatusPending},
			{Description: "Step 3", Status: StatusPending},
		},
		LastAutoAdvancedStep: -1,
	}

	// Turn threshold exceeded, auto-advancing Step 1.
	if !p.AutoAdvanceActiveStep() {
		t.Fatal("expected AutoAdvanceActiveStep to succeed")
	}

	if p.Steps[0].Status != StatusDone {
		t.Errorf("expected step 0 to be DONE, got %s", p.Steps[0].Status)
	}
	if p.Steps[1].Status != StatusActive {
		t.Errorf("expected step 1 to be ACTIVE, got %s", p.Steps[1].Status)
	}
	if p.LastAutoAdvancedStep != 0 {
		t.Errorf("expected LastAutoAdvancedStep to be 0, got %d", p.LastAutoAdvancedStep)
	}

	// Model delayed complete_step call arrives for Step 1.
	if !p.ConfirmOrCompleteStep() {
		t.Fatal("expected ConfirmOrCompleteStep to succeed")
	}

	// It should clear LastAutoAdvancedStep and NOT advance Step 2.
	if p.LastAutoAdvancedStep != -1 {
		t.Errorf("expected LastAutoAdvancedStep to be reset to -1, got %d", p.LastAutoAdvancedStep)
	}
	if p.Steps[1].Status != StatusActive {
		t.Errorf("expected step 1 to remain ACTIVE, got %s", p.Steps[1].Status)
	}
	if p.Steps[2].Status != StatusPending {
		t.Errorf("expected step 2 to remain PENDING, got %s", p.Steps[2].Status)
	}

	// Model completes Step 2 normally.
	if !p.ConfirmOrCompleteStep() {
		t.Fatal("expected ConfirmOrCompleteStep to succeed")
	}
	if p.Steps[1].Status != StatusDone {
		t.Errorf("expected step 1 to be DONE, got %s", p.Steps[1].Status)
	}
	if p.Steps[2].Status != StatusActive {
		t.Errorf("expected step 2 to be ACTIVE, got %s", p.Steps[2].Status)
	}
}

func TestPlanState_ConsecutiveAutoAdvance(t *testing.T) {
	p := &PlanState{
		Goal: "Test consecutive auto-advances",
		Steps: []Step{
			{Description: "Step 1", Status: StatusActive},
			{Description: "Step 2", Status: StatusPending},
			{Description: "Step 3", Status: StatusPending},
		},
		LastAutoAdvancedStep: -1,
	}

	// Auto-advance Step 1.
	p.AutoAdvanceActiveStep()
	// Auto-advance Step 2.
	p.AutoAdvanceActiveStep()

	if p.Steps[0].Status != StatusDone || p.Steps[1].Status != StatusDone {
		t.Error("expected steps 0 and 1 to be DONE")
	}
	if p.Steps[2].Status != StatusActive {
		t.Errorf("expected step 2 to be ACTIVE, got %s", p.Steps[2].Status)
	}
	if p.LastAutoAdvancedStep != 1 {
		t.Errorf("expected LastAutoAdvancedStep to be 1, got %d", p.LastAutoAdvancedStep)
	}

	// Confirm Step 2 complete.
	if !p.ConfirmOrCompleteStep() {
		t.Fatal("expected confirmation of Step 2 to succeed")
	}
	if p.Steps[2].Status != StatusActive {
		t.Errorf("expected step 2 to remain ACTIVE, got %s", p.Steps[2].Status)
	}
	if p.LastAutoAdvancedStep != -1 {
		t.Errorf("expected LastAutoAdvancedStep to be reset, got %d", p.LastAutoAdvancedStep)
	}
}

func TestPlanState_ToCompactStateExcludesField(t *testing.T) {
	p := &PlanState{
		Goal: "Compact state test",
		Steps: []Step{
			{Description: "Step 1", Status: StatusActive},
		},
		LastAutoAdvancedStep: 42,
	}

	compact := p.ToCompactState()
	if strings.Contains(compact, "LastAutoAdvancedStep") || strings.Contains(compact, "42") {
		t.Errorf("ToCompactState leaked internal field: %s", compact)
	}
}

func TestPlanState_Concurrency(t *testing.T) {
	p := &PlanState{
		Goal: "Concurrent test",
		Steps: []Step{
			{Description: "Step 1", Status: StatusActive},
			{Description: "Step 2", Status: StatusPending},
		},
		LastAutoAdvancedStep: -1,
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.ToCompactState()
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		p.AutoAdvanceActiveStep()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		p.ConfirmOrCompleteStep()
	}()

	wg.Wait()
}

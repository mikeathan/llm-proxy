package models

import (
	"fmt"
	"strings"
	"sync"
)

// StepStatus represents the execution state of a single task step.
type StepStatus string

const (
	// StatusPending indicates the step has not been started yet.
	StatusPending StepStatus = "PENDING"
	// StatusActive indicates the step is currently being executed.
	StatusActive StepStatus = "ACTIVE"
	// StatusDone indicates the step has been completed successfully.
	StatusDone StepStatus = "DONE"
)

// Step represents one unit of work in the execution plan.
type Step struct {
	Description string
	Status      StepStatus
}

// PlanState tracks progress through a multi-step task. Rendered as a compact
// block at index [1] of the prompt, it survives sieve truncation so the model
// never loses its place after history pruning.
type PlanState struct {
	mu                   sync.RWMutex
	Goal                 string
	Steps                []Step
	LastAutoAdvancedStep int // Index of the last step that was auto-advanced
}

// ToCompactState renders the execution plan as a dense, token-efficient
// block suitable for injection at index [1] of the prompt. Survives sieve
// truncation — the model always sees what steps are done.
func (p *PlanState) ToCompactState() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Goal: %s\n", p.Goal))
	for _, step := range p.Steps {
		b.WriteString(fmt.Sprintf("- [%s] %s\n", step.Status, step.Description))
	}
	return b.String()
}

// MarkStepComplete transitions the active step to DONE and advances the
// next pending step to ACTIVE. Returns false when no active step is found
// (all steps already done or none active).
func (p *PlanState) MarkStepComplete() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range p.Steps {
		if p.Steps[i].Status == StatusActive {
			p.Steps[i].Status = StatusDone
			if i+1 < len(p.Steps) {
				p.Steps[i+1].Status = StatusActive
			}
			return true
		}
	}
	return false
}

// AutoAdvanceActiveStep transitions active step to DONE, overwrites the last
// auto-advanced step index with the new active step index, and activates the next step.
func (p *PlanState) AutoAdvanceActiveStep() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range p.Steps {
		if p.Steps[i].Status == StatusActive {
			p.Steps[i].Status = StatusDone
			p.LastAutoAdvancedStep = i
			if i+1 < len(p.Steps) {
				p.Steps[i+1].Status = StatusActive
			}
			return true
		}
	}
	return false
}

// ConfirmOrCompleteStep resolves a delayed complete_step call on a step that auto-advanced.
func (p *PlanState) ConfirmOrCompleteStep() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	activeIdx := -1
	for i := range p.Steps {
		if p.Steps[i].Status == StatusActive {
			activeIdx = i
			break
		}
	}
	if activeIdx > 0 && p.LastAutoAdvancedStep == activeIdx-1 {
		p.LastAutoAdvancedStep = -1
		return true
	}
	for i := range p.Steps {
		if p.Steps[i].Status == StatusActive {
			p.Steps[i].Status = StatusDone
			if i+1 < len(p.Steps) {
				p.Steps[i+1].Status = StatusActive
			}
			return true
		}
	}
	return false
}

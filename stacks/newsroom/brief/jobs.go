package brief

import (
	"errors"
	"slices"
	"sync"
	"time"
)

// Stage is what writing an edition by hand is doing: reading the sources of topics not read lately, or asking the model for a section.
type Stage string

// The stages of a write.
const (
	StageReading Stage = "reading"
	StageWriting Stage = "writing"
)

// Step is how far a write has got: Done of Total in its stage, and the section being written (empty while reading).
type Step struct {
	Stage   Stage  `json:"stage"`
	Done    int    `json:"done"`
	Total   int    `json:"total"`
	Section string `json:"section,omitempty"`
}

// Progress is told each step of a write; nil tells no one.
type Progress func(Step)

func (p Progress) tell(s Step) {
	if p != nil {
		p(s)
	}
}

// JobState is where a write by hand is: writing, done (IssueID is the edition written) or failed (Problem says why).
type JobState string

// The states of a write by hand. A write moves from writing to done or failed, and a new write may start only when none is writing.
const (
	JobWriting JobState = "writing"
	JobDone    JobState = "done"
	JobFailed  JobState = "failed"
)

// JobKeep is how long a finished write is still listed, so a page that polls sees how it ended.
const JobKeep = 10 * time.Minute

// ErrBusy refuses a second write of an edition while one is writing.
var ErrBusy = errors.New("this brief is already being written")

// errNotWriting is an End for an edition that has no write in progress: a bug in the caller, never the owner's doing.
var errNotWriting = errors.New("no write of this brief is in progress")

// Job is one write of an edition by hand, as the page reads it.
type Job struct {
	EditionID int64     `json:"edition_id"`
	State     JobState  `json:"state"`
	Step      Step      `json:"step"`
	IssueID   int64     `json:"issue_id,omitempty"`
	Problem   string    `json:"problem,omitempty"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at,omitzero"`
}

// Jobs keeps the writes by hand, one per edition, in memory: a write is lost with the process, and the next page load starts afresh.
type Jobs struct {
	mu   sync.Mutex
	jobs map[int64]Job
}

// Begin starts a write of the edition, or refuses with ErrBusy while one is writing.
func (j *Jobs) Begin(editionID int64, now time.Time) (Job, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.jobs == nil {
		j.jobs = map[int64]Job{}
	}
	if cur, ok := j.jobs[editionID]; ok && cur.State == JobWriting {
		return cur, ErrBusy
	}
	job := Job{EditionID: editionID, State: JobWriting, Step: Step{Stage: StageReading}, StartedAt: now.UTC()}
	j.jobs[editionID] = job
	return job, nil
}

// Progress returns the hook that records each step of the edition's write.
func (j *Jobs) Progress(editionID int64) Progress {
	return func(s Step) {
		j.mu.Lock()
		defer j.mu.Unlock()
		if cur, ok := j.jobs[editionID]; ok && cur.State == JobWriting {
			cur.Step = s
			j.jobs[editionID] = cur
		}
	}
}

// End finishes the edition's write: done with the issue written, or failed with the reason the page may show.
func (j *Jobs) End(editionID int64, issueID int64, problem string, now time.Time) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	cur, ok := j.jobs[editionID]
	if !ok || cur.State != JobWriting {
		return errNotWriting
	}
	cur.EndedAt, cur.IssueID, cur.Problem, cur.State = now.UTC(), issueID, problem, JobDone
	if problem != "" {
		cur.State = JobFailed
	}
	j.jobs[editionID] = cur
	return nil
}

// List returns the writes in edition order: every one writing, and those that ended less than JobKeep ago.
func (j *Jobs) List(now time.Time) []Job {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := []Job{}
	for id, job := range j.jobs {
		if job.State != JobWriting && now.Sub(job.EndedAt) > JobKeep {
			delete(j.jobs, id)
			continue
		}
		out = append(out, job)
	}
	slices.SortFunc(out, func(a, b Job) int { return int(a.EditionID - b.EditionID) })
	return out
}

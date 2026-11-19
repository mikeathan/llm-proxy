// Package dashboard keeps the owner's dashboard layout: the boards (tabs), the cards on each and each card's size. It holds
// no stack data. The screen builds its shipped defaults when nothing is stored, so an empty layout is a normal answer.
package dashboard

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"llm-proxy/models"
)

// The limits keep one request, and the file it becomes, small.
const (
	MaxBoards  = 20
	MaxCards   = 40
	MaxIDLen   = 40
	MaxNameLen = 40
)

// ErrInvalid marks a layout the server refuses; the message says which part and why.
var ErrInvalid = errors.New("the dashboard layout is not valid")

// idPattern is a short lower-case word with digits and hyphens: safe to show, store and use as a key.
var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// allowedCols are the widths the grid offers (S, M, M+, L, Full on a grid of twelve columns); allowedRows the heights.
var (
	allowedCols = map[int]bool{3: true, 4: true, 6: true, 8: true, 12: true}
	allowedRows = map[int]bool{1: true, 2: true}
)

// Store keeps the layout. A missing file is an empty layout.
type Store interface {
	Read() (models.Dashboard, error)
	Write(models.Dashboard) error
	Delete() error
}

// Manager reads and replaces the layout.
type Manager struct{ store Store }

func New(store Store) *Manager { return &Manager{store: store} }

// Get returns the layout. Nothing stored is an empty list of boards, never nil.
func (m *Manager) Get() (models.Dashboard, error) {
	d, err := m.store.Read()
	if err != nil {
		return models.Dashboard{Boards: []models.DashboardBoard{}}, fmt.Errorf("failed to read the dashboard layout: %w", err)
	}
	return normalise(d), nil
}

// Put checks the layout and replaces the stored one with it, returning what was stored. A refused layout stores nothing.
func (m *Manager) Put(d models.Dashboard) (models.Dashboard, error) {
	d = normalise(d)
	if err := validate(d); err != nil {
		return models.Dashboard{}, err
	}
	if err := m.store.Write(d); err != nil {
		return models.Dashboard{}, fmt.Errorf("failed to save the dashboard layout: %w", err)
	}
	return d, nil
}

// Reset forgets the layout, so the screen shows its defaults again. A factory reset calls it too.
func (m *Manager) Reset() error { return m.store.Delete() }

// normalise trims names and makes every list non-nil so the JSON always has [] and never null.
func normalise(d models.Dashboard) models.Dashboard {
	out := models.Dashboard{Boards: make([]models.DashboardBoard, 0, len(d.Boards))}
	for _, b := range d.Boards {
		b.Name = strings.TrimSpace(b.Name)
		cards := make([]models.DashboardCard, 0, len(b.Cards))
		b.Cards = append(cards, b.Cards...)
		out.Boards = append(out.Boards, b)
	}
	return out
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

func validate(d models.Dashboard) error {
	if len(d.Boards) > MaxBoards {
		return invalid("at most %d boards", MaxBoards)
	}
	seen := map[string]bool{}
	for _, b := range d.Boards {
		if err := validateID("board", b.ID); err != nil {
			return err
		}
		if seen[b.ID] {
			return invalid("board %q appears twice", b.ID)
		}
		seen[b.ID] = true
		if err := validateName(b); err != nil {
			return err
		}
		if err := validateCards(b); err != nil {
			return err
		}
	}
	return nil
}

func validateID(what, id string) error {
	if id == "" || utf8.RuneCountInString(id) > MaxIDLen || !idPattern.MatchString(id) {
		return invalid("a %s id is 1 to %d lower-case letters, digits or hyphens, got %q", what, MaxIDLen, id)
	}
	return nil
}

func validateName(b models.DashboardBoard) error {
	n := utf8.RuneCountInString(b.Name)
	if n == 0 || n > MaxNameLen {
		return invalid("board %q needs a name of 1 to %d characters", b.ID, MaxNameLen)
	}
	for _, r := range b.Name {
		if unicode.IsControl(r) {
			return invalid("board %q has a control character in its name", b.ID)
		}
	}
	return nil
}

func validateCards(b models.DashboardBoard) error {
	if len(b.Cards) > MaxCards {
		return invalid("board %q has more than %d cards", b.ID, MaxCards)
	}
	onBoard := map[string]bool{}
	for _, c := range b.Cards {
		if err := validateID("card", c.ID); err != nil {
			return err
		}
		if onBoard[c.ID] {
			return invalid("card %q is twice on board %q", c.ID, b.ID)
		}
		onBoard[c.ID] = true
		if !allowedCols[c.Cols] {
			return invalid("card %q has a width of %d columns; the grid offers 3, 4, 6, 8 or 12", c.ID, c.Cols)
		}
		if !allowedRows[c.Rows] {
			return invalid("card %q has a height of %d rows; the grid offers 1 or 2", c.ID, c.Rows)
		}
	}
	return nil
}

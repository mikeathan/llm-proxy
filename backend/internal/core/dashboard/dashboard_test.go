package dashboard

import (
	"errors"
	"strings"
	"testing"

	"llm-proxy/models"
)

type memStore struct {
	d        models.Dashboard
	readErr  error
	writeErr error
	deleted  bool
}

func (m *memStore) Read() (models.Dashboard, error) { return m.d, m.readErr }
func (m *memStore) Write(d models.Dashboard) error {
	if m.writeErr != nil {
		return m.writeErr
	}
	m.d = d
	return nil
}
func (m *memStore) Delete() error { m.deleted = true; m.d = models.Dashboard{}; return nil }

func card(id string, cols, rows int) models.DashboardCard {
	return models.DashboardCard{ID: id, Cols: cols, Rows: rows}
}

func board(id, name string, cards ...models.DashboardCard) models.DashboardBoard {
	return models.DashboardBoard{ID: id, Name: name, Cards: cards}
}

func TestGet_NothingStoredIsAnEmptyLayoutSoTheScreenUsesItsDefaults(t *testing.T) {
	m := New(&memStore{})
	got, err := m.Get()
	if err != nil || len(got.Boards) != 0 || got.Boards == nil {
		t.Fatalf("Get() = %+v, %v; want an empty, non-nil list of boards and no error", got, err)
	}
}

func TestGet_ReportsAnUnreadableStore(t *testing.T) {
	if _, err := New(&memStore{readErr: errors.New("boom")}).Get(); err == nil {
		t.Fatal("an unreadable store must be an error, not an empty layout that the next save would overwrite")
	}
}

func TestPut_StoresAValidLayoutAndReturnsIt(t *testing.T) {
	st := &memStore{}
	in := models.Dashboard{Boards: []models.DashboardBoard{board("home", "Home", card("host-health", 4, 1), card("radar", 8, 2))}}
	got, err := New(st).Put(in)
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if len(st.d.Boards) != 1 || st.d.Boards[0].Cards[1].Cols != 8 || len(got.Boards) != 1 {
		t.Fatalf("stored %+v, returned %+v", st.d, got)
	}
}

func TestPut_TrimsTheBoardName(t *testing.T) {
	st := &memStore{}
	if _, err := New(st).Put(models.Dashboard{Boards: []models.DashboardBoard{board("home", "  Home  ")}}); err != nil {
		t.Fatal(err)
	}
	if st.d.Boards[0].Name != "Home" {
		t.Fatalf("name = %q, want trimmed", st.d.Boards[0].Name)
	}
}

func TestPut_RefusesWhatCannotBeADashboard(t *testing.T) {
	many := make([]models.DashboardBoard, MaxBoards+1)
	for i := range many {
		many[i] = board(string(rune('a'+i%26))+strings.Repeat("x", i/26), "B")
	}
	tooManyCards := make([]models.DashboardCard, MaxCards+1)
	for i := range tooManyCards {
		tooManyCards[i] = card(strings.Repeat("c", 1+i), 4, 1)
	}
	cases := map[string]models.Dashboard{
		"too many boards":           {Boards: many},
		"too many cards":            {Boards: []models.DashboardBoard{board("home", "Home", tooManyCards...)}},
		"empty board id":            {Boards: []models.DashboardBoard{board("", "Home")}},
		"board id with a slash":     {Boards: []models.DashboardBoard{board("a/b", "Home")}},
		"board id with capitals":    {Boards: []models.DashboardBoard{board("Home", "Home")}},
		"board id too long":         {Boards: []models.DashboardBoard{board(strings.Repeat("a", MaxIDLen+1), "Home")}},
		"duplicate board id":        {Boards: []models.DashboardBoard{board("home", "A"), board("home", "B")}},
		"empty name":                {Boards: []models.DashboardBoard{board("home", "   ")}},
		"name too long":             {Boards: []models.DashboardBoard{board("home", strings.Repeat("n", MaxNameLen+1))}},
		"name with a control char":  {Boards: []models.DashboardBoard{board("home", "Ho\nme")}},
		"card id with a space":      {Boards: []models.DashboardBoard{board("home", "Home", card("a b", 4, 1))}},
		"duplicate card on a board": {Boards: []models.DashboardBoard{board("home", "Home", card("x", 4, 1), card("x", 4, 1))}},
		"width not offered":         {Boards: []models.DashboardBoard{board("home", "Home", card("x", 5, 1))}},
		"width zero":                {Boards: []models.DashboardBoard{board("home", "Home", card("x", 0, 1))}},
		"height not offered":        {Boards: []models.DashboardBoard{board("home", "Home", card("x", 4, 3))}},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			st := &memStore{}
			_, err := New(st).Put(in)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("Put() error = %v, want ErrInvalid", err)
			}
			if len(st.d.Boards) != 0 {
				t.Fatalf("a refused layout must not be stored, got %+v", st.d)
			}
		})
	}
}

func TestPut_TheSameCardMayBeOnTwoBoards(t *testing.T) {
	in := models.Dashboard{Boards: []models.DashboardBoard{board("home", "Home", card("radar", 4, 1)), board("radio", "Radio", card("radar", 8, 2))}}
	if _, err := New(&memStore{}).Put(in); err != nil {
		t.Fatalf("a card pinned to Home also lives on its stack board: %v", err)
	}
}

func TestPut_AStoreFailureIsNotAnInvalidLayout(t *testing.T) {
	_, err := New(&memStore{writeErr: errors.New("disk full")}).Put(models.Dashboard{})
	if err == nil || errors.Is(err, ErrInvalid) {
		t.Fatalf("Put() error = %v, want a storage error that is not ErrInvalid", err)
	}
}

func TestReset_ForgetsTheLayout(t *testing.T) {
	st := &memStore{d: models.Dashboard{Boards: []models.DashboardBoard{board("home", "Home")}}}
	if err := New(st).Reset(); err != nil || !st.deleted {
		t.Fatalf("Reset() = %v, deleted = %v", err, st.deleted)
	}
}

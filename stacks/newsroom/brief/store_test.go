package brief

import (
	"errors"
	"testing"
	"time"

	"llm-proxy-stacks/newsroom/sources"
	"llm-proxy-stacks/sdk"
)

// memDir is an in-memory DataDir.
type memDir struct {
	files       map[string][]byte
	quarantined []string
}

func newMemDir() *memDir { return &memDir{files: map[string][]byte{}} }

func (m *memDir) Read(name string) ([]byte, error) {
	b, ok := m.files[name]
	if !ok {
		return nil, stacksdk.Errorf(stacksdk.CodeNotFound, "no such file")
	}
	return b, nil
}
func (m *memDir) Write(name string, data []byte) error {
	m.files[name] = append([]byte(nil), data...)
	return nil
}
func (m *memDir) Quarantine(name string) error {
	m.quarantined = append(m.quarantined, name)
	delete(m.files, name)
	return nil
}

func newStore(t *testing.T) (*Store, *memDir) {
	t.Helper()
	dir := newMemDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s, dir
}

var worldTopic = Topic{Name: "World", Queries: []string{"Ukraine"}}

func TestStore_TopicsAndEditionsSurviveARestart(t *testing.T) {
	s, dir := newStore(t)
	topic, err := s.CreateTopic(worldTopic, t0)
	if err != nil {
		t.Fatal(err)
	}
	ed, err := s.CreateEdition(Edition{Name: "Morning", ReadyBy: "07:00", Enabled: true, Sections: []Section{{Kind: SectionNews, TopicID: topic.ID}}}, t0)
	if err != nil || ed.ID == topic.ID {
		t.Fatalf("topics and editions share one id counter: %+v, %v", ed, err)
	}
	if _, err := s.RecordCollection(topic.ID, []sources.Item{item("BBC", "", "A story", "https://e/1", t0)}, []string{"x: down"}, t0); err != nil {
		t.Fatal(err)
	}
	again, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	gotTopic, _ := again.Topic(topic.ID)
	if gotTopic.LastNew != 1 || len(gotTopic.LastProblems) != 1 || len(again.Pool()) != 1 || len(again.Editions()) != 1 {
		t.Errorf("everything must come back: topic %+v, pool %d, editions %d", gotTopic, len(again.Pool()), len(again.Editions()))
	}
}

func TestStore_UpdateKeepsTheCollectionRecord(t *testing.T) {
	s, _ := newStore(t)
	topic, _ := s.CreateTopic(worldTopic, t0)
	_, _ = s.RecordCollection(topic.ID, []sources.Item{item("BBC", "", "A story", "https://e/1", t0)}, nil, t0)
	updated, err := s.UpdateTopic(topic.ID, Topic{Name: "World news", Queries: []string{"Ukraine", "Gaza"}})
	if err != nil || updated.Name != "World news" || updated.LastNew != 1 || updated.LastCollected.IsZero() || !updated.CreatedAt.Equal(t0) {
		t.Errorf("an edit changes what the owner may edit and nothing else: %+v, %v", updated, err)
	}
	if _, err := s.UpdateTopic(99, worldTopic); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v", err)
	}
}

func TestStore_ATopicAnEditionListsCannotBeDeleted(t *testing.T) {
	s, _ := newStore(t)
	topic, _ := s.CreateTopic(worldTopic, t0)
	ed, _ := s.CreateEdition(Edition{Name: "Morning", ReadyBy: "07:00", Sections: []Section{{Kind: SectionNews, TopicID: topic.ID}}}, t0)
	if err := s.DeleteTopic(topic.ID); !errors.Is(err, ErrInUse) {
		t.Fatalf("err = %v, want ErrInUse", err)
	}
	if err := s.DeleteEdition(ed.ID); err != nil {
		t.Fatal(err)
	}
	_, _ = s.RecordCollection(topic.ID, []sources.Item{item("BBC", "", "A story", "https://e/1", t0)}, nil, t0)
	if err := s.DeleteTopic(topic.ID); err != nil {
		t.Fatal(err)
	}
	if len(s.Pool()) != 0 {
		t.Error("a story no topic wants any more goes with the topic")
	}
}

func TestStore_BoundsAndMissingTopic(t *testing.T) {
	s, _ := newStore(t)
	for range MaxTopics {
		if _, err := s.CreateTopic(worldTopic, t0); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.CreateTopic(worldTopic, t0); !errors.Is(err, ErrInvalid) {
		t.Errorf("topics are bounded: %v", err)
	}
	if _, err := s.CreateEdition(Edition{Name: "x", ReadyBy: "07:00", Sections: []Section{{Kind: SectionNews, TopicID: 9999}}}, t0); !errors.Is(err, ErrInvalid) {
		t.Errorf("an edition of a topic that does not exist is refused: %v", err)
	}
	if _, err := s.RecordCollection(9999, nil, nil, t0); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v", err)
	}
}

func TestStore_PoolIsNewestFirstWhateverTheFileOrder(t *testing.T) {
	s, _ := newStore(t)
	topic, _ := s.CreateTopic(worldTopic, t0)
	_, _ = s.RecordCollection(topic.ID, []sources.Item{item("a", "", "First one", "https://e/1", time.Time{})}, nil, t0)
	_, _ = s.RecordCollection(topic.ID, []sources.Item{item("a", "", "Second one", "https://e/2", time.Time{}), item("a", "", "Third one", "https://e/3", time.Time{})}, nil, t0.Add(time.Hour))
	got := s.Pool()
	if len(got) != 3 || got[0].Title != "Third one" || got[1].Title != "Second one" || got[2].Title != "First one" {
		t.Errorf("newest first, then by id: %v %v %v", got[0].Title, got[1].Title, got[2].Title)
	}
}

func TestStore_ABadFileIsSetAsideAndTheStoreStartsEmpty(t *testing.T) {
	dir := newMemDir()
	dir.files[FileName] = []byte("{not json")
	s, err := NewStore(dir)
	if err != nil || len(s.Topics()) != 0 || len(dir.quarantined) != 1 {
		t.Fatalf("err %v, topics %d, set aside %v", err, len(s.Topics()), dir.quarantined)
	}
	if err := s.Erase(); err != nil {
		t.Fatal(err)
	}
}

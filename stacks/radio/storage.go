package radio

import (
	"context"
	"errors"
	"io/fs"
	"os"

	"llm-proxy-stacks/sdk"
)

var _ stacksdk.StorageReporter = (*Stack)(nil)

// StorageUse reports the database for the data management page (ADR-0053). The stack is idle until something is stored: with no file there is
// nothing to report and nothing is created. It only reads.
func (s *Stack) StorageUse(ctx context.Context) ([]stacksdk.StorageUse, error) {
	st, err := s.existingStore()
	if err != nil || st == nil {
		return nil, err
	}
	return st.StorageUse(ctx)
}

// existingStore opens the database when its file is already there, and returns nil when it is not (or the stack is not running).
func (s *Stack) existingStore() (*Store, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.store != nil {
		return s.store, nil
	}
	if !s.started {
		return nil, nil
	}
	fp, ok := s.h.DataDir.(stacksdk.FilePather)
	if !ok {
		return nil, nil
	}
	p, err := fp.Path(DBFile)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(p); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	st, err := OpenStore(p, 0)
	if err != nil {
		return nil, stacksdk.Errorf(stacksdk.CodeInternal, "the radio database could not be opened")
	}
	s.store = st
	return st, nil
}

func (s *Stack) closeStoreLocked() error {
	if s.store == nil {
		return nil
	}
	err := s.store.Close()
	s.store = nil
	return err
}

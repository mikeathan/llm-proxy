package radio

import (
	"context"
	"database/sql"
	"time"
)

// decided is one device's row and what the classifier now says about it.
type decided struct {
	row FeatureRow
	d   decision
}

// SetDecisions writes a cycle's decisions in one transaction: the class, its rules version, the confidence and the change it is waiting on. A
// class the owner set is never overwritten. Each change of class adds one row to the class log, with its reason. It returns how many changed class.
func (s *Store) SetDecisions(ctx context.Context, list []decided, version int, at time.Time) (int, error) {
	if len(list) == 0 {
		return 0, nil
	}
	changed := 0
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		changed = 0
		for _, x := range list {
			moved, err := setDecision(ctx, tx, x, version, at)
			if err != nil {
				return err
			}
			if moved {
				changed++
			}
		}
		return nil
	})
	return changed, err
}

func setDecision(ctx context.Context, tx *sql.Tx, x decided, version int, at time.Time) (bool, error) {
	res, err := tx.ExecContext(ctx, `UPDATE devices SET class = ?, class_ver = ?, conf = ?, pending = ?, pending_n = ? WHERE tag = ? AND NOT (class != '' AND class_ver = 0)`,
		string(x.d.class), version, x.d.conf, x.d.pending, x.d.pendingN, x.row.Tag)
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n == 0 || string(x.d.class) == x.row.Class {
		return false, nil
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO class_log(day, tag, owner, from_class, to_class, version, reason) VALUES(?,?,?,?,?,?,?)`,
		dayNumber(at), x.row.Tag, x.row.Owner, x.row.Class, string(x.d.class), version, string(x.d.why))
	return err == nil, err
}

// LearnedModel reads the summary of the model the last cycle trained, and whether there is one.
func (s *Store) LearnedModel(ctx context.Context) (Model, bool, error) {
	var m Model
	ok, err := s.GetDoc(ctx, learnedDoc, &m)
	return m, ok, err
}

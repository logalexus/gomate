package anomaly

import (
	"context"
	"errors"
	"testing"
	"time"
)

type notificationSink struct{ err error }

func (s *notificationSink) SaveAnomalies(context.Context, []Result) error { return s.err }

func TestNotifyOnlyAfterSuccessfulFlush(t *testing.T) {
	sink := &notificationSink{err: errors.New("temporary failure")}
	e := newEngine(catalog(t), sink, time.Second)
	calls := 0
	e.onSaved = func(rows []Result) {
		calls++
		if len(rows) != 1 || rows[0].StreamID != 12 {
			t.Fatal(rows)
		}
	}
	e.pending[12] = Result{StreamID: 12, Score: 42}
	e.flush()
	if calls != 0 || len(e.pending) != 1 {
		t.Fatal("failed write was notified or lost")
	}
	sink.err = nil
	e.flush()
	e.flush()
	if calls != 1 || len(e.pending) != 0 {
		t.Fatal("successful write not notified exactly once")
	}
}

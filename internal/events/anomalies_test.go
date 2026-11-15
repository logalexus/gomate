package events

import (
	"gomate/internal/anomaly"
	"testing"
)

func TestAnomalyFeed(t *testing.T) {
	h := New()
	h.PublishAnomalies([]anomaly.Result{{StreamID: 9, Score: 20}})
	baseline := h.AnomalyNotifications("")
	if len(baseline.Items) != 0 {
		t.Fatal("initial history replayed")
	}
	h.PublishAnomalies([]anomaly.Result{{StreamID: 9, Score: 45}, {StreamID: 2, Score: 60}})
	next := h.AnomalyNotifications(baseline.Cursor)
	if len(next.Items) != 2 || next.Items[0].Score != 45 || next.Items[1].StreamID != 2 {
		t.Fatalf("updates missing: %+v", next)
	}
	if len(h.AnomalyNotifications(next.Cursor).Items) != 0 {
		t.Fatal("replayed events")
	}
	if !New().AnomalyNotifications(next.Cursor).Reset {
		t.Fatal("restart must reset cursor")
	}
	if !h.AnomalyNotifications("invalid").Reset {
		t.Fatal("invalid cursor must reset")
	}
	for i := 0; i < anomalyFeedSize+10; i++ {
		h.PublishAnomalies([]anomaly.Result{{StreamID: int64(i + 100)}})
	}
	tail := h.AnomalyNotifications(next.Cursor)
	if !tail.Gap || len(tail.Items) != anomalyFeedSize || tail.Items[0].StreamID != 110 {
		t.Fatalf("overflow: %+v", tail)
	}
	tail.Items[0].StreamID = 0
	if h.AnomalyNotifications(next.Cursor).Items[0].StreamID != 110 {
		t.Fatal("reader modified ring")
	}
}

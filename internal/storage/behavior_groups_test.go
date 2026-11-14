package storage

import (
	"context"
	"fmt"
	"testing"

	"gomate/internal/anomaly"
)

func BenchmarkBehaviorRetainedSessions(b *testing.B) {
	sessions := make([]behaviorSession, 20000)
	for i := range sessions {
		fields := []anomaly.BehaviorField{}
		for j := 0; j < 20; j++ {
			v := fmt.Sprint(j)
			if j%3 == 0 {
				v = fmt.Sprint(i)
			}
			fields = append(fields, anomaly.BehaviorField{Key: fmt.Sprint(j), Value: v, Preview: v})
		}
		sessions[i] = behaviorSession{id: int64(i), structure: "same", fields: fields}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, e := clusterBehavior(context.Background(), sessions, 0); e != nil {
			b.Fatal(e)
		}
	}
}

func TestBehaviorBurstTokensAndRepeatedPayload(t *testing.T) {
	sessions := []behaviorSession{}
	for i := 0; i < 1000; i++ {
		payload := fmt.Sprint(i)
		if i < 20 {
			payload = "exploit"
		}
		sessions = append(sessions, behaviorSession{id: int64(i), structure: "same", fields: []anomaly.BehaviorField{
			{Key: "nonce", Value: fmt.Sprint(i / 10)}, // reused ten times per burst
			{Key: "payload", Value: payload},
		}})
	}
	groups, e := clusterBehavior(context.Background(), sessions, 0)
	if e != nil || len(groups) != 2 {
		t.Fatalf("burst credentials fragmented repeated behavior: %d groups, %v", len(groups), e)
	}
	for _, g := range groups {
		if g.Count != 20 && g.Count != 980 {
			t.Fatal("repeated exploit lost", g.Count)
		}
	}
}

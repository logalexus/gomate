package storage

import (
	"context"
	"fmt"
	"math/rand"
	"reflect"
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

func TestBehaviorExamplesNewestDistinct(t *testing.T) {
	samples := []behaviorExample{{"a", 1, 1}, {"b", 2, 2}, {"c", 3, 3}, {"a", 5, 4}, {"d", 5, 5}, {"d", 4, 6}}
	want := []behaviorExample{{"d", 5, 5}, {"a", 5, 4}, {"c", 3, 3}}
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 100; i++ {
		rng.Shuffle(len(samples), func(i, j int) { samples[i], samples[j] = samples[j], samples[i] })
		var got []behaviorExample
		for _, sample := range samples {
			got = addBehaviorExample(got, sample)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("samples depend on input order: %#v", got)
		}
	}
}

func TestBehaviorExamplesRespectGroupPeriodAndRedaction(t *testing.T) {
	var sessions []behaviorSession
	for _, group := range []string{"a", "b"} {
		for i := 0; i < 16; i++ {
			sessions = append(sessions, behaviorSession{id: int64(len(sessions) + 1), start: int64(i), structure: "same", fields: []anomaly.BehaviorField{
				{Key: "1:http/path", Value: group, Preview: group},
				{Key: "1:http/json.nonce", Value: fmt.Sprintf("digest-%s-%d", group, i), Preview: fmt.Sprintf("%s-%d", group, i)},
				{Key: "1:http/cookie.session", Value: fmt.Sprintf("secret-%s-%d", group, i), Preview: "[значение скрыто]"},
				{Key: "1:http/json.empty", Value: fmt.Sprintf("empty-%s-%d", group, i), Preview: ""},
			}})
		}
	}
	for _, since := range []int64{0, 14} {
		groups, err := clusterBehavior(context.Background(), sessions, since)
		if err != nil || len(groups) != 2 {
			t.Fatalf("groups=%d, error=%v", len(groups), err)
		}
		for _, g := range groups {
			group := g.Fields[0][len("1:http/path = "):]
			want := []string{group + "-15", group + "-14"}
			if since == 0 {
				want = append(want, group+"-13")
			}
			if !reflect.DeepEqual(g.VariableExamples["1:http/json.nonce"], want) {
				t.Fatalf("wrong group/period examples: %#v", g.VariableExamples)
			}
			if !reflect.DeepEqual(g.VariableExamples["1:http/cookie.session"], []string{"[значение скрыто]"}) || !reflect.DeepEqual(g.VariableExamples["1:http/json.empty"], []string{""}) {
				t.Fatalf("saved previews changed: %#v", g.VariableExamples)
			}
			if g.Count != 16-since || g.Fingerprint != digest([]any{"same", []string{group, "*", "*", "*"}}) {
				t.Fatalf("examples changed grouping: %#v", g.TrafficGroup)
			}
		}
	}
}

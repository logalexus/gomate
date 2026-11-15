package api

import "testing"

func TestChatDisplayAuthorPreservesIdentity(t *testing.T) {
	for _, tc := range []struct{ stored, display string }{
		{"Аноним-1234abcd", "Anonymous-1234abcd"},
		{"Anonymous-1234abcd", "Anonymous-1234abcd"},
		{"Аноним-custom", "Аноним-custom"},
		{"Аноним-1234", "Аноним-1234"},
	} {
		if got := chatDisplayAuthor(tc.stored); got != tc.display {
			t.Errorf("chatDisplayAuthor(%q) = %q, want %q", tc.stored, got, tc.display)
		}
	}
}

package model

import (
	"encoding/json"
	"testing"
)

func TestPagePatternSelectionValidation(t *testing.T) {
	for _, body := range []string{`{"patternIds":[1,2,2],"pattern":{"id":"3"}}`, `{"patternIds":[1,2,3],"pattern":{"id":0}}`} {
		var page Page
		if err := json.Unmarshal([]byte(body), &page); err != nil {
			t.Fatal(err)
		}
		ids, err := page.SelectedPatternIDs()
		if err != nil || len(ids) != 3 || ids[0] != 1 || ids[1] != 2 || ids[2] != 3 {
			t.Fatal(ids, err)
		}
	}
	for _, body := range []string{`{"patternIds":[0]}`, `{"patternIds":[-1]}`, `{"patternIds":[2147483648]}`, `{"pattern":{"id":-1}}`, `{"pattern":{"id":1.5}}`, `{"pattern":{"id":"1 OR 1=1"}}`} {
		var page Page
		if err := json.Unmarshal([]byte(body), &page); err != nil {
			t.Fatal(err)
		}
		if ids, err := page.SelectedPatternIDs(); err == nil {
			t.Fatalf("accepted %s: %v", body, ids)
		}
	}
	var page Page
	ids, err := page.SelectedPatternIDs()
	if err != nil || ids == nil || len(ids) != 0 {
		t.Fatal("empty selection must be non-null SQL array", ids, err)
	}
	page.PatternIDs = make([]int, 101)
	for i := range page.PatternIDs {
		page.PatternIDs[i] = 1
	}
	if _, err = page.SelectedPatternIDs(); err == nil {
		t.Fatal("accepted oversized list")
	}
}

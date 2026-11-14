package storage

import (
	"strings"
	"unicode/utf8"
)

// Bound copied metadata before including it in the batched jsonb UPSERT.
// Stored paths survive payload cleanup and never require packet reads on list.
func boundedRequestField(value string, maxBytes int) (string, bool) {
	value = strings.ReplaceAll(strings.ToValidUTF8(value, "�"), "\x00", "\\0")
	if len(value) <= maxBytes {
		return value, false
	}
	value = value[:maxBytes]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value, true
}

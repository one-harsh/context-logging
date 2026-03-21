package loggingtest

import (
	"bytes"
	"encoding/json"
	"testing"
)

func EntriesFromBytes(t testing.TB, data []byte) []map[string]any {
	t.Helper()

	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	entries := make([]map[string]any, 0, len(lines))

	for _, line := range lines {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatalf("decode log line: %v", err)
		}
		entries = append(entries, entry)
	}

	return entries
}

func LastEntryFromBytes(t testing.TB, data []byte) map[string]any {
	t.Helper()

	entries := EntriesFromBytes(t, data)
	if len(entries) == 0 {
		t.Fatalf("expected at least one log entry")
	}
	return entries[len(entries)-1]
}

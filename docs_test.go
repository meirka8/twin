package main

import (
	"os"
	"strings"
	"testing"
)

// shortcutTableMarker precedes the table in DOCUMENTATION.md that lists the shortcuts of
// DefaultKeyMap, one row per shortcut: key, F-key alias (or an empty cell), action.
const shortcutTableMarker = "<!-- shortcuts: kept in sync with keys.go by docs_test.go -->"

// keyLabel spells a Bubble Tea key string the way the documentation does:
// "alt+c" becomes "Alt+C", "f10" becomes "F10".
func keyLabel(key string) string {
	parts := strings.Split(key, "+")
	for i, p := range parts {
		if p != "" {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "+")
}

func TestDocumentationListsEveryShortcut(t *testing.T) {
	doc, err := os.ReadFile("DOCUMENTATION.md")
	if err != nil {
		t.Fatal(err)
	}
	_, table, found := strings.Cut(string(doc), shortcutTableMarker)
	if !found {
		t.Fatalf("DOCUMENTATION.md has no %q before its shortcut table", shortcutTableMarker)
	}

	documented := make(map[string]string) // Key -> F-key alias
	for _, line := range strings.Split(strings.TrimLeft(table, "\n"), "\n") {
		if !strings.HasPrefix(line, "|") {
			break // End of the table
		}
		cells := strings.Split(line, "|")
		if len(cells) < 4 {
			t.Fatalf("shortcut table row has fewer than three cells: %s", line)
		}
		key := strings.Trim(strings.TrimSpace(cells[1]), "`")
		alias := strings.Trim(strings.TrimSpace(cells[2]), "`")
		if key == "Key" || strings.HasPrefix(key, "-") {
			continue // Header and separator
		}
		if _, dup := documented[key]; dup {
			t.Errorf("%s appears twice in the shortcut table in DOCUMENTATION.md", key)
		}
		documented[key] = alias
	}

	want := make(map[string]string)
	for _, s := range DefaultKeyMap().GetShortcuts() {
		alias := ""
		if s.FKey != "" {
			alias = keyLabel(s.FKey)
		}
		want[keyLabel(s.Key)] = alias
	}

	for key, alias := range want {
		got, ok := documented[key]
		switch {
		case !ok:
			t.Errorf("%s (keys.go) is missing from the shortcut table in DOCUMENTATION.md", key)
		case got != alias:
			t.Errorf("%s: DOCUMENTATION.md gives the F-key %q, keys.go %q", key, got, alias)
		}
	}
	for key := range documented {
		if _, ok := want[key]; !ok {
			t.Errorf("%s is in the shortcut table in DOCUMENTATION.md but not in keys.go", key)
		}
	}
}

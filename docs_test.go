package main

import (
	"flag"
	"os"
	"strings"
	"testing"
)

var updateDocs = flag.Bool("update", false, "rewrite the generated shortcut table in DOCUMENTATION.md")

// The shortcut table in DOCUMENTATION.md is generated from defaultActions and sits between
// these markers. TestDocumentationShortcutTable fails when it is out of date, and rewrites
// it when run with -update.
const (
	shortcutsStart = "<!-- shortcuts: generated from actions.go by `go test -run TestDocumentationShortcutTable -update` -->\n"
	shortcutsEnd   = "<!-- end of shortcuts -->\n"
)

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

// isFKey reports whether key is a function key: "f1" to "f63".
func isFKey(key string) bool {
	return len(key) > 1 && key[0] == 'f' && strings.Trim(key[1:], "0123456789") == ""
}

// shortcutTable renders actions as a Markdown table: the keys of each, its F-keys and what
// it does.
func shortcutTable(actions []action) string {
	var s strings.Builder
	row := func(cells ...string) {
		s.WriteString("|")
		for _, c := range cells {
			if c != "" {
				s.WriteString(" " + c)
			}
			s.WriteString(" |")
		}
		s.WriteString("\n")
	}
	row("Key", "F-key", "Action")
	row("---", "---", "---")
	for _, a := range actions {
		var keys, fKeys []string
		for _, k := range a.keys {
			label := "`" + keyLabel(k) + "`"
			if isFKey(k) {
				fKeys = append(fKeys, label)
			} else {
				keys = append(keys, label)
			}
		}
		row(strings.Join(keys, " "), strings.Join(fKeys, " "), a.help)
	}
	return s.String()
}

func TestDocumentationShortcutTable(t *testing.T) {
	doc, err := os.ReadFile("DOCUMENTATION.md")
	if err != nil {
		t.Fatal(err)
	}
	before, rest, foundStart := strings.Cut(string(doc), shortcutsStart)
	table, after, foundEnd := strings.Cut(rest, shortcutsEnd)
	if !foundStart || !foundEnd {
		t.Fatalf("DOCUMENTATION.md lacks the lines %q and %q around its shortcut table", shortcutsStart, shortcutsEnd)
	}

	want := shortcutTable(defaultActions())
	switch {
	case table == want:
	case *updateDocs:
		if err := os.WriteFile("DOCUMENTATION.md", []byte(before+shortcutsStart+want+shortcutsEnd+after), 0o644); err != nil {
			t.Fatal(err)
		}
	default:
		t.Errorf("the shortcut table in DOCUMENTATION.md is out of date with actions.go; "+
			"go test -run %s -update rewrites it. It should be:\n%s", t.Name(), want)
	}
}

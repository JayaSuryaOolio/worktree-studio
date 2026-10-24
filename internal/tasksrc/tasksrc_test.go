package tasksrc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeAdapter(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "adapter")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestListAndGetPassArgsAndParse(t *testing.T) {
	// Echoes its args back as the title, so the test sees what was passed.
	cmd := writeAdapter(t, `printf '[{"key":"A-1","title":"%s","url":"u"},{"title":"no key"}]' "$*"`)

	items, err := List(cmd, "foo bar")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Key != "A-1" || items[0].Title != "list --query foo bar" {
		t.Fatalf("List = %+v", items)
	}
	items, err = Get(cmd, []string{"A-1", "B-2"})
	if err != nil || items[0].Title != "get A-1 B-2" {
		t.Fatalf("Get = %+v, %v", items, err)
	}
}

func TestErrorsCarryStderr(t *testing.T) {
	cmd := writeAdapter(t, "echo 'not logged in' >&2; exit 1")
	if _, err := List(cmd, ""); err == nil || !strings.Contains(err.Error(), "not logged in") {
		t.Fatalf("err = %v", err)
	}
	cmd = writeAdapter(t, "echo nope")
	if _, err := List(cmd, ""); err == nil {
		t.Fatal("want error for non-JSON output")
	}
}

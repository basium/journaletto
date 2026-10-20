package entries_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"journaletto/internal/entries"
	"journaletto/internal/store"
)

func newSvc(t *testing.T) (*entries.Service, string, func() int) {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	count := func() int {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM entries`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	return &entries.Service{DB: db, DataRoot: dir, UserID: "u"}, dir, count
}

func mdFiles(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	filepath.Walk(filepath.Join(root, "journals"), func(p string, i os.FileInfo, err error) error {
		if err == nil && !i.IsDir() {
			out = append(out, p)
		}
		return nil
	})
	return out
}

func tags(n int, f func(int) string) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = f(i)
	}
	return out
}

func TestSaveValidationLimits(t *testing.T) {
	svc, root, count := newSvc(t)
	svc.MaxBody = 1000
	cases := []struct {
		name  string
		in    entries.Input
		field string // "" means must succeed
	}{
		{"title empty", entries.Input{Title: "  "}, "title"},
		{"title 1", entries.Input{Title: "a"}, ""},
		{"title 200", entries.Input{Title: strings.Repeat("é", 200)}, ""},
		{"title 201", entries.Input{Title: strings.Repeat("a", 201)}, "title"},
		{"title newline", entries.Input{Title: "a\n---\nid: x"}, "title"},
		{"title CR", entries.Input{Title: "a\rb"}, "title"},
		{"body at limit", entries.Input{Title: "t", Description: strings.Repeat("a", 1000)}, ""},
		{"body over limit", entries.Input{Title: "t", Description: strings.Repeat("a", 1001)}, "description"},
		{"100 tags", entries.Input{Title: "t", Tags: tags(100, func(i int) string { return fmt.Sprint("t", i) })}, ""},
		{"101 tags", entries.Input{Title: "t", Tags: tags(101, func(i int) string { return fmt.Sprint("t", i) })}, "tags"},
		{"101 duplicate tags counted raw", entries.Input{Title: "t", Tags: tags(101, func(int) string { return "same" })}, "tags"},
		{"tag 50", entries.Input{Title: "t", Tags: []string{strings.Repeat("a", 50)}}, ""},
		{"tag 51", entries.Input{Title: "t", Tags: []string{strings.Repeat("a", 51)}}, "tags"},
		{"tag 51 duplicate still checked", entries.Input{Title: "t", Tags: []string{"a", strings.Repeat("A", 51)}}, "tags"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			before := count()
			_, err := svc.Save(context.Background(), c.in)
			if c.field == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var ve *entries.ValidationError
			if !errors.As(err, &ve) || ve.Field != c.field {
				t.Fatalf("want validation error on %s, got %v", c.field, err)
			}
			if count() != before {
				t.Fatal("rejected input created a row")
			}
		})
	}
	_ = root
}

func TestMediaEmbedsRejectedOnSaveAndEditButLinksAndCodeAllowed(t *testing.T) {
	svc, root, count := newSvc(t)
	ctx := context.Background()
	rejected := map[string]string{
		"markdown image":    "see ![alt](http://x/y.png)",
		"empty alt":         "![](a.png)",
		"html img":          `text <IMG src="a.png">`,
		"reference image":   "![a][ref]\n\n[ref]: http://x/a.png",
		"collapsed ref":     "![a][]\n\n[a]: http://x/a.png",
		"shortcut ref":      "![a]\n\n[a]: http://x/a.png",
		"after closed code": "```\nok\n```\n![a](b.png)",
		"code span closed":  "`x` ![a](b.png)",
	}
	allowed := map[string]string{
		"plain link":     "read [docs](http://x/docs)",
		"linked bang":    "wow! [docs](http://x)",
		"fenced image":   "```\n![a](b.png) <img src=x>\n```\n",
		"tilde fence":    "~~~md\n![a](b.png)\n~~~",
		"inline code":    "use `![a](b.png)` syntax",
		"bracket no def": "![note] just text",
		"lone html tag":  "<imgur> is a site",
	}
	saved, err := svc.Save(ctx, entries.Input{Title: "keep", Description: "orig"})
	if err != nil {
		t.Fatal(err)
	}
	for name, d := range rejected {
		var ve *entries.ValidationError
		files, rows := len(mdFiles(t, root)), count()
		if _, err := svc.Save(ctx, entries.Input{Title: "t", Description: d}); !errors.As(err, &ve) || ve.Field != "description" || ve.Message == "" {
			t.Errorf("%s: save err = %v", name, err)
		}
		if _, err := svc.Update(ctx, saved.ID, entries.Input{Title: "t", Description: d}); !errors.As(err, &ve) || ve.Field != "description" {
			t.Errorf("%s: update err = %v", name, err)
		}
		if len(mdFiles(t, root)) != files || count() != rows {
			t.Errorf("%s: rejected input wrote a file or row", name)
		}
	}
	for name, d := range allowed {
		if _, err := svc.Save(ctx, entries.Input{Title: "t", Description: d}); err != nil {
			t.Errorf("%s: save: %v", name, err)
		}
		if _, err := svc.Update(ctx, saved.ID, entries.Input{Title: "t", Description: d}); err != nil {
			t.Errorf("%s: update: %v", name, err)
		}
	}
}

func TestSaveDedupesAndLowercasesTags(t *testing.T) {
	svc, _, _ := newSvc(t)
	if _, err := svc.Save(context.Background(), entries.Input{Title: "t", Tags: []string{"Go", "go", " GO "}}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.SuggestTags(context.Background(), "")
	if err != nil || len(got) != 1 || got[0] != "go" {
		t.Fatalf("tags = %v, %v", got, err)
	}
}

func TestSaveLeavesNoOrphanFileWhenDBFails(t *testing.T) {
	svc, root, count := newSvc(t)
	if _, err := svc.DB.Exec(`DROP TABLE entries_fts`); err != nil { // force the insert to fail
		t.Fatal(err)
	}
	if _, err := svc.Save(context.Background(), entries.Input{Title: "t"}); err == nil {
		t.Fatal("expected error")
	}
	if f := mdFiles(t, root); len(f) != 0 {
		t.Fatalf("orphan files: %v", f)
	}
	if count() != 0 {
		t.Fatal("row left behind")
	}
}

func TestSaveRenameFailureLeavesNoRowAndNoFile(t *testing.T) {
	svc, root, count := newSvc(t)
	svc.Rename = func(string, string) error { return errors.New("disk full") }
	if _, err := svc.Save(context.Background(), entries.Input{Title: "t"}); err == nil {
		t.Fatal("expected error")
	}
	if count() != 0 || len(mdFiles(t, root)) != 0 {
		t.Fatalf("rows=%d files=%v", count(), mdFiles(t, root))
	}
}

func TestSaveCommitFailureRemovesPublishedFile(t *testing.T) {
	svc, root, count := newSvc(t)
	svc.Commit = func(*sql.Tx) error { return errors.New("commit failed") } // tx is rolled back by Save
	if _, err := svc.Save(context.Background(), entries.Input{Title: "t"}); err == nil {
		t.Fatal("expected error")
	}
	if count() != 0 || len(mdFiles(t, root)) != 0 {
		t.Fatalf("rows=%d files=%v", count(), mdFiles(t, root))
	}
}

func TestSaveWritesExactlyOneRowAndFile(t *testing.T) {
	svc, root, count := newSvc(t)
	e, err := svc.Save(context.Background(), entries.Input{Title: "ok"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(e.Path); err != nil {
		t.Fatal(err)
	}
	if count() != 1 || len(mdFiles(t, root)) != 1 {
		t.Fatal("expected exactly one row and one file")
	}
}

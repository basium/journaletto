package entries_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"journaletto/internal/entries"
)

func TestListFiltersDeletedScopesUserAndPaginatesNewestFirst(t *testing.T) {
	svc, _, _ := newSvc(t)
	ctx := context.Background()
	clock := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)
	svc.Now = func() time.Time { return clock }
	var ids []string
	for i := 0; i < 23; i++ {
		clock = clock.Add(time.Minute)
		e, err := svc.Save(ctx, entries.Input{Title: fmt.Sprintf("E%02d", i)})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, e.ID)
	}
	if _, err := svc.DB.Exec(`INSERT INTO entries(id,user_id,title,body,journal_date,created_at,updated_at,timezone,file_path)
		VALUES ('other','someone-else','X','b','2026-03-04','2027-01-01T00:00:00Z','2027-01-01T00:00:00Z','UTC','/x')`); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, ids[22]); err != nil { // newest
		t.Fatal(err)
	}

	p1, err := svc.List(ctx, false, 1)
	if err != nil {
		t.Fatal(err)
	}
	if p1.Total != 22 || len(p1.Items) != 20 || !p1.HasNext || p1.HasPrev || p1.Items[0].Title != "E21" {
		t.Fatalf("page 1 = total %d len %d first %q", p1.Total, len(p1.Items), p1.Items[0].Title)
	}
	p2, _ := svc.List(ctx, false, 2)
	if len(p2.Items) != 2 || p2.HasNext || !p2.HasPrev || p2.Items[1].Title != "E00" {
		t.Fatalf("page 2 = %+v", p2)
	}
	all, _ := svc.List(ctx, true, 1)
	if all.Total != 23 || all.Items[0].Title != "E22" || !all.Items[0].Deleted || all.Items[0].Status != "deleted" {
		t.Fatalf("include deleted = total %d first %+v", all.Total, all.Items[0])
	}
}

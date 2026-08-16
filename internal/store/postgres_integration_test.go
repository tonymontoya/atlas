package store

import (
	"context"
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/cases"
	"github.com/tonymontoya/ceph-atlas/internal/testdb"
)

func TestPostgresStoreListsAndGetsSeedCases(t *testing.T) {
	db, _ := testdb.Open(t)
	ctx := context.Background()

	store := NewPostgres(db)
	listed, err := store.ListCases(ctx, 50)
	if err != nil {
		t.Fatalf("list cases: %v", err)
	}
	if len(listed) < 2 {
		t.Fatalf("case count = %d, want at least 2", len(listed))
	}
	if listed[0].Title != "Review weekly capacity trend" {
		t.Fatalf("first case title = %q", listed[0].Title)
	}

	item, err := store.GetCase(ctx, listed[0].ID)
	if err != nil {
		t.Fatalf("get case: %v", err)
	}
	if item.ID != listed[0].ID || item.Status != cases.CaseStatusTriaged {
		t.Fatalf("case = %+v, want listed triaged case %+v", item, listed[0])
	}
}

func TestPostgresStoreListsSeedCaseTimeline(t *testing.T) {
	db, _ := testdb.Open(t)
	ctx := context.Background()

	store := NewPostgres(db)
	listed, err := store.ListCases(ctx, 50)
	if err != nil {
		t.Fatalf("list cases: %v", err)
	}
	if len(listed) == 0 {
		t.Fatal("expected seeded cases")
	}

	timeline, err := store.ListCaseTimeline(ctx, listed[0].ID)
	if err != nil {
		t.Fatalf("list case timeline: %v", err)
	}
	if len(timeline) != 2 {
		t.Fatalf("timeline event count = %d, want 2; timeline=%+v", len(timeline), timeline)
	}
	if timeline[0].CaseID != listed[0].ID || timeline[0].Type != cases.TimelineEventCaseDetected {
		t.Fatalf("first timeline event = %+v, want case_detected for case %d", timeline[0], listed[0].ID)
	}
	if timeline[1].Type != cases.TimelineEventCaseTriaged {
		t.Fatalf("second timeline event = %+v, want case_triaged", timeline[1])
	}
	if timeline[0].Actor.Type != cases.TimelineActorUser || timeline[0].Actor.DisplayName == "" {
		t.Fatalf("actor = %+v, want displayable user actor", timeline[0].Actor)
	}
	if timeline[0].Payload["source"] == "" {
		t.Fatalf("payload = %+v, want normalized source context", timeline[0].Payload)
	}
	if !timeline[0].OccurredAt.Before(timeline[1].OccurredAt) {
		t.Fatalf("timeline not oldest-first: %+v", timeline)
	}
}

func TestPostgresStoreReturnsNotFoundForMissingCase(t *testing.T) {
	db, _ := testdb.Open(t)
	ctx := context.Background()

	_, err := NewPostgres(db).GetCase(ctx, 999999)
	if err == nil {
		t.Fatal("expected missing case error")
	}
}

func TestPostgresStoreReturnsNotFoundForMissingCaseTimeline(t *testing.T) {
	db, _ := testdb.Open(t)
	ctx := context.Background()

	_, err := NewPostgres(db).ListCaseTimeline(ctx, 999999)
	if err == nil {
		t.Fatal("expected missing case timeline error")
	}
}

package api

import (
	"context"
	"database/sql"
	"testing"

	"github.com/lugvitc/whats4linux/internal/misc"
	"github.com/lugvitc/whats4linux/internal/query"
	"github.com/lugvitc/whats4linux/internal/wa"
	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/whatsmeow"
)

func TestGetCommunityListReturnsPopulatedCache(t *testing.T) {
	originalConfigDir := misc.ConfigDir
	misc.ConfigDir = t.TempDir()
	t.Cleanup(func() { misc.ConfigDir = originalConfigDir })

	db, err := sql.Open("sqlite3", misc.GetSQLiteAddress("app.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(query.CreateGroupsTable); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		query.InsertOrReplaceGroup,
		"community@g.us", "Test community", "Topic", "", 0, "", "", 1, 0,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		query.InsertOrReplaceGroup,
		"group@g.us", "Test group", "", "", 3,
		"community@g.us", "Test community", 0, 0,
	); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	appDB, err := wa.NewAppDatabase(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = appDB.Close() })

	// A zero-value client cannot perform a live request. A successful result
	// therefore verifies that a populated cache is returned without refreshing.
	a := &Api{cw: appDB, waClient: &whatsmeow.Client{}}
	communities, err := a.GetCommunityList()
	if err != nil {
		t.Fatal(err)
	}
	if len(communities) != 1 {
		t.Fatalf("got %d communities, want 1", len(communities))
	}
	if communities[0].JID != "community@g.us" || communities[0].GroupCount != 1 {
		t.Fatalf("unexpected community: %+v", communities[0])
	}
}

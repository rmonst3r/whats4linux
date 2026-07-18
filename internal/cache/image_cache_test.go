package cache

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/lugvitc/whats4linux/internal/query"
	_ "github.com/mattn/go-sqlite3"
)

func TestImageCacheEvictsOldFilesToSizeLimit(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite3", filepath.Join(dir, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(query.CreateImageIndexTable); err != nil {
		t.Fatal(err)
	}
	getStmt, err := db.Prepare(query.GetImageByID)
	if err != nil {
		t.Fatal(err)
	}
	saveStmt, err := db.Prepare(query.SaveImageIndex)
	if err != nil {
		t.Fatal(err)
	}
	ic := &ImageCache{db: db, imagesDir: dir, getStmt: getStmt, saveStmt: saveStmt}
	t.Cleanup(func() { _ = ic.Close() })

	hash, err := ic.SaveImage("message", []byte("cached image"), "image/png", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, hash+".png")
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	if err := ic.evictToSize(0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("cached file still exists after eviction: %v", err)
	}
	meta, err := ic.GetImageByMessageID("message")
	if err != nil {
		t.Fatal(err)
	}
	if meta != nil {
		t.Fatal("cache index still contains evicted image")
	}
}

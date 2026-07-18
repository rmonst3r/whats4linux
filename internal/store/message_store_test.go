package store

import (
	"database/sql"
	"testing"
	"time"

	"github.com/lugvitc/whats4linux/internal/misc"
	"github.com/lugvitc/whats4linux/internal/query"
)

func newTestMessageStore(t *testing.T) *MessageStore {
	t.Helper()
	oldConfigDir := misc.ConfigDir
	misc.ConfigDir = t.TempDir()
	t.Cleanup(func() { misc.ConfigDir = oldConfigDir })

	ms, err := NewMessageStore()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ms.Close() })
	return ms
}

func insertTestMessage(t *testing.T, ms *MessageStore, id, chat string, timestamp int64, replyTo string) {
	t.Helper()
	err := ms.runSync(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			query.InsertMessage,
			id,
			chat,
			"sender@s.whatsapp.net",
			timestamp,
			false,
			id,
			false,
			replyTo,
			false,
			false,
			0,
		)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func messageIDs(messages []DecodedMessage) []string {
	ids := make([]string, len(messages))
	for i := range messages {
		ids[i] = messages[i].Info.ID
	}
	return ids
}

func TestMessagePaginationUsesStableCompoundCursor(t *testing.T) {
	ms := newTestMessageStore(t)
	const (
		chat      = "123@s.whatsapp.net"
		timestamp = int64(1_700_000_000)
	)
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		insertTestMessage(t, ms, id, chat, timestamp, "")
	}

	first, err := ms.GetDecodedMessagesPaged(chat, 0, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ms.GetDecodedMessagesPaged(chat, timestamp, first[0].Info.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	third, err := ms.GetDecodedMessagesPaged(chat, timestamp, second[0].Info.ID, 2)
	if err != nil {
		t.Fatal(err)
	}

	got := append(append(messageIDs(third), messageIDs(second)...), messageIDs(first)...)
	want := []string{"a", "b", "c", "d", "e"}
	if len(got) != len(want) {
		t.Fatalf("got IDs %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got IDs %v, want %v", got, want)
		}
	}
}

func TestPagedQuotedMessagesAreShallow(t *testing.T) {
	ms := newTestMessageStore(t)
	const chat = "123@s.whatsapp.net"
	insertTestMessage(t, ms, "a", chat, 1, "b")
	insertTestMessage(t, ms, "b", chat, 2, "a")

	messages, err := ms.GetDecodedMessagesPaged(chat, 0, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 {
		t.Fatalf("got %d messages, want 2", len(messages))
	}
	for _, message := range messages {
		contextInfo := message.Content.ExtendedTextMessage.ContextInfo
		if contextInfo == nil || contextInfo.QuotedMessage == nil {
			t.Fatalf("message %s has no quoted summary", message.Info.ID)
		}
		if contextInfo.QuotedMessage.ExtendedTextMessage != nil {
			t.Fatalf("message %s recursively decoded its quoted message", message.Info.ID)
		}
	}
}

func TestDecodedMessagesIncludeLinkPreviewMetadata(t *testing.T) {
	ms := newTestMessageStore(t)
	const (
		chat = "123@s.whatsapp.net"
		id   = "preview-message"
	)
	insertTestMessage(t, ms, id, chat, 1, "")
	err := ms.runSync(func(tx *sql.Tx) error {
		_, err := tx.Exec(
			query.InsertLinkPreview,
			id,
			"https://example.com/article",
			"Example title",
			"Example description",
			[]byte{1, 2, 3},
			"",
			nil,
			nil,
			nil,
		)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	page, err := ms.GetDecodedMessagesPaged(chat, 0, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 1 || page[0].LinkPreview == nil {
		t.Fatalf("paged message has no link preview: %#v", page)
	}
	preview := page[0].LinkPreview
	if preview.URL != "https://example.com/article" || preview.Title != "Example title" || !preview.HasPoster {
		t.Fatalf("unexpected paged preview: %#v", preview)
	}

	message, err := ms.GetDecodedMessage(chat, id)
	if err != nil {
		t.Fatal(err)
	}
	if message.LinkPreview == nil || !message.LinkPreview.HasPoster {
		t.Fatalf("single decoded message has no poster metadata: %#v", message.LinkPreview)
	}
}

func TestRunSyncReturnsCommitFailure(t *testing.T) {
	ms := newTestMessageStore(t)
	err := ms.runSync(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`PRAGMA defer_foreign_keys=ON`); err != nil {
			return err
		}
		_, err := tx.Exec(
			`INSERT INTO reactions (message_id, sender_id, emoji) VALUES (?, ?, ?)`,
			"missing-message",
			"sender",
			"👍",
		)
		return err
	})
	if err == nil {
		t.Fatal("runSync returned success for a transaction that failed at commit")
	}
}

func TestRunSyncReturnsBeginFailureWithoutBlocking(t *testing.T) {
	ms := newTestMessageStore(t)
	if err := ms.db.Close(); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		done <- ms.runSync(func(*sql.Tx) error { return nil })
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("runSync returned success after the database was closed")
		}
	case <-time.After(time.Second):
		t.Fatal("runSync blocked after transaction begin failed")
	}
}

package store

import (
	"database/sql"
	"time"

	"github.com/lugvitc/whats4linux/internal/query"
)

// SetChatMuted persists the mute state for a chat. mutedUntil is unix seconds:
// -1 mutes forever, a future timestamp mutes until then, and 0 removes the
// mute entirely (the row is deleted).
func (ms *MessageStore) SetChatMuted(chatJID string, mutedUntil int64) error {
	return ms.runSync(func(tx *sql.Tx) error {
		if mutedUntil == 0 {
			_, err := tx.Exec(query.DeleteMutedChatByChatJID, chatJID)
			return err
		}
		_, err := tx.Exec(query.UpsertMutedChat, chatJID, mutedUntil)
		return err
	})
}

// IsChatMuted reports whether a chat is currently muted: a row must exist and
// be either muted forever (-1) or have an end timestamp in the future. Expired
// rows count as not muted.
func (ms *MessageStore) IsChatMuted(chatJID string) bool {
	var mutedUntil int64
	if err := ms.db.QueryRow(query.SelectMutedUntilByChatJID, chatJID).Scan(&mutedUntil); err != nil {
		// sql.ErrNoRows (not muted) or a query failure; either way don't
		// suppress notifications on error.
		return false
	}
	return mutedUntil == -1 || mutedUntil > time.Now().Unix()
}

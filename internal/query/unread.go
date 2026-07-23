package query

const (
	// chat_read_state holds one row per chat describing how far the user has
	// read. read_timestamp is a Unix-second watermark: every incoming message
	// newer than it is unread. marked_unread is WhatsApp's explicit "mark as
	// unread" flag (a chat flagged unread despite having no unread messages).
	//
	// Unread is *derived* from this watermark against the messages table rather
	// than stored as a counter, so it's always consistent on restart and can't
	// drift — the same model the official multi-device clients use.
	CreateChatReadStateTable = `
	CREATE TABLE IF NOT EXISTS chat_read_state (
		chat_jid TEXT PRIMARY KEY,
		read_timestamp INTEGER NOT NULL DEFAULT 0,
		marked_unread INTEGER NOT NULL DEFAULT 0
	);
	`

	// app_meta is a tiny key/value table for one-shot flags (currently only the
	// marker left by an abandoned read-state seeding approach, checked during
	// startup cleanup).
	CreateAppMetaTable = `
	CREATE TABLE IF NOT EXISTS app_meta (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);
	`

	// SetReadWatermarkMax moves the watermark forward only (a read never
	// un-reads older messages) and clears any mark-as-unread flag. Used for
	// live reads and incremental app-state deltas.
	SetReadWatermarkMax = `
	INSERT INTO chat_read_state (chat_jid, read_timestamp, marked_unread)
	VALUES (?, ?, 0)
	ON CONFLICT(chat_jid) DO UPDATE SET
		read_timestamp = MAX(chat_read_state.read_timestamp, excluded.read_timestamp),
		marked_unread = 0
	`

	// SetMarkedUnread flags a chat unread without touching its watermark.
	SetMarkedUnread = `
	INSERT INTO chat_read_state (chat_jid, read_timestamp, marked_unread)
	VALUES (?, 0, 1)
	ON CONFLICT(chat_jid) DO UPDATE SET marked_unread = 1
	`

	// SeedWatermarkForNewMessage creates a watermark row for a chat that has
	// none yet, placed at the newest incoming message *older* than the message
	// that just arrived. That makes only the new message (and anything after it)
	// count as unread, while everything already in history is treated as read —
	// so a chat we were never tracking doesn't retroactively flood with unread.
	// INSERT OR IGNORE leaves an existing watermark untouched.
	SeedWatermarkForNewMessage = `
	INSERT OR IGNORE INTO chat_read_state (chat_jid, read_timestamp)
	SELECT ?, COALESCE(MAX(timestamp), 0) FROM messages
	WHERE chat_jid = ? AND is_from_me = 0 AND timestamp < ?
	`

	// SelectUnreadCountForChat counts incoming messages newer than the chat's
	// watermark. An INNER JOIN means a chat with no watermark row counts zero —
	// i.e. "fully read" until something starts tracking it.
	SelectUnreadCountForChat = `
	SELECT COUNT(*) FROM messages m
	JOIN chat_read_state r ON r.chat_jid = m.chat_jid
	WHERE m.chat_jid = ? AND m.is_from_me = 0
	  AND m.timestamp > r.read_timestamp
	`

	SelectMarkedUnreadForChat = `SELECT marked_unread FROM chat_read_state WHERE chat_jid = ?`

	// SelectAllUnreadCounts computes unread for every tracked chat in one pass.
	SelectAllUnreadCounts = `
	SELECT m.chat_jid, COUNT(*)
	FROM messages m
	JOIN chat_read_state r ON r.chat_jid = m.chat_jid
	WHERE m.is_from_me = 0
	  AND m.timestamp > r.read_timestamp
	GROUP BY m.chat_jid
	`

	// SelectMarkedUnreadChats lists chats explicitly flagged unread.
	SelectMarkedUnreadChats = `SELECT chat_jid FROM chat_read_state WHERE marked_unread = 1`

	// SelectNewestIncomingTS / SelectNthNewestIncomingTS support seeding the
	// watermark from the server's link-time unread count: the newest incoming
	// message's timestamp ("all read"), and the timestamp of the N-th newest
	// incoming message (everything from it onward should count unread).
	SelectNewestIncomingTS = `
	SELECT MAX(timestamp) FROM messages WHERE chat_jid = ? AND is_from_me = 0
	`
	SelectNthNewestIncomingTS = `
	SELECT timestamp FROM messages WHERE chat_jid = ? AND is_from_me = 0
	ORDER BY timestamp DESC LIMIT 1 OFFSET ?
	`

	// ReplaceChatReadState overwrites a chat's read state unconditionally.
	// Only used for a positive link-time signal (unreadCount > 0 or the
	// marked-unread flag), which is authoritative for that chat.
	ReplaceChatReadState = `
	INSERT OR REPLACE INTO chat_read_state (chat_jid, read_timestamp, marked_unread)
	VALUES (?, ?, ?)
	`

	// SeedChatReadStateIfMissing fills a row only when none exists. Used for
	// the ambiguous unreadCount=0 case: later history-sync batches re-deliver
	// conversations without the unread field, and overwriting would wipe a
	// correct positive seed from an earlier batch.
	SeedChatReadStateIfMissing = `
	INSERT OR IGNORE INTO chat_read_state (chat_jid, read_timestamp, marked_unread)
	VALUES (?, ?, 0)
	`
)

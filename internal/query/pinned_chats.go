package query

const (
	CreatePinnedChatsTable = `
	CREATE TABLE IF NOT EXISTS pinned_chats (
		chat_jid TEXT PRIMARY KEY,
		pinned_at INTEGER NOT NULL
	);
	`
	UpsertPinnedChat = `
	INSERT OR REPLACE INTO pinned_chats (chat_jid, pinned_at)
	VALUES (?, ?);
	`
	DeletePinnedChat = `
	DELETE FROM pinned_chats WHERE chat_jid = ?;
	`
	SelectPinnedChats = `
	SELECT chat_jid, pinned_at FROM pinned_chats;
	`
	// Pinned messages joined with their text for the chat banner. Expired pins
	// (expiry seconds after pinned_at) are filtered out.
	GetChatPinnedMessagesWithText = `
	SELECT p.message_id, p.sender_jid, p.pinned_at, COALESCE(m.text, '')
	FROM pinned_messages p
	LEFT JOIN messages m ON m.message_id = p.message_id AND m.chat_jid = p.chat_jid
	WHERE p.chat_jid = ? AND (p.expiry <= 0 OR p.pinned_at + p.expiry > ?)
	ORDER BY p.pinned_at ASC;
	`
)

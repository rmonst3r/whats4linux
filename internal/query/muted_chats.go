package query

const (
	// muted_until is unix seconds; -1 means muted forever. A row whose
	// muted_until is in the past counts as NOT muted.
	CreateMutedChatsTable = `
	CREATE TABLE IF NOT EXISTS muted_chats (
		chat_jid TEXT PRIMARY KEY,
		muted_until INTEGER NOT NULL
	);
	`

	UpsertMutedChat = `
	INSERT OR REPLACE INTO muted_chats
	(chat_jid, muted_until)
	VALUES (?, ?)
	`

	SelectMutedUntilByChatJID = `
	SELECT muted_until
	FROM muted_chats
	WHERE chat_jid = ?
	`

	DeleteMutedChatByChatJID = `
	DELETE FROM muted_chats
	WHERE chat_jid = ?
	`
)

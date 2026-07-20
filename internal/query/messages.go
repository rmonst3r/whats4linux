package query

const (

	// Messages database queries (messages.db)
	CreateMessagesTable = `
	CREATE TABLE IF NOT EXISTS messages (
		message_id TEXT PRIMARY KEY,
		chat_jid TEXT NOT NULL,
		sender_jid TEXT NOT NULL,
		timestamp INTEGER NOT NULL,
		is_from_me BOOLEAN NOT NULL,
		text TEXT,
		has_media BOOLEAN DEFAULT FALSE,
		reply_to_message_id TEXT,
		edited BOOLEAN DEFAULT FALSE,
		forwarded BOOLEAN DEFAULT FALSE,
		type INTEGER DEFAULT 0,
		status INTEGER DEFAULT 0
	);
		CREATE INDEX IF NOT EXISTS idx_messages_chat_jid ON messages(chat_jid);
		CREATE INDEX IF NOT EXISTS idx_messages_sender_jid ON messages(sender_jid);
		CREATE INDEX IF NOT EXISTS idx_messages_timestamp ON messages(timestamp DESC);
		CREATE INDEX IF NOT EXISTS idx_messages_chat_cursor ON messages(chat_jid, timestamp DESC, message_id DESC);
	`

	// AddStatusColumn migrates pre-existing messages tables to carry the
	// outgoing read-receipt status (0=none/sent, 2=delivered, 3=read/played).
	AddStatusColumn = `
	ALTER TABLE messages ADD COLUMN status INTEGER DEFAULT 0;
	`

	// UpdateMessageStatus advances a sent message's receipt status. The
	// status < ? guard keeps it monotonic so a late "delivered" can never
	// downgrade a message already marked "read".
	UpdateMessageStatus = `
	UPDATE messages
	SET status = ?
	WHERE message_id = ? AND is_from_me = TRUE AND status < ?
	`

	SelectMessageStatusByID = `
	SELECT status FROM messages WHERE message_id = ? LIMIT 1
	`

	// Per-participant receipt state for our outgoing messages. In a group each
	// recipient acknowledges independently; the message's displayed tick is the
	// lowest level reached by *all* recipients, matching the official client
	// (blue only once everyone has read). One row per (message, participant).
	CreateMessageReceiptsTable = `
	CREATE TABLE IF NOT EXISTS message_receipts (
		message_id TEXT NOT NULL,
		participant TEXT NOT NULL,
		status INTEGER NOT NULL,
		PRIMARY KEY (message_id, participant)
	);
	CREATE INDEX IF NOT EXISTS idx_message_receipts_message_id ON message_receipts(message_id);
	`

	// UpsertMessageReceipt records a participant's receipt, only ever advancing
	// it (the conflict guard keeps a stale "delivered" from clobbering "read").
	UpsertMessageReceipt = `
	INSERT INTO message_receipts (message_id, participant, status)
	VALUES (?, ?, ?)
	ON CONFLICT(message_id, participant) DO UPDATE SET status = excluded.status
	WHERE excluded.status > message_receipts.status
	`

	// CountMessageReceiptsAtLeast counts how many distinct participants have
	// reached (at least) a given status for a message.
	CountMessageReceiptsAtLeast = `
	SELECT COUNT(*) FROM message_receipts WHERE message_id = ? AND status >= ?
	`

	InsertMessage = `
	INSERT OR REPLACE INTO messages 
	(message_id, chat_jid, sender_jid, timestamp, is_from_me, text, has_media, reply_to_message_id, edited, forwarded, type)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	UpdateMessage = `
	UPDATE messages
	SET text = ?, edited = TRUE
	WHERE message_id = ?
	`

	SelectMessageByID = `
	SELECT chat_jid, sender_jid, timestamp, is_from_me, text, has_media, reply_to_message_id, edited, forwarded
	FROM messages
	WHERE message_id = ?
	`

	SelectDecodedMessageByChatAndID = `
	SELECT m.sender_jid, m.timestamp, m.is_from_me, m.text, m.reply_to_message_id, m.edited, m.forwarded,
	       mm.type, mm.file_name, mm.width, mm.height, mm.gif_playback,
	       lp.url, lp.title, lp.description,
	       CASE WHEN length(COALESCE(lp.thumbnail, x'')) > 0 OR
	                      (COALESCE(lp.direct_path, '') <> '' AND length(COALESCE(lp.media_key, x'')) > 0)
	            THEN 1 ELSE 0 END
	FROM messages AS m
	LEFT JOIN message_media AS mm ON mm.message_id = m.message_id
	LEFT JOIN link_previews AS lp ON lp.message_id = m.message_id
	WHERE m.chat_jid = ? AND m.message_id = ?
	LIMIT 1
	`

	// Full-text-ish search within a single chat (case-insensitive LIKE on the
	// decoded text). Newest matches first.
	SearchMessagesByChat = `
	SELECT m.message_id, m.chat_jid, m.sender_jid, m.timestamp, m.is_from_me, m.text, m.reply_to_message_id, m.edited, m.forwarded,
	       mm.type, mm.file_name, mm.width, mm.height, mm.gif_playback,
	       lp.url, lp.title, lp.description,
	       CASE WHEN length(COALESCE(lp.thumbnail, x'')) > 0 OR
	                      (COALESCE(lp.direct_path, '') <> '' AND length(COALESCE(lp.media_key, x'')) > 0)
	            THEN 1 ELSE 0 END
	FROM messages AS m
	LEFT JOIN message_media AS mm ON mm.message_id = m.message_id
	LEFT JOIN link_previews AS lp ON lp.message_id = m.message_id
	WHERE m.chat_jid = ? AND m.text LIKE ? ESCAPE '\'
	ORDER BY m.timestamp DESC
	LIMIT ?
	`

	// Distinct chats with at least one message matching the query, ordered by
	// the most recent match. Excludes newsletters/broadcast. Used for global
	// (content) search in the chat list.
	SearchChatsByMessage = `
	SELECT chat_jid, MAX(timestamp) AS ts
	FROM messages
	WHERE text LIKE ? ESCAPE '\'
	  AND chat_jid NOT LIKE '%@newsletter'
	  AND chat_jid NOT LIKE '%@broadcast'
	GROUP BY chat_jid
	ORDER BY ts DESC
	LIMIT ?
	`

	SelectMessageTimestampByID = `
	SELECT timestamp FROM messages WHERE message_id = ? LIMIT 1
	`

	// A window of messages centred on a target timestamp: `limit` at/older than
	// the target plus `limit` newer, so a search result can be shown in context.
	SelectMessagesAround = `
	SELECT m.message_id, m.chat_jid, m.sender_jid, m.timestamp, m.is_from_me, m.text, m.reply_to_message_id, m.edited, m.forwarded,
	       mm.type, mm.file_name, mm.width, mm.height, mm.gif_playback,
	       lp.url, lp.title, lp.description,
	       CASE WHEN length(COALESCE(lp.thumbnail, x'')) > 0 OR
	                      (COALESCE(lp.direct_path, '') <> '' AND length(COALESCE(lp.media_key, x'')) > 0)
	            THEN 1 ELSE 0 END
	FROM (
		SELECT * FROM (
			SELECT message_id, chat_jid, sender_jid, timestamp, is_from_me, text, reply_to_message_id, edited, forwarded
			FROM messages
			WHERE chat_jid = ? AND timestamp <= ?
			ORDER BY timestamp DESC
			LIMIT ?
		)
		UNION
		SELECT * FROM (
			SELECT message_id, chat_jid, sender_jid, timestamp, is_from_me, text, reply_to_message_id, edited, forwarded
			FROM messages
			WHERE chat_jid = ? AND timestamp > ?
			ORDER BY timestamp ASC
			LIMIT ?
		)
	) AS m
	LEFT JOIN message_media AS mm ON mm.message_id = m.message_id
	LEFT JOIN link_previews AS lp ON lp.message_id = m.message_id
	ORDER BY m.timestamp ASC
	`

	// Migration queries for messages.db
	SelectAllMessagesJIDs = `
	SELECT message_id, chat_jid, sender_jid
	FROM messages;
	`

	UpdateMessageJIDs = `
	UPDATE messages
	SET chat_jid = ?, sender_jid = ?
	WHERE message_id = ?;
	`

	// Messages.db paged queries (for frontend)
	SelectMessagesByChatBeforeCursor = `
	SELECT m.message_id, m.chat_jid, m.sender_jid, m.timestamp, m.is_from_me, m.text, m.reply_to_message_id, m.edited, m.forwarded,
	       mm.type, mm.file_name, mm.width, mm.height, mm.gif_playback,
	       lp.url, lp.title, lp.description,
	       CASE WHEN length(COALESCE(lp.thumbnail, x'')) > 0 OR
	                      (COALESCE(lp.direct_path, '') <> '' AND length(COALESCE(lp.media_key, x'')) > 0)
	            THEN 1 ELSE 0 END
	FROM (
		SELECT message_id, chat_jid, sender_jid, timestamp, is_from_me, text, reply_to_message_id, edited, forwarded
		FROM messages
		WHERE chat_jid = ?
		  AND (timestamp < ? OR (timestamp = ? AND message_id < ?))
		ORDER BY timestamp DESC, message_id DESC
		LIMIT ?
	) AS m 
	LEFT JOIN message_media AS mm ON mm.message_id = m.message_id
	LEFT JOIN link_previews AS lp ON lp.message_id = m.message_id
	ORDER BY m.timestamp ASC, m.message_id ASC
	`

	SelectLatestMessagesByChat = `
	SELECT m.message_id, m.chat_jid, m.sender_jid, m.timestamp, m.is_from_me, m.text, m.reply_to_message_id, m.edited, m.forwarded,
	       mm.type, mm.file_name, mm.width, mm.height, mm.gif_playback,
	       lp.url, lp.title, lp.description,
	       CASE WHEN length(COALESCE(lp.thumbnail, x'')) > 0 OR
	                      (COALESCE(lp.direct_path, '') <> '' AND length(COALESCE(lp.media_key, x'')) > 0)
	            THEN 1 ELSE 0 END
	FROM (
		SELECT message_id, chat_jid, sender_jid, timestamp, is_from_me, text, reply_to_message_id, edited, forwarded
		FROM messages
		WHERE chat_jid = ?
		ORDER BY timestamp DESC, message_id DESC
		LIMIT ?
	) AS m
	LEFT JOIN message_media AS mm ON mm.message_id = m.message_id
	LEFT JOIN link_previews AS lp ON lp.message_id = m.message_id
	ORDER BY m.timestamp ASC, m.message_id ASC
	`

	// SelectDecodedMessagesByIDPrefix is completed with a placeholder list and
	// a closing parenthesis. It loads quoted-message summaries in one query.
	SelectDecodedMessagesByIDPrefix = `
	SELECT m.message_id, m.sender_jid, m.text, mm.type, mm.file_name,
	       mm.width, mm.height, mm.gif_playback
	FROM messages AS m
	LEFT JOIN message_media AS mm ON mm.message_id = m.message_id
	WHERE m.message_id IN (
	`

	SelectMessageByChatAndID = `
	SELECT sender_jid, timestamp, is_from_me, text, has_media, reply_to_message_id, edited, forwarded
	FROM messages
	WHERE chat_jid = ? AND message_id = ?
	LIMIT 1
	`

	// Chat list from messages.db
	SelectDecodedChatList = `
	SELECT m.message_id, m.chat_jid, m.sender_jid, m.timestamp, m.is_from_me, m.text, m.reply_to_message_id, m.edited, m.forwarded, mm.type, mm.file_name
	FROM (
		SELECT 
			message_id, chat_jid, sender_jid, timestamp, is_from_me, text, reply_to_message_id, edited, forwarded,
			ROW_NUMBER() OVER (
				PARTITION BY chat_jid
				ORDER BY timestamp DESC
			) AS rn
		FROM messages
	) AS m
	LEFT JOIN message_media AS mm ON mm.message_id = m.message_id
	WHERE rn = 1
	  AND m.chat_jid NOT LIKE '%@newsletter'
	  AND m.chat_jid NOT LIKE '%@broadcast'
	ORDER BY m.timestamp DESC;
	`

	// Same as SelectDecodedChatList but only Channels (newsletter feeds).
	SelectDecodedChannelList = `
	SELECT m.message_id, m.chat_jid, m.sender_jid, m.timestamp, m.is_from_me, m.text, m.reply_to_message_id, m.edited, m.forwarded, mm.type, mm.file_name
	FROM (
		SELECT
			message_id, chat_jid, sender_jid, timestamp, is_from_me, text, reply_to_message_id, edited, forwarded,
			ROW_NUMBER() OVER (
				PARTITION BY chat_jid
				ORDER BY timestamp DESC
			) AS rn
		FROM messages
	) AS m
	LEFT JOIN message_media AS mm ON mm.message_id = m.message_id
	WHERE rn = 1
	  AND m.chat_jid LIKE '%@newsletter'
	ORDER BY m.timestamp DESC;
	`

	UpdateMessagesChat = `
	UPDATE messages
	SET chat_jid = ?
	WHERE chat_jid = ?;
	`
)

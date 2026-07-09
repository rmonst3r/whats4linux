package query

const (
	CreateLinkPreviewsTable = `
	CREATE TABLE IF NOT EXISTS link_previews (
		message_id TEXT PRIMARY KEY,
		url TEXT,
		title TEXT,
		description TEXT,
		thumbnail BLOB,
		direct_path TEXT,
		media_key BLOB,
		file_sha256 BLOB,
		file_enc_sha256 BLOB
	);
	`

	// Migrations for link_previews tables created before the poster-download
	// key columns existed. Caller ignores the "duplicate column" error.
	AddLinkPreviewDirectPath = `ALTER TABLE link_previews ADD COLUMN direct_path TEXT;`
	AddLinkPreviewMediaKey   = `ALTER TABLE link_previews ADD COLUMN media_key BLOB;`
	AddLinkPreviewFileSHA    = `ALTER TABLE link_previews ADD COLUMN file_sha256 BLOB;`
	AddLinkPreviewFileEncSHA = `ALTER TABLE link_previews ADD COLUMN file_enc_sha256 BLOB;`

	InsertLinkPreview = `
	INSERT OR REPLACE INTO link_previews
	(message_id, url, title, description, thumbnail, direct_path, media_key, file_sha256, file_enc_sha256)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);
	`

	SelectLinkPreviewByMessageID = `
	SELECT url, title, description, thumbnail FROM link_previews WHERE message_id = ?;
	`

	// Download info + any cached thumbnail for lazily fetching the poster image.
	SelectLinkPreviewMediaByMessageID = `
	SELECT thumbnail, direct_path, media_key, file_sha256, file_enc_sha256
	FROM link_previews WHERE message_id = ?;
	`

	UpdateLinkPreviewThumbnail = `
	UPDATE link_previews SET thumbnail = ? WHERE message_id = ?;
	`
)

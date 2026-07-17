package query

const (
	CreateGroupsTable = `
	CREATE TABLE IF NOT EXISTS whats4linux_groups (
		jid TEXT PRIMARY KEY,
		name TEXT,
		topic TEXT,
		owner_jid TEXT,
		participant_count INTEGER,
		parent_jid TEXT DEFAULT '',
		parent_name TEXT DEFAULT '',
		is_parent INTEGER DEFAULT 0,
		is_default_sub INTEGER DEFAULT 0
	);
	`

	// Migrations for DBs created before community columns existed.
	// SQLite ignores duplicate column errors when we run these carefully from Go.
	AlterGroupsAddParentJID    = `ALTER TABLE whats4linux_groups ADD COLUMN parent_jid TEXT DEFAULT '';`
	AlterGroupsAddParentName   = `ALTER TABLE whats4linux_groups ADD COLUMN parent_name TEXT DEFAULT '';`
	AlterGroupsAddIsParent     = `ALTER TABLE whats4linux_groups ADD COLUMN is_parent INTEGER DEFAULT 0;`
	AlterGroupsAddIsDefaultSub = `ALTER TABLE whats4linux_groups ADD COLUMN is_default_sub INTEGER DEFAULT 0;`

	InsertOrReplaceGroup = `
	INSERT OR REPLACE INTO whats4linux_groups
	(jid, name, topic, owner_jid, participant_count, parent_jid, parent_name, is_parent, is_default_sub)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);
	`

	SelectAllGroups = `
	SELECT jid, name, topic, owner_jid, participant_count,
	       COALESCE(parent_jid, ''), COALESCE(parent_name, ''),
	       COALESCE(is_parent, 0), COALESCE(is_default_sub, 0)
	FROM whats4linux_groups;
	`

	SelectGroupByJID = `
	SELECT jid, name, topic, owner_jid, participant_count,
	       COALESCE(parent_jid, ''), COALESCE(parent_name, ''),
	       COALESCE(is_parent, 0), COALESCE(is_default_sub, 0)
	FROM whats4linux_groups
	WHERE jid = ?;
	`

	SelectCommunities = `
	SELECT jid, name, topic, owner_jid, participant_count,
	       COALESCE(parent_jid, ''), COALESCE(parent_name, ''),
	       COALESCE(is_parent, 0), COALESCE(is_default_sub, 0)
	FROM whats4linux_groups
	WHERE is_parent = 1
	ORDER BY name COLLATE NOCASE;
	`

	// Distinct parent communities discovered via child groups (parent may not be stored as is_parent).
	SelectDistinctParents = `
	SELECT DISTINCT parent_jid, parent_name
	FROM whats4linux_groups
	WHERE parent_jid IS NOT NULL AND parent_jid != '';
	`

	CountGroupsByParent = `
	SELECT COUNT(*) FROM whats4linux_groups WHERE parent_jid = ?;
	`
)

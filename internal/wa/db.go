package wa

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/lugvitc/whats4linux/internal/misc"
	"github.com/lugvitc/whats4linux/internal/query"

	"go.mau.fi/whatsmeow"
)

type AppDatabase struct {
	db  *sql.DB
	mu  sync.Mutex
	ctx context.Context
}

func NewAppDatabase(ctx context.Context) (*AppDatabase, error) {
	db, err := sql.Open("sqlite3", misc.GetSQLiteAddress("app.db"))
	if err != nil {
		return nil, err
	}
	return &AppDatabase{
		db:  db,
		ctx: ctx,
	}, nil
}

func (cw *AppDatabase) Initialise(client *whatsmeow.Client) error {
	_, err := cw.db.Exec(query.CreateGroupsTable)
	if err != nil {
		return fmt.Errorf("failed to create whats4linux_groups table: %w", err)
	}

	// Best-effort migrations for existing installs.
	for _, stmt := range []string{
		query.AlterGroupsAddParentJID,
		query.AlterGroupsAddParentName,
		query.AlterGroupsAddIsParent,
		query.AlterGroupsAddIsDefaultSub,
	} {
		if _, err := cw.db.Exec(stmt); err != nil {
			// "duplicate column name" is expected after the first run.
			if !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
				log.Println("groups schema migrate:", err)
			}
		}
	}

	err = cw.FetchAndStoreGroups(client)
	if err != nil {
		return fmt.Errorf("failed to fetch and store groups: %w", err)
	}
	return nil
}

func (cw *AppDatabase) FetchAndStoreGroups(client *whatsmeow.Client) error {
	cw.mu.Lock()
	defer cw.mu.Unlock()

	groups, err := client.GetJoinedGroups(cw.ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch joined groups: %w", err)
	}

	// parentJID -> display name
	parentNames := make(map[string]string)
	for _, group := range groups {
		if group.IsParent {
			parentNames[group.JID.String()] = group.Name
		}
	}

	// Resolve parent names for linked subgroups whose parent wasn't in the list.
	for _, group := range groups {
		if group.LinkedParentJID.IsEmpty() {
			continue
		}
		key := group.LinkedParentJID.String()
		if _, ok := parentNames[key]; ok {
			continue
		}
		info, err := client.GetGroupInfo(cw.ctx, group.LinkedParentJID)
		if err != nil {
			log.Println("FetchAndStoreGroups: parent info failed:", key, err)
			// Fall back later to a placeholder; still record the link.
			parentNames[key] = ""
			continue
		}
		name := info.Name
		if name == "" {
			name = "Community"
		}
		parentNames[key] = name

		// Ensure the parent itself is stored so communities list can find it.
		// Parent groups often have no participants in the list response.
	}

	tx, err := cw.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Clear stale rows so left groups / unlinked parents disappear.
	if _, err := tx.Exec(`DELETE FROM whats4linux_groups`); err != nil {
		return fmt.Errorf("failed to clear groups: %w", err)
	}

	stmt, err := tx.Prepare(query.InsertOrReplaceGroup)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	storedParents := make(map[string]bool)

	for _, group := range groups {
		parentJID := ""
		parentName := ""
		if !group.LinkedParentJID.IsEmpty() {
			parentJID = group.LinkedParentJID.String()
			parentName = parentNames[parentJID]
		}

		isParent := 0
		if group.IsParent {
			isParent = 1
			storedParents[group.JID.String()] = true
		}
		isDefaultSub := 0
		if group.IsDefaultSubGroup {
			isDefaultSub = 1
		}

		_, err := stmt.Exec(
			group.JID.String(),
			group.Name,
			group.Topic,
			group.OwnerJID.String(),
			len(group.Participants),
			parentJID,
			parentName,
			isParent,
			isDefaultSub,
		)
		if err != nil {
			return fmt.Errorf("failed to insert group %s: %w", group.JID.String(), err)
		}
	}

	// Store parent community rows that only appeared as LinkedParentJID.
	for parentJID, parentName := range parentNames {
		if storedParents[parentJID] {
			continue
		}
		if parentName == "" {
			parentName = "Community"
		}
		_, err := stmt.Exec(
			parentJID,
			parentName,
			"",
			"",
			0,
			"",
			"",
			1, // is_parent
			0,
		)
		if err != nil {
			return fmt.Errorf("failed to insert parent community %s: %w", parentJID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

type Group struct {
	JID              string
	Name             string
	Topic            string
	OwnerJID         string
	ParticipantCount int
	ParentJID        string
	ParentName       string
	IsParent         bool
	IsDefaultSub     bool
}

func scanGroup(scanner interface{ Scan(dest ...any) error }) (*Group, error) {
	var g Group
	var isParent, isDefaultSub int
	err := scanner.Scan(
		&g.JID, &g.Name, &g.Topic, &g.OwnerJID, &g.ParticipantCount,
		&g.ParentJID, &g.ParentName, &isParent, &isDefaultSub,
	)
	if err != nil {
		return nil, err
	}
	g.IsParent = isParent != 0
	g.IsDefaultSub = isDefaultSub != 0
	return &g, nil
}

func (cw *AppDatabase) FetchGroups() ([]Group, error) {
	rows, err := cw.db.Query(query.SelectAllGroups)
	if err != nil {
		return nil, fmt.Errorf("failed to query groups: %w", err)
	}
	defer rows.Close()

	var groups []Group
	for rows.Next() {
		g, err := scanGroup(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan group row: %w", err)
		}
		groups = append(groups, *g)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return groups, nil
}

// StoreGroup upserts a single group row. Used to repair rows that were
// stored with an empty name during early history sync.
func (cw *AppDatabase) StoreGroup(g Group) error {
	cw.mu.Lock()
	defer cw.mu.Unlock()

	isParent, isDefaultSub := 0, 0
	if g.IsParent {
		isParent = 1
	}
	if g.IsDefaultSub {
		isDefaultSub = 1
	}
	_, err := cw.db.Exec(query.InsertOrReplaceGroup,
		g.JID, g.Name, g.Topic, g.OwnerJID, g.ParticipantCount,
		g.ParentJID, g.ParentName, isParent, isDefaultSub)
	if err != nil {
		return fmt.Errorf("failed to upsert group %s: %w", g.JID, err)
	}
	return nil
}

func (cw *AppDatabase) FetchGroup(jid string) (*Group, error) {
	row := cw.db.QueryRow(query.SelectGroupByJID, jid)
	g, err := scanGroup(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("group with JID %s not found", jid)
		}
		return nil, fmt.Errorf("failed to scan group row: %w", err)
	}
	return g, nil
}

// FetchCommunities returns stored parent communities (is_parent=1).
func (cw *AppDatabase) FetchCommunities() ([]Group, error) {
	rows, err := cw.db.Query(query.SelectCommunities)
	if err != nil {
		return nil, fmt.Errorf("failed to query communities: %w", err)
	}
	defer rows.Close()

	var groups []Group
	for rows.Next() {
		g, err := scanGroup(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan community row: %w", err)
		}
		groups = append(groups, *g)
	}
	return groups, rows.Err()
}

// CountLinkedGroups returns how many stored groups link to parentJID.
func (cw *AppDatabase) CountLinkedGroups(parentJID string) (int, error) {
	var n int
	err := cw.db.QueryRow(query.CountGroupsByParent, parentJID).Scan(&n)
	return n, err
}

// ParentCommunityName looks up a stored parent display name.
func (cw *AppDatabase) ParentCommunityName(parentJID string) string {
	if parentJID == "" {
		return ""
	}
	g, err := cw.FetchGroup(parentJID)
	if err == nil && g.Name != "" {
		return g.Name
	}
	// Fall back to any child row that cached parent_name.
	var name string
	_ = cw.db.QueryRow(
		`SELECT parent_name FROM whats4linux_groups WHERE parent_jid = ? AND parent_name != '' LIMIT 1`,
		parentJID,
	).Scan(&name)
	return name
}

func (cw *AppDatabase) Close() error {
	return cw.db.Close()
}

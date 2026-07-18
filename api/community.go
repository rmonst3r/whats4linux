package api

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// CommunitySummary is a community entry for the communities list.
type CommunitySummary struct {
	JID        string `json:"jid"`
	Name       string `json:"name"`
	Topic      string `json:"topic,omitempty"`
	GroupCount int    `json:"group_count"`
	AvatarURL  string `json:"avatar_url,omitempty"`
}

// CommunityGroup is a subgroup (or announcement group) inside a community.
type CommunityGroup struct {
	JID               string `json:"jid"`
	Name              string `json:"name"`
	IsAnnouncement    bool   `json:"is_announcement"`
	IsDefaultSubGroup bool   `json:"is_default_sub_group"`
	AvatarURL         string `json:"avatar_url,omitempty"`
}

// CommunityDetails is the community home view: metadata + linked groups.
type CommunityDetails struct {
	JID          string           `json:"jid"`
	Name         string           `json:"name"`
	Topic        string           `json:"topic,omitempty"`
	CreatedAt    string           `json:"created_at,omitempty"`
	MemberCount  int              `json:"member_count"`
	AvatarURL    string           `json:"avatar_url,omitempty"`
	Announcement *CommunityGroup  `json:"announcement,omitempty"`
	Groups       []CommunityGroup `json:"groups"`
}

// GetCommunityList returns parent communities the user belongs to.
// Prefers the local group cache (filled at login); refreshes from WhatsApp only
// when the cache is empty. Opening the tab must not block on a network refresh.
func (a *Api) GetCommunityList() ([]CommunitySummary, error) {
	if a.waClient == nil {
		return nil, fmt.Errorf("client not ready")
	}

	if a.cw != nil {
		if list, err := a.communitiesFromCache(); err == nil && len(list) > 0 {
			return list, nil
		}

		// A new install may reach the tab before the Connected handler finishes
		// populating the cache. Refresh once, then return the resulting cache even
		// when it is legitimately empty.
		if err := a.cw.FetchAndStoreGroups(a.waClient); err == nil {
			return a.communitiesFromCache()
		} else {
			log.Println("GetCommunityList: cache refresh failed:", err)
		}
	}

	// Live discovery from WhatsApp.
	return a.communitiesFromLive()
}

func (a *Api) communitiesFromCache() ([]CommunitySummary, error) {
	parents, err := a.cw.FetchCommunities()
	if err != nil {
		return nil, err
	}

	// Also include parents that only exist as parent_jid on children.
	// FetchCommunities only returns is_parent=1 rows; those are inserted for
	// both real parent groups and synthesized parent rows.
	result := make([]CommunitySummary, 0, len(parents))
	for _, p := range parents {
		count, _ := a.cw.CountLinkedGroups(p.JID)
		name := p.Name
		if name == "" {
			name = "Community"
		}
		result = append(result, CommunitySummary{
			JID:        p.JID,
			Name:       name,
			Topic:      p.Topic,
			GroupCount: count,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return result, nil
}

func (a *Api) communitiesFromLive() ([]CommunitySummary, error) {
	groups, err := a.waClient.GetJoinedGroups(a.ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch groups: %w", err)
	}

	type parentMeta struct {
		name  string
		topic string
	}
	parents := make(map[string]parentMeta)
	linkedCount := make(map[string]int)

	for _, g := range groups {
		if g.IsParent {
			parents[g.JID.String()] = parentMeta{name: g.Name, topic: g.Topic}
		}
		if !g.LinkedParentJID.IsEmpty() {
			key := g.LinkedParentJID.String()
			linkedCount[key]++
			if _, ok := parents[key]; !ok {
				// Always register — LinkedParent means this is a community.
				name := ""
				topic := ""
				if info, err := a.waClient.GetGroupInfo(a.ctx, g.LinkedParentJID); err == nil {
					name = info.Name
					topic = info.Topic
				}
				if name == "" {
					name = "Community"
				}
				parents[key] = parentMeta{name: name, topic: topic}
			}
		}
	}

	result := make([]CommunitySummary, 0, len(parents))
	for jid, meta := range parents {
		result = append(result, CommunitySummary{
			JID:        jid,
			Name:       meta.name,
			Topic:      meta.topic,
			GroupCount: linkedCount[jid],
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})

	log.Printf("GetCommunityList: discovered %d communities from live groups", len(result))
	return result, nil
}

// GetCommunityDetails returns community home data: info + announcement + groups.
func (a *Api) GetCommunityDetails(jidStr string) (CommunityDetails, error) {
	if a.waClient == nil {
		return CommunityDetails{}, fmt.Errorf("client not ready")
	}
	if !strings.HasSuffix(jidStr, "@g.us") {
		return CommunityDetails{}, fmt.Errorf("JID is not a group/community JID")
	}

	jid, err := types.ParseJID(jidStr)
	if err != nil {
		return CommunityDetails{}, fmt.Errorf("invalid JID: %w", err)
	}

	details := CommunityDetails{
		JID:    jid.String(),
		Name:   "Community",
		Groups: []CommunityGroup{},
	}

	// Prefer live community info; fall back to cache.
	if info, err := a.waClient.GetGroupInfo(a.ctx, jid); err == nil {
		details.Name = info.Name
		details.Topic = info.Topic
		details.MemberCount = info.ParticipantCount
		if !info.GroupCreated.IsZero() {
			details.CreatedAt = info.GroupCreated.Format("2006-01-02")
		}
	} else if a.cw != nil {
		if g, err := a.cw.FetchGroup(jidStr); err == nil {
			details.Name = g.Name
			details.Topic = g.Topic
		}
	}
	if details.Name == "" {
		details.Name = "Community"
	}

	// Linked subgroups (includes announcement / default sub-group).
	subGroups, err := a.waClient.GetSubGroups(a.ctx, jid)
	if err != nil {
		log.Println("GetCommunityDetails: GetSubGroups failed:", jidStr, err)
		return a.communityDetailsFromJoined(details, jid)
	}

	for _, sg := range subGroups {
		if sg == nil {
			continue
		}
		cg := CommunityGroup{
			JID:               sg.JID.String(),
			Name:              sg.Name,
			IsDefaultSubGroup: sg.IsDefaultSubGroup,
			IsAnnouncement:    sg.IsDefaultSubGroup,
		}
		if cg.IsAnnouncement && (cg.Name == "" || cg.Name == details.Name) {
			cg.Name = "Announcements"
		}
		if cg.IsAnnouncement {
			details.Announcement = &cg
		} else {
			details.Groups = append(details.Groups, cg)
		}
	}

	if details.MemberCount == 0 {
		if members, err := a.waClient.GetLinkedGroupsParticipants(a.ctx, jid); err == nil {
			details.MemberCount = len(members)
		}
	}

	return details, nil
}

func (a *Api) communityDetailsFromJoined(details CommunityDetails, parent types.JID) (CommunityDetails, error) {
	// Prefer local cache of linked groups.
	if a.cw != nil {
		if groups, err := a.cw.FetchGroups(); err == nil {
			parentKey := parent.String()
			for _, g := range groups {
				if g.ParentJID != parentKey {
					continue
				}
				cg := CommunityGroup{
					JID:               g.JID,
					Name:              g.Name,
					IsDefaultSubGroup: g.IsDefaultSub,
					IsAnnouncement:    g.IsDefaultSub,
				}
				if cg.IsAnnouncement && (cg.Name == "" || cg.Name == details.Name) {
					cg.Name = "Announcements"
				}
				if cg.IsAnnouncement {
					details.Announcement = &cg
				} else {
					details.Groups = append(details.Groups, cg)
				}
			}
			if details.Announcement != nil || len(details.Groups) > 0 {
				return details, nil
			}
		}
	}

	groups, err := a.waClient.GetJoinedGroups(a.ctx)
	if err != nil {
		return details, nil
	}

	for _, g := range groups {
		if g.LinkedParentJID != parent {
			continue
		}
		cg := CommunityGroup{
			JID:               g.JID.String(),
			Name:              g.Name,
			IsDefaultSubGroup: g.IsDefaultSubGroup,
			IsAnnouncement:    g.IsDefaultSubGroup,
		}
		if cg.IsAnnouncement && (cg.Name == "" || cg.Name == details.Name) {
			cg.Name = "Announcements"
		}
		if cg.IsAnnouncement {
			details.Announcement = &cg
		} else {
			details.Groups = append(details.Groups, cg)
		}
	}
	return details, nil
}

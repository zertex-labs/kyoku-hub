package fsagent

import (
	"strings"
	"sync"
)

// TrackIssue describes missing required tags on a single scanned file.
type TrackIssue struct {
	Rpath   string   `json:"rpath"`
	Missing []string `json:"missing"`
}

// UntaggedTrack is a scanned file lacking a MusicBrainz release-group ID, so
// it cannot be linked to a release group directly.
type UntaggedTrack struct {
	AgentID string `json:"agent_id"`
	Rpath   string `json:"rpath"`
	Title   string `json:"title"`
	Album   string `json:"album"`
	Artist  string `json:"artist"`
}

// scanState holds the details of an agent's latest scan. Kept in memory only:
// it is cheap to recompute and repopulated on every sync (agents sync
// automatically on connect).
type scanState struct {
	issues   map[string][]TrackIssue // rgID -> tracks with missing tags
	untagged []UntaggedTrack
}

// stateStore is an in-memory map of the latest scan details per user/agent.
type stateStore struct {
	mu sync.RWMutex
	// userID -> agentID -> latest scan
	byUser map[string]map[string]*scanState
}

func newStateStore() *stateStore {
	return &stateStore{byUser: make(map[string]map[string]*scanState)}
}

// setScan replaces the stored scan details for one agent.
func (s *stateStore) setScan(userID, agentID string, issues map[string][]TrackIssue, untagged []UntaggedTrack) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.byUser[userID] == nil {
		s.byUser[userID] = make(map[string]*scanState)
	}
	s.byUser[userID][agentID] = &scanState{issues: issues, untagged: untagged}
}

// dropAgent removes stored scan details for one agent (e.g. when the agent
// registration is deleted).
func (s *stateStore) dropAgent(userID, agentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if agents, ok := s.byUser[userID]; ok {
		delete(agents, agentID)
		if len(agents) == 0 {
			delete(s.byUser, userID)
		}
	}
}

// issuesFor returns tag issues for one release group, merged across the
// user's agents.
func (s *stateStore) issuesFor(userID, rgID string) []TrackIssue {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []TrackIssue
	for _, scan := range s.byUser[userID] {
		out = append(out, scan.issues[rgID]...)
	}
	return out
}

// untaggedFor returns all untagged tracks for a user, merged across agents.
func (s *stateStore) untaggedFor(userID string) []UntaggedTrack {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []UntaggedTrack
	for _, scan := range s.byUser[userID] {
		out = append(out, scan.untagged...)
	}
	return out
}

// hasUntaggedMatch reports whether any of the user's untagged tracks look
// like they belong to the given artist + release-group title (normalized,
// case-insensitive comparison).
func (s *stateStore) hasUntaggedMatch(userID, artistName, rgTitle string) bool {
	artist := normalize(artistName)
	title := normalize(rgTitle)
	if artist == "" || title == "" {
		return false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, scan := range s.byUser[userID] {
		for _, t := range scan.untagged {
			if normalize(t.Artist) == artist && normalize(t.Album) == title {
				return true
			}
		}
	}
	return false
}

// normalize lowercases and collapses whitespace for fuzzy-ish name matching.
func normalize(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

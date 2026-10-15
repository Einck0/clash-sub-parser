package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"
)

// ComputeConfigurationDigest hashes logical policy content, excluding edit metadata.
func ComputeConfigurationDigest(groups []NodeGroup, edges map[string][]GroupEdge) (string, error) {
	sortedGroups := append([]NodeGroup(nil), groups...)
	// Keep the historical empty graph representation stable.
	if sortedGroups == nil {
		sortedGroups = []NodeGroup{}
	}
	for i := range sortedGroups {
		sortedGroups[i].Revision = 0
		sortedGroups[i].CreatedAt = time.Time{}
		sortedGroups[i].UpdatedAt = time.Time{}
		sortedGroups[i].NodeFilter = nil
		sources := append([]MemberSource(nil), sortedGroups[i].Sources...)
		sort.SliceStable(sources, func(a, b int) bool { return sources[a].Position < sources[b].Position })
		for j := range sources {
			sources[j].CreatedAt = time.Time{}
			sources[j].UpdatedAt = time.Time{}
		}
		sortedGroups[i].Sources = sources
	}
	sort.SliceStable(sortedGroups, func(i, j int) bool { return sortedGroups[i].ID < sortedGroups[j].ID })
	canonicalEdges := make(map[string][]GroupEdge, len(edges))
	for k, list := range edges {
		sorted := make([]GroupEdge, len(list))
		copy(sorted, list)
		sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Position < sorted[j].Position })
		canonicalEdges[k] = sorted
	}
	data, err := json.Marshal(struct {
		Groups []NodeGroup            `json:"groups"`
		Edges  map[string][]GroupEdge `json:"edges"`
	}{sortedGroups, canonicalEdges})
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

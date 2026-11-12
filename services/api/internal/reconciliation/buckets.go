package reconciliation

import (
	"strings"
	"time"

	"github.com/cocok/cocok/services/api/internal/matching"
	"github.com/google/uuid"
)

// buckets returns all txs of one direction (as match targets / pool).
func buckets(byID map[uuid.UUID]matching.Tx, side string) []matching.Tx {
	out := make([]matching.Tx, 0, len(byID))
	for _, t := range byID {
		if t.Direction == side {
			out = append(out, t)
		}
	}
	return out
}

// creditGroups / debitGroups bucket one side by normalized counterparty and by
// booking date — the two realistic sources of N:1 settlements.
func creditGroups(rows []matching.Tx) [][]matching.Tx { return groupRows(rows) }
func debitGroups(rows []matching.Tx) [][]matching.Tx  { return groupRows(rows) }

func groupRows(rows []matching.Tx) [][]matching.Tx {
	byCP := map[string][]matching.Tx{}
	byDate := map[string][]matching.Tx{}
	for _, t := range rows {
		if cp := normCP(t.Counterparty); cp != "" {
			byCP[cp] = append(byCP[cp], t)
		}
		d := t.BookedAt.Format("2006-01-02")
		byDate[d] = append(byDate[d], t)
	}
	var out [][]matching.Tx
	for _, g := range byCP {
		out = append(out, g)
	}
	for _, g := range byDate {
		out = append(out, g)
	}
	return out
}

func normCP(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// inWindow: target booked within the members' span ± windowDays.
func inWindow(members []matching.Tx, target matching.Tx, windowDays int) bool {
	if len(members) == 0 || windowDays < 0 {
		return false
	}
	minT, maxT := members[0].BookedAt, members[0].BookedAt
	for _, m := range members[1:] {
		if m.BookedAt.Before(minT) {
			minT = m.BookedAt
		}
		if m.BookedAt.After(maxT) {
			maxT = m.BookedAt
		}
	}
	pad := time.Duration(windowDays) * 24 * time.Hour
	return !target.BookedAt.Before(minT.Add(-pad)) && !target.BookedAt.After(maxT.Add(pad))
}

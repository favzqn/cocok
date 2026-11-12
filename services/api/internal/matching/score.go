package matching

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Weights is the versioned scoring configuration (from scoring_weights).
// Tolerances are integers — never float money math.
type Weights struct {
	Version           int     `json:"version"`
	Amount            float64 `json:"amount"`
	Date              float64 `json:"date"`
	Ref               float64 `json:"ref"`
	Counterparty      float64 `json:"counterparty"`
	Description       float64 `json:"description"`
	MinScore          float64 `json:"min_score"`
	AmountToleranceBP int     `json:"amount_tolerance_bp"`
	DateWindowDays    int     `json:"date_window_days"`
}

// Tx is the scoring view of a canonical transaction.
type Tx struct {
	ID           uuid.UUID
	BookedAt     time.Time
	Direction    string
	AmountMinor  int64
	Description  string
	Counterparty string
	ExternalRef  string
}

// Result carries total score plus per-component breakdown (stored as jsonb).
type Result struct {
	Score      float64
	Components map[string]float64
}

// Score evaluates a candidate: debit side vs credit side (either may be N rows).
// Components in [0,1]; total = weighted sum, clamped to [0,1].
func Score(debit, credit []Tx, w Weights) Result {
	if len(debit) == 0 || len(credit) == 0 {
		return Result{Score: 0, Components: map[string]float64{}}
	}
	sum := func(rows []Tx) int64 {
		var s int64
		for _, t := range rows {
			s += t.AmountMinor
		}
		return s
	}
	dSum, cSum := sum(debit), sum(credit)

	// amount: exact sum match → 1; else 1 - relDiff (relDiff > 1 → 0).
	amount := 1.0
	if dSum != cSum {
		max := math.Max(float64(dSum), float64(cSum))
		if max <= 0 {
			amount = 0
		} else {
			rel := math.Abs(float64(dSum-cSum)) / max
			amount = math.Max(0, 1-rel)
		}
	}

	// date: span across all members vs window.
	span := 0.0
	{
		var minT, maxT time.Time
		for _, t := range append(append([]Tx{}, debit...), credit...) {
			if minT.IsZero() || t.BookedAt.Before(minT) {
				minT = t.BookedAt
			}
			if maxT.IsZero() || t.BookedAt.After(maxT) {
				maxT = t.BookedAt
			}
		}
		days := maxT.Sub(minT).Hours() / 24
		span = days
	}
	date := 1.0
	if w.DateWindowDays > 0 {
		date = math.Max(0, 1-span/float64(w.DateWindowDays))
	}

	// ref: any external_ref shared across sides → 1; refs on both sides but none
	// shared → 0; fewer than two refs total → neutral 0.5.
	ref := 0.5
	dRefs, cRefs := refSet(debit), refSet(credit)
	if len(dRefs) > 0 && len(cRefs) > 0 {
		ref = 0
		for r := range dRefs {
			if cRefs[r] {
				ref = 1
				break
			}
		}
	}

	// counterparty: Jaccard on normalized names; both empty → neutral 0.5.
	cp := jaccard(tokenSet(sideText(debit, true)), tokenSet(sideText(credit, true)), 0.5)

	// description: Jaccard on tokens; both empty → neutral 0.5.
	desc := jaccard(tokenSet(sideText(debit, false)), tokenSet(sideText(credit, false)), 0.5)

	comps := map[string]float64{
		"amount":       amount,
		"date":         date,
		"ref":          ref,
		"counterparty": cp,
		"description":  desc,
	}
	total := w.Amount*amount + w.Date*date + w.Ref*ref + w.Counterparty*cp + w.Description*desc
	return Result{Score: math.Max(0, math.Min(1, total)), Components: comps}
}

// Kind labels group cardinality.
func Kind(debitN, creditN int) string {
	switch {
	case debitN <= 1 && creditN <= 1:
		return "1:1"
	case debitN > 1:
		return "1:N" // many debits settled by one credit
	default:
		return "N:1"
	}
}

// Fingerprint dedupes candidates regardless of member order.
func Fingerprint(creditIDs, debitIDs []uuid.UUID) string {
	c := joinIDs(creditIDs)
	d := joinIDs(debitIDs)
	h := sha256.Sum256([]byte(c + "|" + d))
	return hex.EncodeToString(h[:])
}

func joinIDs(ids []uuid.UUID) string {
	s := make([]string, len(ids))
	for i, id := range ids {
		s[i] = id.String()
	}
	sort.Strings(s)
	return strings.Join(s, ",")
}

func refSet(rows []Tx) map[string]bool {
	m := map[string]bool{}
	for _, t := range rows {
		if r := strings.TrimSpace(t.ExternalRef); r != "" {
			m[r] = true
		}
	}
	return m
}

// sideText joins counterparty (or description) across a side.
func sideText(rows []Tx, counterparty bool) string {
	var b strings.Builder
	for _, t := range rows {
		if counterparty {
			b.WriteString(t.Counterparty)
		} else {
			b.WriteString(t.Description)
		}
		b.WriteByte(' ')
	}
	return b.String()
}

func tokenSet(s string) map[string]bool {
	m := map[string]bool{}
	for _, tok := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	}) {
		if tok != "" {
			m[tok] = true
		}
	}
	return m
}

func jaccard(a, b map[string]bool, neutralIfEmpty float64) float64 {
	if len(a) == 0 || len(b) == 0 {
		return neutralIfEmpty
	}
	inter := 0
	for t := range a {
		if b[t] {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return neutralIfEmpty
	}
	return float64(inter) / float64(union)
}

package imports

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/cocok/cocok/services/api/internal/platform"
	"github.com/jackc/pgx/v5/pgtype"
)

// Mapping maps canonical fields to CSV header names (as written in the file).
type Mapping struct {
	Date         string `json:"date"`
	Amount       string `json:"amount"`
	Description  string `json:"description,omitempty"`
	Counterparty string `json:"counterparty,omitempty"`
	Direction    string `json:"direction,omitempty"` // column with debit/credit values; optional
	Reference    string `json:"reference,omitempty"`
}

// ParsedRow is one normalized canonical transaction candidate.
type ParsedRow struct {
	SourceRow    int
	BookedAt     time.Time
	Direction    string // credit | debit
	AmountMinor  int64  // always >= 0 (sign folded into direction)
	Description  string
	Counterparty string
	ExternalRef  string
	Fingerprint  string
	Raw          map[string]string
}

// RowError marks a row that failed validation.
type RowError struct {
	Row     int    `json:"row"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

var dateLayouts = []string{
	"2006-01-02",
	"02/01/2006",
	"2006/01/02",
	"02-01-2006",
	"2 Jan 2006",
	"2 January 2006",
	"Jan 2, 2006",
	"January 2, 2006",
}

func parseDate(s string, loc *time.Location) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("date is required")
	}
	// Drop time part if present (take the date token).
	if i := strings.IndexAny(s, "T "); i > 0 {
		datePart := s[:i]
		for _, l := range dateLayouts {
			if t, err := time.ParseInLocation(l, datePart, loc); err == nil {
				return t, nil
			}
		}
	}
	for _, l := range dateLayouts {
		if t, err := time.ParseInLocation(l, s, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized date %q", s)
}

func parseDirection(s string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "credit", "cr", "in", "masuk", "kredit":
		return "credit", true
	case "debit", "dr", "out", "keluar", "debet":
		return "debit", true
	}
	return "", false
}

// ParseCSV parses the whole file against mapping. Returns valid rows, per-row
// errors, and the header row (header is returned even on column-not-found so
// the client can remap).
func ParseCSV(raw []byte, m Mapping, currency string, loc *time.Location) ([]ParsedRow, []RowError, []string, error) {
	r := csv.NewReader(bytes.NewReader(raw))
	r.FieldsPerRecord = -1 // lenient; per-row length checked below
	r.LazyQuotes = true
	records, err := r.ReadAll()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%w: invalid CSV: %v", platform.ErrValidation, err)
	}
	if len(records) < 2 {
		return nil, nil, nil, fmt.Errorf("%w: CSV must have a header and at least one data row", platform.ErrValidation)
	}
	header := trimHeader(records[0])
	lowerIdx := map[string]int{}
	for i, h := range header {
		if _, dup := lowerIdx[strings.ToLower(h)]; !dup {
			lowerIdx[strings.ToLower(h)] = i
		}
	}
	col := func(name string) (int, bool) {
		name = strings.TrimSpace(name)
		if name == "" {
			return -1, false
		}
		i, ok := lowerIdx[strings.ToLower(name)]
		if !ok {
			return -1, false
		}
		return i, true
	}

	dateI, ok := col(m.Date)
	if !ok {
		return nil, nil, header, fmt.Errorf("%w: date column %q not found in header", platform.ErrValidation, m.Date)
	}
	amtI, ok := col(m.Amount)
	if !ok {
		return nil, nil, header, fmt.Errorf("%w: amount column %q not found in header", platform.ErrValidation, m.Amount)
	}
	descI, _ := col(m.Description)
	cpI, _ := col(m.Counterparty)
	dirI, _ := col(m.Direction)
	refI, _ := col(m.Reference)

	var rows []ParsedRow
	var errs []RowError
	for i, rec := range records[1:] {
		rowNo := i + 2 // 1-based incl. header
		get := func(ci int) string {
			if ci < 0 || ci >= len(rec) {
				return ""
			}
			return strings.TrimSpace(rec[ci])
		}
		rawMap := map[string]string{}
		for j, h := range header {
			if j < len(rec) {
				rawMap[h] = rec[j]
			}
		}

		booked, err := parseDate(get(dateI), loc)
		if err != nil {
			errs = append(errs, RowError{Row: rowNo, Field: "date", Message: err.Error()})
			continue
		}
		signed, err := platform.ParseAmount(get(amtI), currency)
		if err != nil {
			errs = append(errs, RowError{Row: rowNo, Field: "amount", Message: err.Error()})
			continue
		}
		dir := ""
		if dirI >= 0 {
			dir, ok = parseDirection(get(dirI))
			if !ok {
				errs = append(errs, RowError{Row: rowNo, Field: "direction", Message: "direction must be credit/debit (or masuk/keluar)"})
				continue
			}
		} else if signed < 0 {
			dir = "debit"
		} else {
			dir = "credit"
		}
		amount := signed
		if amount < 0 {
			amount = -amount
		}

		pr := ParsedRow{
			SourceRow:    rowNo,
			BookedAt:     booked,
			Direction:    dir,
			AmountMinor:  amount,
			Description:  get(descI),
			Counterparty: get(cpI),
			ExternalRef:  get(refI),
			Raw:          rawMap,
		}
		pr.Fingerprint = Fingerprint(pr)
		rows = append(rows, pr)
	}
	return rows, errs, header, nil
}

// Fingerprint dedupes re-imports of the same source line.
// Includes external_ref when present so distinct payments with equal
// amount/description on the same day are not collapsed.
func Fingerprint(p ParsedRow) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%d|%s|%s|%s",
		p.BookedAt.Format("2006-01-02"),
		p.Direction,
		p.AmountMinor,
		normalizeDesc(p.Description),
		normalizeDesc(p.Counterparty),
		strings.TrimSpace(p.ExternalRef),
	)
	return hex.EncodeToString(h.Sum(nil))
}

func normalizeDesc(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

func trimHeader(h []string) []string {
	out := make([]string, len(h))
	for i, s := range h {
		out[i] = strings.TrimSpace(s)
	}
	return out
}

// SampleRows limits preview payload size.
func SampleRows(rows []ParsedRow, n int) []ParsedRow {
	if len(rows) <= n {
		return rows
	}
	return rows[:n]
}

// toText converts optional strings for sqlc params.
func toText(s string) pgtype.Text {
	s = strings.TrimSpace(s)
	return pgtype.Text{String: s, Valid: s != ""}
}

package imports

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strings"

	"github.com/cocok/cocok/services/api/internal/platform"
	"github.com/xuri/excelize/v2"
)

// XLSXToCSV converts the first worksheet of an xlsx workbook into CSV bytes so
// the whole ingest pipeline (mapping, validation, preview, commit) stays shared.
func XLSXToCSV(raw []byte) ([]byte, error) {
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid xlsx: %v", platform.ErrValidation, err)
	}
	defer f.Close()
	sheet := f.GetSheetName(0)
	if sheet == "" {
		return nil, fmt.Errorf("%w: workbook has no sheets", platform.ErrValidation)
	}
	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, fmt.Errorf("%w: cannot read sheet: %v", platform.ErrValidation, err)
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("%w: sheet must have a header and at least one data row", platform.ErrValidation)
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	for _, row := range rows {
		// excelize drops trailing empty cells; pad so column indexes stay stable.
		if err := w.Write(pad(row, len(rows[0]))); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func pad(row []string, n int) []string {
	for len(row) < n {
		row = append(row, "")
	}
	return row
}

// LooksLikeXLSX reports whether the upload should take the xlsx path.
func LooksLikeXLSX(filename string) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(filename)), ".xlsx")
}

// loadParsable returns CSV bytes for either supported upload format; the raw
// original file is what gets stored (source of truth for reprocessing).
func loadParsable(filename string, raw []byte) ([]byte, error) {
	if LooksLikeXLSX(filename) {
		return XLSXToCSV(raw)
	}
	return raw, nil
}

// PeekHeaders returns the header row for either upload format (used for
// server-side column guessing when the client cannot parse xlsx itself).
func PeekHeaders(filename string, raw []byte) ([]string, error) {
	if LooksLikeXLSX(filename) {
		f, err := excelize.OpenReader(bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("%w: invalid xlsx: %v", platform.ErrValidation, err)
		}
		defer f.Close()
		sheet := f.GetSheetName(0)
		rows, err := f.GetRows(sheet)
		if err != nil || len(rows) == 0 {
			return nil, fmt.Errorf("%w: cannot read sheet", platform.ErrValidation)
		}
		return trimHeader(rows[0]), nil
	}
	r := csv.NewReader(bytes.NewReader(raw))
	r.FieldsPerRecord = -1
	rec, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("%w: cannot read header: %v", platform.ErrValidation, err)
	}
	return trimHeader(rec), nil
}

// GuessMapping maps well-known Indonesian/English header names to canonical
// fields — best effort; the client can remap after preview.
func GuessMapping(header []string) Mapping {
	find := func(patterns ...string) string {
		for _, h := range header {
			l := strings.ToLower(h)
			for _, p := range patterns {
				if strings.Contains(l, p) {
					return h
				}
			}
		}
		return ""
	}
	return Mapping{
		Date:         find("tanggal", "date", "tgl", "booking"),
		Amount:       find("nominal", "amount", "nilai", "jumlah", "gross"),
		Description:  find("keterangan", "description", "deskripsi", "memo", "narrative"),
		Counterparty: find("counterparty", "payer", "payee", "merchant", "nama"),
		Direction:    find("jenis", "direction", "tipe"),
		Reference:    find("referensi", "reference", "ref", "trx", "order"),
	}
}

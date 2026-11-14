package imports

import (
	"github.com/xuri/excelize/v2"
	"testing"
)

func buildXLSX(t *testing.T) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	_ = f.SetSheetName("Sheet1", "Statement")
	rows := [][]interface{}{
		{"Tanggal", "Keterangan", "Nominal", "Jenis", "Ref"},
		{"2024-05-01", "XLSX payout", 750000, "masuk", "XL-1"},
		{"2024-05-02", "XLSX fee", -25000, "keluar", "XL-2"},
		{"bad-date", "broken", 1000, "masuk", "XL-3"},
	}
	for r, row := range rows {
		for c, v := range row {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+1)
			if err := f.SetCellValue("Statement", cell, v); err != nil {
				t.Fatal(err)
			}
		}
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestXLSXToCSV(t *testing.T) {
	raw := buildXLSX(t)
	csvBytes, err := XLSXToCSV(raw)
	if err != nil {
		t.Fatalf("XLSXToCSV: %v", err)
	}
	m := Mapping{Date: "Tanggal", Amount: "Nominal", Description: "Keterangan", Direction: "Jenis", Reference: "Ref"}
	rows, errs, header, err := ParseCSV(csvBytes, m, "IDR", loc(t))
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	if len(header) != 5 {
		t.Errorf("header = %v", header)
	}
	if len(rows) != 2 || len(errs) != 1 {
		t.Errorf("rows=%d errs=%d, want 2/1", len(rows), len(errs))
	}
	if rows[0].AmountMinor != 750000 || rows[0].Direction != "credit" {
		t.Errorf("row1 = %+v", rows[0])
	}
	if rows[1].AmountMinor != 25000 || rows[1].Direction != "debit" {
		t.Errorf("row2 = %+v", rows[1])
	}
}

func TestLooksLikeXLSX(t *testing.T) {
	if !LooksLikeXLSX("statement.XLSX") || LooksLikeXLSX("statement.csv") {
		t.Error("extension detection wrong")
	}
}

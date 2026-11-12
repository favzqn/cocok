package imports

import (
	"testing"
	"time"
)

func loc(t *testing.T) *time.Location {
	t.Helper()
	l, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatalf("load loc: %v", err)
	}
	return l
}

const sampleCSV = `Tanggal,Keterangan,Nominal,Jenis,Ref
2024-05-01,Transfer masuk Budi,1.500.000,masuk,TRX-1
2024-05-02,Payment invoice,-250000,keluar,TRX-2
bad-date,Row error,1000,masuk,TRX-3
2024-05-03,OK row,abc,masuk,TRX-4
2024-05-01,Transfer  masuk Budi,1.500.000,masuk,TRX-1
`

func TestParseCSV(t *testing.T) {
	m := Mapping{Date: "Tanggal", Amount: "Nominal", Description: "Keterangan", Direction: "Jenis", Reference: "Ref"}
	rows, errs, header, err := ParseCSV([]byte(sampleCSV), m, "IDR", loc(t))
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	if len(header) != 5 {
		t.Errorf("header = %v", header)
	}
	if len(rows) != 3 {
		t.Errorf("valid rows = %d, want 3 (%+v)", len(rows), rows)
	}
	if len(errs) != 2 {
		t.Errorf("row errors = %d, want 2 (%+v)", len(errs), errs)
	}
	// direction from column; sign folded
	if rows[0].Direction != "credit" || rows[0].AmountMinor != 1500000 {
		t.Errorf("row1 = %+v", rows[0])
	}
	if rows[1].Direction != "debit" || rows[1].AmountMinor != 250000 {
		t.Errorf("row2 = %+v", rows[1])
	}
	// rows 1 and 5 same content → same fingerprint (dedup candidate)
	if rows[0].Fingerprint != rows[2].Fingerprint {
		t.Errorf("identical rows differ:\n%s\n%s", rows[0].Fingerprint, rows[2].Fingerprint)
	}
	// different ref → different fingerprint
	if rows[0].Fingerprint == rows[1].Fingerprint {
		t.Error("different transactions share fingerprint")
	}
}

func TestParseCSVSignInfersDirection(t *testing.T) {
	csv := "date,amount\n2024-01-01,-500\n2024-01-02,500\n"
	m := Mapping{Date: "date", Amount: "amount"}
	rows, errs, _, err := ParseCSV([]byte(csv), m, "IDR", loc(t))
	if err != nil || len(errs) > 0 {
		t.Fatalf("err=%v errs=%v", err, errs)
	}
	if rows[0].Direction != "debit" || rows[0].AmountMinor != 500 {
		t.Errorf("negative → debit got %+v", rows[0])
	}
	if rows[1].Direction != "credit" || rows[1].AmountMinor != 500 {
		t.Errorf("positive → credit got %+v", rows[1])
	}
}

func TestParseCSVMissingColumn(t *testing.T) {
	m := Mapping{Date: "nope", Amount: "amount"}
	_, _, header, err := ParseCSV([]byte("date,amount\n2024-01-01,1\n"), m, "IDR", loc(t))
	if err == nil {
		t.Fatal("want error for missing column")
	}
	if len(header) != 2 {
		t.Errorf("header should still be returned, got %v", header)
	}
}

func TestFingerprintNormalization(t *testing.T) {
	base := ParsedRow{
		BookedAt:    time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC),
		Direction:   "credit",
		AmountMinor: 1000,
		Description: "  Transfer   Masuk  ",
	}
	a := Fingerprint(base)
	base.Description = "transfer masuk"
	b := Fingerprint(base)
	if a != b {
		t.Error("whitespace/case should normalize to same fingerprint")
	}
	base.ExternalRef = "TRX-9"
	if Fingerprint(base) == a {
		t.Error("external_ref must change fingerprint")
	}
}

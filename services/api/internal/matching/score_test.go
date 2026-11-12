package matching

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func tx(dir string, amount int64, day int, desc, cp, ref string) Tx {
	return Tx{
		ID:           uuid.New(),
		BookedAt:     time.Date(2024, 5, day, 0, 0, 0, 0, time.UTC),
		Direction:    dir,
		AmountMinor:  amount,
		Description:  desc,
		Counterparty: cp,
		ExternalRef:  ref,
	}
}

func fullWeights() Weights {
	return Weights{
		Amount: 0.35, Date: 0.20, Ref: 0.15, Counterparty: 0.15, Description: 0.15,
		MinScore: 0.6, AmountToleranceBP: 100, DateWindowDays: 7,
	}
}

func TestScorePerfectMatch(t *testing.T) {
	d := tx("debit", 1500000, 1, "Transfer ke Budi", "Budi Santoso", "TRX-1")
	c := tx("credit", 1500000, 2, "transfer ke  budi", "budi santoso", "TRX-1")
	res := Score([]Tx{d}, []Tx{c}, fullWeights())
	if res.Score < 0.9 {
		t.Errorf("perfect match score = %.3f, want >= 0.9", res.Score)
	}
	for k, v := range res.Components {
		if v < 0 || v > 1 {
			t.Errorf("component %s out of range: %f", k, v)
		}
	}
}

func TestScoreAmountMismatch(t *testing.T) {
	d := tx("debit", 1000000, 1, "x", "", "")
	c := tx("credit", 900000, 1, "y", "", "") // 10% off
	res := Score([]Tx{d}, []Tx{c}, fullWeights())
	if res.Components["amount"] != 0.9 {
		t.Errorf("amount component = %f, want 0.9", res.Components["amount"])
	}
	if res.Score >= 0.9 {
		t.Errorf("mismatch should score below exact: %.3f", res.Score)
	}
}

func TestScoreDateWindowDecay(t *testing.T) {
	d := tx("debit", 100, 1, "", "", "")
	c := tx("credit", 100, 8, "", "", "") // 7 days apart
	res := Score([]Tx{d}, []Tx{c}, fullWeights())
	if res.Components["date"] != 0 {
		t.Errorf("date at window edge = %f, want 0", res.Components["date"])
	}
}

func TestScoreRefDominates(t *testing.T) {
	w := fullWeights()
	d := tx("debit", 100, 1, "a", "", "REF-9")
	c := tx("credit", 100, 1, "b", "", "REF-9")
	res := Score([]Tx{d}, []Tx{c}, w)
	if res.Components["ref"] != 1 {
		t.Errorf("shared ref component = %f, want 1", res.Components["ref"])
	}
	c2 := tx("credit", 100, 1, "b", "", "REF-OTHER")
	res2 := Score([]Tx{d}, []Tx{c2}, w)
	if res2.Components["ref"] != 0 {
		t.Errorf("different refs component = %f, want 0", res2.Components["ref"])
	}
}

func TestScoreEmptyNeutral(t *testing.T) {
	res := Score([]Tx{tx("debit", 1, 1, "", "", "")}, []Tx{tx("credit", 1, 1, "", "", "")}, fullWeights())
	if res.Components["counterparty"] != 0.5 || res.Components["description"] != 0.5 {
		t.Errorf("empty cp/desc should be neutral: %v", res.Components)
	}
}

func TestScoreNilSides(t *testing.T) {
	res := Score(nil, []Tx{tx("credit", 1, 1, "", "", "")}, fullWeights())
	if res.Score != 0 {
		t.Errorf("empty debit side should score 0, got %f", res.Score)
	}
}

func TestFingerprintOrderIndependent(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	f1 := Fingerprint([]uuid.UUID{a, b}, nil)
	f2 := Fingerprint([]uuid.UUID{b, a}, nil)
	if f1 != f2 {
		t.Error("fingerprint must ignore member order")
	}
	if Fingerprint([]uuid.UUID{a}, []uuid.UUID{b}) == f1 {
		t.Error("credit/debit swap must differ")
	}
}

func TestKind(t *testing.T) {
	if Kind(1, 1) != "1:1" || Kind(3, 1) != "1:N" || Kind(1, 3) != "N:1" {
		t.Errorf("kind labels wrong: %s %s %s", Kind(1, 1), Kind(3, 1), Kind(1, 3))
	}
}

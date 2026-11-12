package platform

import "testing"

func TestParseAmount(t *testing.T) {
	cases := []struct {
		in, cur string
		want    int64
		wantErr bool
	}{
		{"1234", "IDR", 1234, false},
		{"1.234.567", "IDR", 1234567, false},
		{"Rp 1.234.567", "IDR", 1234567, false},
		{"1,234,567", "IDR", 1234567, false},
		{"-5000", "IDR", -5000, false},
		{"(5000)", "IDR", -5000, false},
		{"1,234.56", "USD", 123456, false},
		{"1.234,56", "USD", 123456, false},
		{"1.5", "USD", 150, false},
		{"1,23", "USD", 123, false},
		{"0", "IDR", 0, false},
		{"1.234", "USD", 123400, false}, // grouping, not 1.234
		{"", "IDR", 0, true},
		{"abc", "IDR", 0, true},
		{"1.2345", "USD", 0, true}, // too many decimals
	}
	for _, c := range cases {
		got, err := ParseAmount(c.in, c.cur)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseAmount(%q,%q) = %d, want error", c.in, c.cur, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseAmount(%q,%q) error: %v", c.in, c.cur, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseAmount(%q,%q) = %d, want %d", c.in, c.cur, got, c.want)
		}
	}
}

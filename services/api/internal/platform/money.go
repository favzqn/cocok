package platform

import (
	"fmt"
	"strings"
)

// Precision returns the number of decimal digits for a currency.
// IDR has none (rupiah is the minor unit); most others use 2.
func Precision(currency string) int {
	if strings.EqualFold(strings.TrimSpace(currency), "IDR") {
		return 0
	}
	return 2
}

// ParseAmount converts a human amount string into integer minor units.
// Accepts: "1234", "1,234.56", "1.234,56", "Rp 1.234.567", "-5000", "(5000)".
// Never uses float64.
func ParseAmount(raw string, currency string) (int64, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, fmt.Errorf("%w: empty amount", ErrValidation)
	}
	neg := false
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		neg = true
		s = s[1 : len(s)-1]
	}
	// Strip currency symbols/codes and spaces.
	s = strings.Map(func(r rune) rune {
		switch {
		case r >= '0' && r <= '9':
			return r
		case r == '.' || r == ',' || r == '-' || r == '+':
			return r
		default:
			return -1
		}
	}, s)
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	}
	s = strings.TrimPrefix(s, "+")
	if s == "" {
		return 0, fmt.Errorf("%w: invalid amount", ErrValidation)
	}

	dec := Precision(currency)
	whole, frac, err := splitDecimal(s, dec)
	if err != nil {
		return 0, err
	}

	// minor = whole * 10^dec + frac
	var minor int64
	for _, c := range whole {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("%w: invalid amount", ErrValidation)
		}
		minor = minor*10 + int64(c-'0')
	}
	mult := int64(1)
	for i := 0; i < dec; i++ {
		mult *= 10
	}
	if dec > 0 {
		// frac already right-padded to dec digits by splitDecimal.
		var f int64
		for _, c := range frac {
			f = f*10 + int64(c-'0')
		}
		minor = minor*mult + f
	}
	if neg {
		minor = -minor
	}
	return minor, nil
}

// splitDecimal normalizes a digits+[.,digits] string into (whole, frac).
// frac is padded/truncated to exactly dec digits. When dec == 0 every
// separator is a thousands separator.
func splitDecimal(s string, dec int) (string, string, error) {
	lastSep := -1
	lastChar := byte(0)
	for i := 0; i < len(s); i++ {
		if s[i] == '.' || s[i] == ',' {
			lastSep = i
			lastChar = s[i]
		}
	}
	if lastSep < 0 {
		return s, strings.Repeat("0", dec), nil
	}

	if dec == 0 {
		// IDR: separators are always grouping (1.234.567).
		return strings.NewReplacer(".", "", ",", "").Replace(s), "", nil
	}

	// If both '.' and ',' present, the last one is the decimal separator.
	hasDot := strings.ContainsRune(s, '.')
	hasComma := strings.ContainsRune(s, ',')
	var whole, frac string
	if hasDot && hasComma {
		if lastChar == '.' {
			whole = strings.ReplaceAll(s[:lastSep], ",", "")
			frac = s[lastSep+1:]
		} else {
			whole = strings.ReplaceAll(s[:lastSep], ".", "")
			frac = s[lastSep+1:]
		}
	} else {
		// Single separator type: grouping if it repeats or groups exactly 3
		// digits (1,234 / 1.234.567); decimal otherwise (1.5 / 1,234.56 → handled above).
		sepCount := strings.Count(s, string(lastChar))
		grouped := sepCount > 1 || (sepCount == 1 && len(s)-lastSep-1 == 3)
		// "1.234" with dec=2: ambiguous — treat 3 trailing digits as grouping
		// (common in exports: 1.234 means one thousand two hundred thirty-four
		// only when currency precision says so; for 2-decimal currencies a
		// single separator with exactly 3 trailing digits is grouping).
		if grouped && sepCount == 1 && len(s)-lastSep-1 != 3 {
			grouped = false
		}
		if grouped {
			whole = strings.ReplaceAll(s, string(lastChar), "")
			frac = ""
		} else {
			if lastSep == 0 {
				return "", "", fmt.Errorf("%w: invalid amount", ErrValidation)
			}
			whole = s[:lastSep]
			frac = s[lastSep+1:]
			// Drop grouping inside whole (e.g. "1.234,56" handled above; "1,234.56" too).
			// With a single separator type, whole has no further separators.
		}
	}

	// Remove any leftover grouping in whole.
	whole = strings.NewReplacer(".", "", ",", "").Replace(whole)
	if whole == "" {
		whole = "0"
	}
	if len(frac) > dec {
		// Too much precision — reject instead of silently truncating money.
		trim := strings.TrimRight(frac, "0")
		if len(trim) > dec {
			return "", "", fmt.Errorf("%w: too many decimal places", ErrValidation)
		}
	}
	if len(frac) < dec {
		frac = frac + strings.Repeat("0", dec-len(frac))
	} else if len(frac) > dec {
		frac = frac[:dec]
	}
	if frac == "" {
		frac = strings.Repeat("0", dec)
	}
	return whole, frac, nil
}

package gocrumbs

import "testing"

// Expected values: § 32a EStG 2026 evaluated with exact fractions (independent of
// the integer implementation), floored to full euros. Zone limits from the statute:
// 12,348 | 12,349-17,799 | 17,800-69,878 | 69,879-277,825 | 277,826+.
func TestTariffEuro(t *testing.T) {
	p, _ := ForYear(2026)
	cases := []struct {
		zve  int64
		want int64
		why  string
	}{
		{0, 0, "no income"},
		{12348, 0, "Grundfreibetrag"},
		{12349, 0, "zone 2 start: (914.51*0.0001+1400)*0.0001 = 0.14 -> 0"},
		{13000, 95, "zone 2"},
		{15000, 435, "zone 2"},
		{17799, 1034, "last euro of zone 2"},
		{17800, 1035, "first euro of zone 3"},
		{30000, 4217, "zone 3"},
		{50000, 10548, "zone 3"},
		{69878, 18213, "last euro of zone 3: 18213.4"},
		{69879, 18213, "first euro of zone 4: 0.42*69879-11135.63 = 18213.55"},
		{100000, 30864, "zone 4: 42000-11135.63 = 30864.37"},
		{277825, 105550, "last euro of zone 4: 105550.5"},
		{277826, 105551, "first euro of zone 5: 0.45*277826-19470.38 = 105551.32"},
		{300000, 115529, "zone 5: 135000-19470.38 = 115529.62"},
	}
	for _, c := range cases {
		if got := p.tariffEuro(c.zve); got != c.want {
			t.Errorf("tariffEuro(%d) = %d, want %d (%s)", c.zve, got, c.want, c.why)
		}
	}
}

// Splitting is defined as 2 x tariff(floor(zvE / 2)).
func TestTariffSplitting(t *testing.T) {
	p, _ := ForYear(2026)
	cases := []struct{ zve, want int64 }{
		{24696, 0},      // half = 12,348 (Grundfreibetrag) -> 0
		{26000, 190},    // half = 13,000 -> 95
		{60000, 8434},   // half = 30,000 -> 4,217
		{100000, 21096}, // half = 50,000 -> 10,548
		{100001, 21096}, // odd income: floor(50,000.5) = 50,000
	}
	for _, c := range cases {
		if got := p.tariffSplitting(c.zve); got != c.want {
			t.Errorf("tariffSplitting(%d) = %d, want %d", c.zve, got, c.want)
		}
	}
}

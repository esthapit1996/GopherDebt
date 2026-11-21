package gocrumbs

import "testing"

// Expected Midijob bases: BE = F*G + ((OG - G*F)/(OG - G)) * (AE - G) with
// G = 603, OG = 2,000, F = 0.6619, evaluated with exact fractions and rounded half up.
func TestMidijobBase(t *testing.T) {
	p, _ := ForYear(2026)
	cases := []struct{ ae, want int64 }{
		{60301, 39914},   // just above the Minijob limit: ~ F*G = 399.13
		{100000, 85406},  // 1,000.00
		{150000, 142703}, // 1,500.00
		{199999, 199999}, // slope is ~1.0 near the top
		{200000, 200000}, // continuous: BE = OG at the upper limit
	}
	for _, c := range cases {
		if got := p.midijobBase(c.ae); got != c.want {
			t.Errorf("midijobBase(%d) = %d, want %d", c.ae, got, c.want)
		}
	}
}

func TestMidijobBaseNeverExceedsGross(t *testing.T) {
	p, _ := ForYear(2026)
	for ae := p.minijobMax + 1; ae <= p.midijobMax; ae += 37 {
		if be := p.midijobBase(ae); be > ae || be <= 0 {
			t.Fatalf("midijobBase(%d) = %d out of (0, ae]", ae, be)
		}
	}
}

func TestKVRate(t *testing.T) {
	p, _ := ForYear(2026)
	// Contribution actually paid: 7.30% + 2.90% / 2 = 8.75%.
	if got := p.kvRate(290); got != 87_500 {
		t.Errorf("kvRate(2.90%%) = %d, want 87500", got)
	}
	if got := p.kvRate(0); got != 73_000 {
		t.Errorf("kvRate(0) = %d, want 73000", got)
	}
	// The PAP's Vorsorgepauschale rate stays at 7.00% + Zusatzbeitrag / 2.
	if got := p.kvsatzan(290); got != 84_500 {
		t.Errorf("kvsatzan(2.90%%) = %d, want 84500", got)
	}
}

// Employee PV rates in ppm. Outside Sachsen: 1.8% base, +0.6% childless, -0.25% per
// child from the 2nd. Sachsen (AOK/BMG tables): 2.90, 2.30, 2.05, 1.80, 1.55 %.
func TestPVRate(t *testing.T) {
	p, _ := ForYear(2026)
	cases := []struct {
		name     string
		pvs, pvz bool
		pva      int
		want     int64
	}{
		{"childless", false, true, 0, 24_000},
		{"childless under 23", false, false, 0, 18_000},
		{"1 child", false, false, 0, 18_000},
		{"2 children", false, false, 1, 15_500},
		{"3 children", false, false, 2, 13_000},
		{"4 children", false, false, 3, 10_500},
		{"5 children", false, false, 4, 8_000},
		{"Sachsen childless", true, true, 0, 29_000},
		{"Sachsen 1 child", true, false, 0, 23_000},
		{"Sachsen 2 children", true, false, 1, 20_500},
		{"Sachsen 3 children", true, false, 2, 18_000},
		{"Sachsen 4 children", true, false, 3, 15_500},
	}
	for _, c := range cases {
		if got := p.pvsatzan(c.pvs, c.pvz, c.pva); got != c.want {
			t.Errorf("%s: pvsatzan = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestContribBasesCeilings(t *testing.T) {
	p, _ := ForYear(2026)
	capRV, capKV := p.bbgRVAVMonth(), p.bbgKVPVMonth()
	if capRV != 845_000 || capKV != 581_250 {
		t.Fatalf("monthly ceilings = %d / %d, want 845000 / 581250", capRV, capKV)
	}
	cases := []struct {
		name         string
		gross        int64
		wantRV       int64
		wantKV       int64
		wantReduced  bool
		wantCapped   bool
		allowMidijob bool
	}{
		{"below both ceilings", 400_000, 400_000, 400_000, false, false, true},
		{"exactly at KV ceiling", 581_250, 581_250, 581_250, false, false, true},
		{"between ceilings", 700_000, 700_000, 581_250, false, true, true},
		{"exactly at RV ceiling", 845_000, 845_000, 581_250, false, true, true},
		{"above both ceilings", 1_000_000, 845_000, 581_250, false, true, true},
		{"midijob", 100_000, 85_406, 85_406, true, false, true},
		{"midijob not allowed (side job)", 100_000, 100_000, 100_000, false, false, false},
	}
	for _, c := range cases {
		b, reduced, capped := p.contribBases(c.gross, capRV, capKV, c.allowMidijob)
		if b.rv != c.wantRV || b.kv != c.wantKV || reduced != c.wantReduced || capped != c.wantCapped {
			t.Errorf("%s: got rv=%d kv=%d reduced=%v capped=%v", c.name, b.rv, b.kv, reduced, capped)
		}
	}
}

func TestEmployeeContribsWerkstudentPaysOnlyRV(t *testing.T) {
	p, _ := ForYear(2026)
	b := bases{rv: 200_000, kv: 200_000}
	full := p.employeeContribs(b, false, 290, 24_000)
	// 2,000.00: RV 9.3% = 186.00, AV 1.3% = 26.00, KV 8.75% = 175.00, PV 2.4% = 48.00
	if full != (contribs{rv: 18_600, av: 2_600, kv: 17_500, pv: 4_800}) {
		t.Errorf("full contribs = %+v", full)
	}
	werk := p.employeeContribs(b, true, 290, 24_000)
	if werk != (contribs{rv: 18_600}) {
		t.Errorf("werkstudent contribs = %+v, want RV only", werk)
	}
}

// Soli: 5.5% of the annual Lohnsteuer above the Freigrenze 20,350, but at most 11.9%
// of the excess (Milderungszone). Values in cents per year, exact fractions.
func TestSolzYear(t *testing.T) {
	p, _ := ForYear(2026)
	cases := []struct {
		jbmg, kztab, want int64
		why               string
	}{
		{20350, 1, 0, "at the Freigrenze"},
		{20351, 1, 11, "1 EUR over: 11.9% of 1 = 0.119 -> 11 cents"},
		{20364, 1, 166, "14 EUR over -> 1.66 EUR"},
		{21000, 1, 7735, "Milderungszone"},
		{30000, 1, 114835, "Milderungszone: 9,650 x 11.9% = 1,148.35"},
		{40000, 1, 220000, "full 5.5% of 40,000 = 2,200.00 is the smaller value"},
		{40700, 2, 0, "Steuerklasse III: Freigrenze doubles"},
		{40701, 2, 11, "Steuerklasse III: 1 EUR over"},
	}
	for _, c := range cases {
		if got := p.solzYear(c.jbmg, c.kztab); got != c.want {
			t.Errorf("solzYear(%d, %d) = %d, want %d (%s)", c.jbmg, c.kztab, got, c.want, c.why)
		}
	}
}

func TestSolzNeverAboveFivePointFivePercent(t *testing.T) {
	p, _ := ForYear(2026)
	for j := int64(0); j < 200_000; j += 13 {
		if got, max := p.solzYear(j, 1), j*11/2; got > max {
			t.Fatalf("solzYear(%d) = %d above 5.5%% (%d)", j, got, max)
		}
	}
}

package gocrumbs

import (
	"errors"
	"math"
	"slices"
	"testing"
)

// base is a fully insured, childless employee over 23 in NRW, class I, Zusatzbeitrag 2.9%.
func base(gross float64) Input {
	return Input{
		Year: 2026, WorkType: WorkFTE, GrossMain: gross, SteuerKlasse: 1,
		Bundesland: "NW", ZusatzbeitragPct: 2.9,
	}
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.0051 }

type want struct {
	rv, av, kv, pv, lst, soli, kist, net float64
}

func check(t *testing.T, name string, got Result, w want) {
	t.Helper()
	g := want{got.RV, got.AV, got.KV, got.PV, got.Lohnsteuer, got.Soli, got.Kirchensteuer, got.Net}
	if !near(g.rv, w.rv) || !near(g.av, w.av) || !near(g.kv, w.kv) || !near(g.pv, w.pv) ||
		!near(g.lst, w.lst) || !near(g.soli, w.soli) || !near(g.kist, w.kist) || !near(g.net, w.net) {
		t.Errorf("%s:\n got  %+v\n want %+v", name, g, w)
	}
}

// Expected values were derived independently (exact fractions, PAP 2026 rules) and the
// social-insurance arithmetic is shown per case.
func TestCalculateScenarios(t *testing.T) {
	t.Run("A: 4,000 class I", func(t *testing.T) {
		// RV 372.00, AV 52.00, KV 4000*8.75% = 350.00, PV 4000*2.4% = 96.00 (childless).
		// Lohnsteuer: annual 6,294 EUR -> 524.50/month. Net = 4000 - 870 - 524.50.
		got, err := Calculate(base(4000))
		if err != nil {
			t.Fatal(err)
		}
		check(t, "A", got, want{rv: 372, av: 52, kv: 350, pv: 96, lst: 524.50, net: 2605.50})
		if !slices.Contains(got.Notes, NoteChildlessSurcharge) {
			t.Errorf("notes %v lack %s", got.Notes, NoteChildlessSurcharge)
		}
	})

	t.Run("under 23 and childless: no PV surcharge", func(t *testing.T) {
		in := base(4000)
		in.Under23 = true
		got, _ := Calculate(in)
		if !near(got.PV, 72) { // 4000 * 1.8%
			t.Errorf("PV = %v, want 72", got.PV)
		}
	})

	t.Run("D: above both ceilings, church tax in Bayern (8%)", func(t *testing.T) {
		// RV 8450*9.3% = 785.85, AV 8450*1.3% = 109.85,
		// KV 5812.50*8.75% = 508.59375 -> 508.59, PV 5812.50*2.4% = 139.50.
		in := base(10000)
		in.Bundesland = "BY"
		in.ChurchTax = true
		got, _ := Calculate(in)
		// Lohnsteuer 31,593/yr -> 2,632.75; Soli 11.15/month; Kirchensteuer 8% of 2,632.75 = 210.62.
		check(t, "D", got, want{rv: 785.85, av: 109.85, kv: 508.59, pv: 139.50,
			lst: 2632.75, soli: 111.49, kist: 210.62, net: 5501.35})
		if !slices.Contains(got.Notes, NoteBBGCapped) {
			t.Errorf("notes %v lack %s", got.Notes, NoteBBGCapped)
		}
	})

	t.Run("E: class III with 1 child", func(t *testing.T) {
		// 1 child: no PV surcharge, no discount -> PV 1.8% = 90.00.
		// RV 465.00, AV 65.00, KV 5000*8.75% = 437.50. Lohnsteuer 4,916/yr -> 409.66.
		in := base(5000)
		in.SteuerKlasse = 3
		in.Children = 1
		got, _ := Calculate(in)
		check(t, "E", got, want{rv: 465, av: 65, kv: 437.50, pv: 90, lst: 409.66, net: 3532.84})
	})

	t.Run("F: Sachsen, 3 children, class IV", func(t *testing.T) {
		// Sachsen 3 children: PV 2.3% - 2*0.25% = 1.8% = 54.00.
		in := base(3000)
		in.SteuerKlasse = 4
		in.Children = 3
		in.Bundesland = "SN"
		got, _ := Calculate(in)
		// RV 279.00, AV 39.00, KV 3000*8.75% = 262.50. Lohnsteuer 3,576/yr -> 298.00.
		check(t, "F", got, want{rv: 279, av: 39, kv: 262.50, pv: 54, lst: 298, net: 2067.50})
	})

	t.Run("Minijob: no deductions", func(t *testing.T) {
		got, _ := Calculate(base(500))
		check(t, "minijob", got, want{net: 500})
		if !slices.Contains(got.Notes, NoteMinijobMain) {
			t.Errorf("notes %v lack %s", got.Notes, NoteMinijobMain)
		}
	})

	t.Run("exactly at the Minijob limit is still a Minijob", func(t *testing.T) {
		got, _ := Calculate(base(603))
		check(t, "603", got, want{net: 603})
	})

	t.Run("Midijob for FTE: reduced base for all four branches", func(t *testing.T) {
		got, _ := Calculate(base(1000))
		// Employee base 20/13.97 * (1000 - 603) = 568.36: RV 52.86, AV 7.39, KV 49.73.
		// PV 568.36 * 1.8% + childless surcharge on the Gesamtbeitrag base 854.06 * 0.6% = 15.35.
		if !near(got.RV, 52.86) || !near(got.AV, 7.39) || !near(got.KV, 49.73) || !near(got.PV, 15.35) {
			t.Errorf("midijob contribs = %v %v %v %v", got.RV, got.AV, got.KV, got.PV)
		}
		if !slices.Contains(got.Notes, NoteMidijobReduced) {
			t.Errorf("notes %v lack %s", got.Notes, NoteMidijobReduced)
		}
	})
}

func TestCalculateWerkstudent(t *testing.T) {
	werk := func(gross float64) Input {
		in := base(gross)
		in.WorkType = WorkWerkstudent
		return in
	}
	t.Run("1,000: pension only, no tax", func(t *testing.T) {
		// Employee base 568.36 -> RV 52.86. Taxable income stays below the Grundfreibetrag.
		got, _ := Calculate(werk(1000))
		check(t, "werk1000", got, want{rv: 52.86, net: 947.14})
		if !slices.Contains(got.Notes, NoteWerkstudentRVOnly) {
			t.Errorf("notes %v lack %s", got.Notes, NoteWerkstudentRVOnly)
		}
	})
	t.Run("1,800: pension only, some Lohnsteuer", func(t *testing.T) {
		// Employee base 1,713.67 -> RV 159.37. Taxable 21,600 - 1,230 - 36 - 2,008 (pension part
		// only) = 18,325 -> Lohnsteuer 1,161/yr -> 96.75/month.
		got, _ := Calculate(werk(1800))
		check(t, "werk1800", got, want{rv: 159.37, lst: 96.75, net: 1543.88})
	})
	t.Run("above 2,000: full RV rate, still no KV/PV/AV", func(t *testing.T) {
		got, _ := Calculate(werk(2500))
		if !near(got.RV, 232.50) || got.AV != 0 || got.KV != 0 || got.PV != 0 { // 2500 * 9.3%
			t.Errorf("RV/AV/KV/PV = %v/%v/%v/%v", got.RV, got.AV, got.KV, got.PV)
		}
	})
	t.Run("side income is rejected", func(t *testing.T) {
		in := werk(1000)
		in.GrossSide = 100
		if _, err := Calculate(in); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("err = %v, want ErrInvalidInput", err)
		}
	})
}

func TestCalculateSideIncome(t *testing.T) {
	t.Run("up to 603 is a Minijob: net = gross", func(t *testing.T) {
		a, _ := Calculate(base(4000))
		in := base(4000)
		in.GrossSide = 400
		got, _ := Calculate(in)
		if !near(got.Net, a.Net+400) || !near(got.Gross, 4400) {
			t.Errorf("net = %v gross = %v, want %v / 4400", got.Net, got.Gross, a.Net+400)
		}
		if !slices.Contains(got.Notes, NoteMinijobSide) {
			t.Errorf("notes %v lack %s", got.Notes, NoteMinijobSide)
		}
	})
	t.Run("above 603 is a second job in class VI", func(t *testing.T) {
		// Side 1,000 in class VI: Lohnsteuer 1,341/yr -> 111.75; contributions on
		// the headroom under the ceilings: RV 93.00, AV 13.00, KV 87.50, PV 24.00.
		// Side net 670.75 on top of scenario A's net 2,605.50.
		in := base(4000)
		in.GrossSide = 1000
		got, _ := Calculate(in)
		if !near(got.Net, 2605.50+670.75) || !near(got.Lohnsteuer, 524.50+111.75) {
			t.Errorf("net = %v lst = %v, want 3276.25 / 636.25", got.Net, got.Lohnsteuer)
		}
		if !slices.Contains(got.Notes, NoteSideSecondJob) {
			t.Errorf("notes %v lack %s", got.Notes, NoteSideSecondJob)
		}
	})
	t.Run("side job shares the ceilings with the main job", func(t *testing.T) {
		// Main job already above both ceilings: no social insurance on the side job.
		in := base(10000)
		in.GrossSide = 1000
		got, _ := Calculate(in)
		main, _ := Calculate(base(10000))
		if got.RV != main.RV || got.AV != main.AV || got.KV != main.KV || got.PV != main.PV {
			t.Errorf("contribs changed with side income above the ceilings: %+v vs %+v", got, main)
		}
	})
}

func TestCalculateExpenses(t *testing.T) {
	in := base(4000)
	in.Rent = 900
	in.Savings = 300
	in.Subscriptions = []Expense{{Name: "Netflix", Amount: 12.99}, {Name: "Gym", Amount: 29.90}}
	in.Custom = []Expense{{Name: "Phone", Amount: 20}}
	got, _ := Calculate(in)
	// 900 + 300 + 42.89 + 20 = 1,262.89; left = 2,605.50 - 1,262.89 = 1,342.61
	e := got.Expenses
	if !near(e.Rent, 900) || !near(e.Savings, 300) || !near(e.Subscriptions, 42.89) || !near(e.Custom, 20) || !near(e.Total, 1262.89) {
		t.Errorf("expenses = %+v", e)
	}
	if !near(got.Left, 1342.61) {
		t.Errorf("left = %v, want 1342.61", got.Left)
	}
	if !near(got.SavingsRate, 300/2605.50) || !near(got.ExpenseRate, 1262.89/2605.50) {
		t.Errorf("rates = %v / %v", got.SavingsRate, got.ExpenseRate)
	}
}

func TestCalculateNegativeLeft(t *testing.T) {
	in := base(2000)
	in.Rent = 1500
	got, _ := Calculate(in)
	if got.Left >= 0 {
		t.Errorf("left = %v, want negative", got.Left)
	}
}

func TestCalculateNotes(t *testing.T) {
	in := base(3000)
	in.SteuerKlasse = 2
	got, _ := Calculate(in)
	if !slices.Contains(got.Notes, NoteClass2NoChildren) {
		t.Errorf("notes %v lack %s", got.Notes, NoteClass2NoChildren)
	}
	in.SteuerKlasse = 3
	got, _ = Calculate(in)
	if !slices.Contains(got.Notes, NoteClassPartner) {
		t.Errorf("notes %v lack %s", got.Notes, NoteClassPartner)
	}
	if got.Notes == nil {
		t.Error("Notes must be non-nil so JSON renders [] not null")
	}
}

func TestCalculateInvalidInput(t *testing.T) {
	cases := map[string]func(*Input){
		"unknown work type":  func(in *Input) { in.WorkType = "freelancer" },
		"class 0":            func(in *Input) { in.SteuerKlasse = 0 },
		"class 7":            func(in *Input) { in.SteuerKlasse = 7 },
		"unknown Bundesland": func(in *Input) { in.Bundesland = "XX" },
		"negative children":  func(in *Input) { in.Children = -1 },
		"too many children":  func(in *Input) { in.Children = 21 },
		"negative gross":     func(in *Input) { in.GrossMain = -1 },
		"huge gross":         func(in *Input) { in.GrossMain = 1e9 },
		"NaN gross":          func(in *Input) { in.GrossMain = math.NaN() },
		"Inf rent":           func(in *Input) { in.Rent = math.Inf(1) },
		"negative savings":   func(in *Input) { in.Savings = -5 },
		"Zusatzbeitrag 11%":  func(in *Input) { in.ZusatzbeitragPct = 11 },
		"negative sub":       func(in *Input) { in.Subscriptions = []Expense{{Amount: -1}} },
	}
	for name, mutate := range cases {
		in := base(3000)
		mutate(&in)
		if _, err := Calculate(in); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%s: err = %v, want ErrInvalidInput", name, err)
		}
	}
}

func TestCalculateUnknownYear(t *testing.T) {
	in := base(3000)
	in.Year = 2099
	if _, err := Calculate(in); err == nil {
		t.Fatal("want error for an unsupported tax year, got none")
	}
}

// Properties over gross 0..15,000 EUR in 1 EUR steps, for every Steuerklasse.
func TestCalculateProperties(t *testing.T) {
	for stkl := 1; stkl <= 6; stkl++ {
		var prevNet float64
		for g := 0; g <= 15000; g++ {
			in := base(float64(g))
			in.SteuerKlasse = stkl
			in.Rent = 500
			r, err := Calculate(in)
			if err != nil {
				t.Fatal(err)
			}
			lines := []float64{r.RV, r.AV, r.KV, r.PV, r.Lohnsteuer, r.Soli, r.Kirchensteuer}
			for _, l := range lines {
				if l < 0 {
					t.Fatalf("class %d gross %d: negative line %v", stkl, g, lines)
				}
			}
			sum := lines[0] + lines[1] + lines[2] + lines[3] + lines[4] + lines[5] + lines[6]
			if !near(r.TotalDeductions, sum) {
				t.Fatalf("class %d gross %d: deductions %v != sum %v", stkl, g, r.TotalDeductions, sum)
			}
			if r.Net > r.Gross+1e-9 || r.Net < 0 {
				t.Fatalf("class %d gross %d: net %v outside [0, gross]", stkl, g, r.Net)
			}
			if !near(r.Left, r.Net-500) {
				t.Fatalf("class %d gross %d: left %v != net - expenses", stkl, g, r.Left)
			}
			// Net may not fall as gross rises, except across the Minijob limit where
			// the law itself creates a cliff (603.00 -> 604).
			if g > 604 && r.Net < prevNet-1e-9 {
				t.Fatalf("class %d: net fell from %v to %v at gross %d", stkl, prevNet, r.Net, g)
			}
			prevNet = r.Net
		}
	}
}

// A Werkstudent pays no care or health insurance, so the notes about the care insurance
// surcharge and the KV ceiling must not appear.
func TestCalculateWerkstudentNotes(t *testing.T) {
	in := base(7000) // above the KV ceiling (5,812.50) but below the RV ceiling (8,450)
	in.WorkType = WorkWerkstudent
	got, _ := Calculate(in)
	for _, code := range []string{NoteChildlessSurcharge, NoteBBGCapped} {
		if slices.Contains(got.Notes, code) {
			t.Errorf("werkstudent notes %v must not contain %s", got.Notes, code)
		}
	}
	if !slices.Contains(got.Notes, NoteWerkstudentRVOnly) {
		t.Errorf("werkstudent notes %v lack %s", got.Notes, NoteWerkstudentRVOnly)
	}

	// In the Midijob range the base is reduced, which is not a ceiling
	in.GrossMain = 1000
	got, _ = Calculate(in)
	if slices.Contains(got.Notes, NoteBBGCapped) {
		t.Errorf("werkstudent at 1,000: notes %v must not contain %s", got.Notes, NoteBBGCapped)
	}

	// Above the RV ceiling the pension contribution really is capped
	in.GrossMain = 9000
	got, _ = Calculate(in)
	if !slices.Contains(got.Notes, NoteBBGCapped) {
		t.Errorf("werkstudent at 9,000: notes %v lack %s", got.Notes, NoteBBGCapped)
	}
}

// A real October 2026 payslip (class I, childless, no church tax, Berlin, TK with
// Zusatzbeitrag 2.69%): every contribution and the Lohnsteuer must match to the cent.
func TestCalculateMatchesPayslip(t *testing.T) {
	in := base(4916.67)
	in.Bundesland = "BE"
	in.ZusatzbeitragPct = 2.69
	got, err := Calculate(in)
	if err != nil {
		t.Fatal(err)
	}
	// KV 4916.67*(7.3%+1.345%) = 425.05, RV 9.3% = 457.25, AV 1.3% = 63.92, PV 2.4% = 118.00.
	check(t, "payslip", got, want{rv: 457.25, av: 63.92, kv: 425.05, pv: 118, lst: 761.58, net: 3090.87})
}

// The same payslip with its on-call lines (taxable 500.00 and tax-free 150.00): total
// Lohnsteuer 899.66, social insurance 1,172.44 and payout 3,494.57 as printed.
func TestCalculateExtraPayMatchesPayslip(t *testing.T) {
	in := base(4916.67)
	in.Bundesland = "BE"
	in.ZusatzbeitragPct = 2.69
	in.Extra = []ExtraPay{{Name: "On-call", Amount: 500}, {Name: "On-call Sunday", Amount: 150, TaxFree: true}}
	got, err := Calculate(in)
	if err != nil {
		t.Fatal(err)
	}
	// Taxable gross 5,416.67: KV 468.27, RV 503.75, AV 70.42, PV 130.00.
	check(t, "payslip+extra", got, want{rv: 503.75, av: 70.42, kv: 468.27, pv: 130, lst: 899.66, net: 3494.57})
	if !near(got.Gross, 5566.67) || !near(got.ExtraTaxable, 500) || !near(got.ExtraFree, 150) {
		t.Errorf("gross %v extra taxable %v free %v, want 5566.67 / 500 / 150", got.Gross, got.ExtraTaxable, got.ExtraFree)
	}
	if !near(got.TotalDeductions, 899.66+1172.44) {
		t.Errorf("deductions = %v, want %v", got.TotalDeductions, 899.66+1172.44)
	}
}

func TestCalculateExtraPay(t *testing.T) {
	t.Run("taxable extra equals the same amount of salary", func(t *testing.T) {
		a := base(4500)
		b := base(4000)
		b.Extra = []ExtraPay{{Name: "On-call", Amount: 500}}
		ra, _ := Calculate(a)
		rb, _ := Calculate(b)
		if ra.Net != rb.Net || ra.Lohnsteuer != rb.Lohnsteuer || ra.KV != rb.KV || ra.Gross != rb.Gross {
			t.Errorf("salary %+v vs extra %+v", ra, rb)
		}
	})
	t.Run("tax-free extra is paid out in full and changes no deduction", func(t *testing.T) {
		a, _ := Calculate(base(4000))
		in := base(4000)
		in.Extra = []ExtraPay{{Name: "Sunday", Amount: 150, TaxFree: true}}
		got, _ := Calculate(in)
		if !near(got.Net, a.Net+150) || got.TotalDeductions != a.TotalDeductions || !near(got.Gross, 4150) {
			t.Errorf("net %v deductions %v gross %v, want %v / %v / 4150", got.Net, got.TotalDeductions, got.Gross, a.Net+150, a.TotalDeductions)
		}
		if !near(got.TaxRate, a.TaxRate) {
			t.Errorf("tax rate %v, want %v (tax-free pay is outside the taxable gross)", got.TaxRate, a.TaxRate)
		}
	})
	t.Run("extra pay counts towards left", func(t *testing.T) {
		in := base(4000)
		in.Rent = 1000
		in.Extra = []ExtraPay{{Name: "Sunday", Amount: 100, TaxFree: true}}
		got, _ := Calculate(in)
		if !near(got.Left, got.Net-1000) || !near(got.Net, 2605.50+100) {
			t.Errorf("net %v left %v", got.Net, got.Left)
		}
	})
	t.Run("negative or huge extra is rejected", func(t *testing.T) {
		for _, amount := range []float64{-1, 2e6} {
			in := base(4000)
			in.Extra = []ExtraPay{{Name: "x", Amount: amount}}
			if _, err := Calculate(in); !errors.Is(err, ErrInvalidInput) {
				t.Errorf("amount %v: err = %v, want ErrInvalidInput", amount, err)
			}
		}
	})
	t.Run("works for a Werkstudent", func(t *testing.T) {
		in := base(1500)
		in.WorkType = WorkWerkstudent
		in.Extra = []ExtraPay{{Name: "Bonus", Amount: 100, TaxFree: true}}
		got, err := Calculate(in)
		if err != nil || got.KV != 0 || !near(got.ExtraFree, 100) {
			t.Errorf("err %v result %+v", err, got)
		}
	})
}

// Two 2026 payslips of a Werkstudent (class I, Berlin, 17 EUR/hour, Midijob range). In June the
// monthly pay is 1,360.00: employee base 1,083.75 (printed as "Beitragspfl. Einnahme AN"), RV
// 100.79 and Lohnsteuer 14.91, which only a pension-only Vorsorgepauschale produces. The May
// payslip was corrected afterwards to 1,632.00: totals of both lines are RV 137.00 (base 1,473.16)
// and Lohnsteuer 14.91 + 46.59, payout 1,433.50.
func TestCalculateMatchesWerkstudentPayslips(t *testing.T) {
	werk := func(gross float64) Input {
		in := base(gross)
		in.Bundesland = "BE"
		in.WorkType = WorkWerkstudent
		return in
	}
	// Independent check of the taxable income: 16,320 - 1,230 - 36 - 9.3% * 16,320 = 13,536
	// -> tariff 179/yr -> 14.91/month.
	got, err := Calculate(werk(1360))
	if err != nil {
		t.Fatal(err)
	}
	check(t, "june", got, want{rv: 100.79, lst: 14.91, net: 1244.30})

	// 19,584 - 1,266 - 1,821 = 16,496 -> tariff 738/yr -> 61.50/month.
	got, _ = Calculate(werk(1632))
	check(t, "may", got, want{rv: 137.00, lst: 61.50, net: 1433.50})
}

// In the Midijob range the PV childless surcharge and the child discount are calculated on the
// Gesamtbeitrag base (§ 20 Abs. 2a Satz 1 SGB IV), the regular share on the employee's base
// (Satz 6). Expected values from exact fractions: S6(1000) = 568.36, S1(1000) = 854.06,
// S6(1500) = 1,284.18, S1(1500) = 1,427.03, S6(650) = 67.29, S1(650) = 452.98.
func TestCalculateMidijobPV(t *testing.T) {
	cases := []struct {
		name     string
		gross    float64
		children int
		land     string
		want     float64
	}{
		{"childless", 1000, 0, "NW", 15.35},                            // 568.36*1.8% + 854.06*0.6%
		{"childless higher in the range", 1500, 0, "NW", 31.68},        // 1284.18*1.8% + 1427.03*0.6%
		{"childless in Sachsen", 1000, 0, "SN", 18.20},                 // 568.36*2.3% + 854.06*0.6%
		{"3 children", 1000, 3, "NW", 5.96},                            // 568.36*1.8% - 854.06*0.5%
		{"5 children just above 603: never negative", 650, 5, "NW", 0}, // 67.29*1.8% - 452.98*1.0% < 0
	}
	for _, c := range cases {
		in := base(c.gross)
		in.Children = c.children
		in.Bundesland = c.land
		got, err := Calculate(in)
		if err != nil {
			t.Fatal(err)
		}
		if !near(got.PV, c.want) {
			t.Errorf("%s: PV = %v, want %v", c.name, got.PV, c.want)
		}
	}
}

// With no note applying (childless, under 23, regular income) Notes must be an empty list, not nil:
// nil became "notes": null in the API and crashed the page.
func TestCalculateNoNotesIsEmptyNotNil(t *testing.T) {
	in := base(4000)
	in.Under23 = true
	got, err := Calculate(in)
	if err != nil {
		t.Fatal(err)
	}
	if got.Notes == nil || len(got.Notes) != 0 {
		t.Errorf("Notes = %#v, want an empty non-nil slice", got.Notes)
	}
}

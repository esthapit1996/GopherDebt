package gocrumbs

import (
	"errors"
	"fmt"
	"math"
)

// Work types.
const (
	WorkFTE         = "fte"
	WorkWerkstudent = "werkstudent"
)

// Note codes returned in Result.Notes; the UI maps them to text.
const (
	NoteMinijobMain         = "minijob_main"
	NoteMinijobSide         = "minijob_side"
	NoteMidijobReduced      = "midijob_reduced"
	NoteBBGCapped           = "bbg_capped"
	NoteChildlessSurcharge  = "childless_surcharge"
	NoteClass2NoChildren    = "class_ii_without_children"
	NoteClassPartner        = "class_iii_iv_v_partner"
	NoteSideSecondJob       = "side_second_job_class_vi"
	NoteWerkstudentRVOnly   = "werkstudent_rv_only"
	NoteChildAllowanceShare = "child_allowance_shared"
)

// ErrInvalidInput wraps every input validation failure.
var ErrInvalidInput = errors.New("gocrumbs: invalid input")

// churchRate is the Kirchensteuer rate in percent per Bundesland.
var churchRate = map[string]int64{
	"BW": 8, "BY": 8,
	"BE": 9, "BB": 9, "HB": 9, "HH": 9, "HE": 9, "MV": 9, "NI": 9, "NW": 9,
	"RP": 9, "SL": 9, "SN": 9, "ST": 9, "SH": 9, "TH": 9,
}

// Expense is one named monthly expense line.
type Expense struct {
	Name     string
	Amount   float64
	Category string
}

// ExtraPay is a flat monthly payment on top of the salary, such as on-call pay (Rufbereitschaft).
// It is paid with the salary: taxable and subject to social insurance, unless TaxFree
// (e.g. Sunday and holiday surcharges under § 3b EStG), which is paid out in full.
type ExtraPay struct {
	Name    string
	Amount  float64
	TaxFree bool
}

// Input is everything the calculation needs. Money is EUR per month.
type Input struct {
	Year             int
	WorkType         string // WorkFTE or WorkWerkstudent
	GrossMain        float64
	GrossSide        float64 // FTE only
	Extra            []ExtraPay
	SteuerKlasse     int // 1-6
	ChurchTax        bool
	Bundesland       string // two-letter code
	Children         int
	Under23          bool // childless and under 23: no PV surcharge
	ZusatzbeitragPct float64

	Rent          float64
	Savings       float64
	Subscriptions []Expense
	Custom        []Expense
}

// ExpenseTotals are the monthly expenses in EUR.
type ExpenseTotals struct {
	Rent          float64 `json:"rent"`
	Savings       float64 `json:"savings"`
	Subscriptions float64 `json:"subscriptions"`
	Custom        float64 `json:"custom"`
	Total         float64 `json:"total"`
}

// Result is the full monthly breakdown in EUR. Rates are fractions (0.25 = 25%).
type Result struct {
	TaxYear int `json:"tax_year"`

	Gross           float64 `json:"gross"` // everything paid out before deductions, tax-free extras included
	RV              float64 `json:"rv"`
	AV              float64 `json:"av"`
	KV              float64 `json:"kv"`
	PV              float64 `json:"pv"`
	Lohnsteuer      float64 `json:"lohnsteuer"`
	Soli            float64 `json:"soli"`
	Kirchensteuer   float64 `json:"kirchensteuer"`
	ExtraTaxable    float64 `json:"extra_taxable"` // taxable extra pay (already part of the main job's gross)
	ExtraFree       float64 `json:"extra_free"`    // tax-free extra pay
	TotalDeductions float64 `json:"total_deductions"`
	Net             float64 `json:"net"`

	Expenses ExpenseTotals `json:"expenses"`
	Left     float64       `json:"left"`

	SavingsRate float64  `json:"savings_rate"`
	ExpenseRate float64  `json:"expense_rate"`
	TaxRate     float64  `json:"tax_rate"`
	Notes       []string `json:"notes"`
}

func toCents(eur float64) int64 { return int64(math.Round(eur * 100)) }
func euro(cents int64) float64  { return float64(cents) / 100 }

func validAmount(name string, v, max float64) error {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > max {
		return fmt.Errorf("%w: %s out of range", ErrInvalidInput, name)
	}
	return nil
}

func (in Input) validate() error {
	if in.WorkType != WorkFTE && in.WorkType != WorkWerkstudent {
		return fmt.Errorf("%w: work_type", ErrInvalidInput)
	}
	if in.SteuerKlasse < 1 || in.SteuerKlasse > 6 {
		return fmt.Errorf("%w: steuerklasse", ErrInvalidInput)
	}
	if _, ok := churchRate[in.Bundesland]; !ok {
		return fmt.Errorf("%w: bundesland", ErrInvalidInput)
	}
	if in.Children < 0 || in.Children > 20 {
		return fmt.Errorf("%w: children", ErrInvalidInput)
	}
	if in.WorkType == WorkWerkstudent && in.GrossSide > 0 {
		return fmt.Errorf("%w: side income not allowed for werkstudent", ErrInvalidInput)
	}
	checks := []struct {
		name string
		v    float64
		max  float64
	}{
		{"zusatzbeitrag", in.ZusatzbeitragPct, 10},
		{"gross_main", in.GrossMain, 1e6},
		{"gross_side", in.GrossSide, 1e6},
		{"rent", in.Rent, 1e6},
		{"savings", in.Savings, 1e6},
	}
	for _, c := range checks {
		if err := validAmount(c.name, c.v, c.max); err != nil {
			return err
		}
	}
	for _, e := range append(append([]Expense{}, in.Subscriptions...), in.Custom...) {
		if err := validAmount("expense amount", e.Amount, 1e6); err != nil {
			return err
		}
	}
	for _, e := range in.Extra {
		if err := validAmount("extra pay amount", e.Amount, 1e6); err != nil {
			return err
		}
	}
	return nil
}

// job is the payroll result of one employment in cents per month.
type job struct {
	c                contribs
	lst, solz, kist  int64
	minijob, midijob bool
	capped           bool
}

// jobCtx are the person-level settings shared by all jobs.
type jobCtx struct {
	werkstudent   bool
	church        bool
	churchPct     int64
	kvzHundredths int64
	pv            pvRates
	pva           int
	pvs, pvz      bool
}

// payslip computes deductions of one employment. capRV and capKV are the monthly
// ceilings still available for this job.
func (p *Params) payslip(gross int64, stkl int, zkfTenths int, freibJahr int64, capRV, capKV int64, allowMidijob bool, ctx jobCtx) job {
	if gross <= p.minijobMax {
		return job{minijob: gross > 0}
	}
	b, reduced, capped := p.contribBases(gross, capRV, capKV, allowMidijob)
	if ctx.werkstudent && !reduced {
		capped = b.rv < gross // only pension insurance is paid, so only its ceiling counts
	}
	out := job{
		c:       p.employeeContribs(b, ctx.werkstudent, ctx.kvzHundredths, ctx.pv),
		midijob: reduced,
		capped:  capped,
	}
	// A Werkstudent bears no KV/PV contributions through the employer, so the 2026
	// Vorsorgepauschale has no KV/PV part: PKV = 1 with no contributions (checked against
	// 2026 payslips, where only the pension part reduces the taxable income).
	res := p.pap(papIn{
		zre4j:         gross * 12,
		pkv:           ctx.werkstudent,
		stkl:          stkl,
		zkfTenths:     zkfTenths,
		freibJahr:     freibJahr,
		alvExempt:     ctx.werkstudent,
		kvzHundredths: ctx.kvzHundredths,
		pva:           ctx.pva,
		pvs:           ctx.pvs,
		pvz:           ctx.pvz,
		church:        ctx.church,
	})
	out.lst = res.lstMonth
	out.solz = res.solzMonth
	out.kist = roundHalfUp(res.bkMonth*ctx.churchPct, 100)
	return out
}

// Calculate computes net pay and what is left after expenses for one month.
func Calculate(in Input) (Result, error) {
	p, err := ForYear(in.Year)
	if err != nil {
		return Result{}, err
	}
	if err := in.validate(); err != nil {
		return Result{}, err
	}

	werk := in.WorkType == WorkWerkstudent
	pva := 0
	if in.Children >= 2 {
		pva = min(in.Children-1, 4)
	}
	pvz := in.Children == 0 && !in.Under23
	ctx := jobCtx{
		werkstudent:   werk,
		church:        in.ChurchTax,
		churchPct:     churchRate[in.Bundesland],
		kvzHundredths: int64(math.Round(in.ZusatzbeitragPct * 100)),
		pva:           pva,
		pvs:           in.Bundesland == "SN",
		pvz:           pvz,
	}
	ctx.pv = p.pvRatesFor(ctx.pvs, ctx.pvz, ctx.pva)

	// Kinderfreibetrag (affects Soli and Kirchensteuer base only). Classes I and IV
	// share the allowance with the other parent, II and III get it in full.
	zkfTenths := 0
	switch in.SteuerKlasse {
	case 1:
		zkfTenths = in.Children * 5
	case 2, 3, 4:
		zkfTenths = in.Children * 10
	}
	// Steuerklasse II: Entlastungsbetrag for every further child (ELStAM Freibetrag).
	var freibJahr int64
	if in.SteuerKlasse == 2 && in.Children > 1 {
		freibJahr = int64(in.Children-1) * p.entlastungWeiteresKind * 100
	}

	// Extra pay is paid with the salary, so taxable lines count as main-job gross (this is how
	// a payslip adds them); tax-free lines only add to the payout.
	var extraTaxable, extraFree int64
	for _, e := range in.Extra {
		if e.TaxFree {
			extraFree += toCents(e.Amount)
		} else {
			extraTaxable += toCents(e.Amount)
		}
	}
	mainGross := toCents(in.GrossMain) + extraTaxable
	main := p.payslip(mainGross, in.SteuerKlasse, zkfTenths, freibJahr,
		p.bbgRVAVMonth(), p.bbgKVPVMonth(), true, ctx)

	// Side income (FTE only): Minijob up to the limit, otherwise a second job in
	// Steuerklasse VI that shares the contribution ceilings with the main job.
	sideGross := toCents(in.GrossSide)
	headRV := p.bbgRVAVMonth() - min64(mainGross, p.bbgRVAVMonth())
	headKV := p.bbgKVPVMonth() - min64(mainGross, p.bbgKVPVMonth())
	side := p.payslip(sideGross, 6, 0, 0, headRV, headKV, false, ctx)

	grossTotal := mainGross + sideGross // taxable gross
	social := contribs{
		rv: main.c.rv + side.c.rv, av: main.c.av + side.c.av,
		kv: main.c.kv + side.c.kv, pv: main.c.pv + side.c.pv,
	}
	lst := main.lst + side.lst
	solz := main.solz + side.solz
	kist := main.kist + side.kist
	deductions := social.total() + lst + solz + kist
	net := grossTotal + extraFree - deductions

	subs, custom := sumExpenses(in.Subscriptions), sumExpenses(in.Custom)
	rent, savings := toCents(in.Rent), toCents(in.Savings)
	expenses := rent + savings + subs + custom

	res := Result{
		TaxYear:         p.Year,
		Gross:           euro(grossTotal + extraFree),
		RV:              euro(social.rv),
		AV:              euro(social.av),
		KV:              euro(social.kv),
		PV:              euro(social.pv),
		Lohnsteuer:      euro(lst),
		Soli:            euro(solz),
		Kirchensteuer:   euro(kist),
		ExtraTaxable:    euro(extraTaxable),
		ExtraFree:       euro(extraFree),
		TotalDeductions: euro(deductions),
		Net:             euro(net),
		Expenses: ExpenseTotals{
			Rent: euro(rent), Savings: euro(savings), Subscriptions: euro(subs),
			Custom: euro(custom), Total: euro(expenses),
		},
		Left:  euro(net - expenses),
		Notes: []string{},
	}
	if net > 0 {
		res.SavingsRate = float64(savings) / float64(net)
		res.ExpenseRate = float64(expenses) / float64(net)
	}
	if grossTotal > 0 {
		res.TaxRate = float64(lst+solz+kist) / float64(grossTotal)
	}
	res.Notes = notes(in, p, main, side, mainGross, sideGross, pvz)
	return res, nil
}

func sumExpenses(list []Expense) int64 {
	var total int64
	for _, e := range list {
		total += toCents(e.Amount)
	}
	return total
}

func notes(in Input, p *Params, main, side job, mainGross, sideGross int64, pvz bool) []string {
	out := []string{} // never nil: the API must send [] (the page reads notes.length)
	add := func(cond bool, code string) {
		if cond {
			out = append(out, code)
		}
	}
	add(main.minijob, NoteMinijobMain)
	add(side.minijob, NoteMinijobSide)
	add(main.midijob, NoteMidijobReduced)
	add(main.capped || side.capped, NoteBBGCapped)
	add(sideGross > p.minijobMax, NoteSideSecondJob)
	add(in.WorkType == WorkWerkstudent && mainGross > p.minijobMax, NoteWerkstudentRVOnly)
	add(pvz && mainGross > p.minijobMax && in.WorkType == WorkFTE, NoteChildlessSurcharge)
	add(in.SteuerKlasse == 2 && in.Children == 0, NoteClass2NoChildren)
	add(in.SteuerKlasse >= 3 && in.SteuerKlasse <= 5, NoteClassPartner)
	add(in.Children > 0 && (in.SteuerKlasse == 1 || in.SteuerKlasse == 4), NoteChildAllowanceShare)
	return out
}

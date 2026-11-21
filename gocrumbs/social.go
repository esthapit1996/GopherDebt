package gocrumbs

// contribs are employee social-insurance shares in cents for one month.
type contribs struct{ rv, av, kv, pv int64 }

func (c contribs) total() int64 { return c.rv + c.av + c.kv + c.pv }

// bases are the monthly contribution bases in cents: rv also applies to av, kv also
// applies to pv. pvAdj is the base of the PV childless surcharge and child discount: the
// same as kv, except in the Uebergangsbereich, where those are calculated on the
// Gesamtbeitrag base (§ 20 Abs. 2a Satz 1 SGB IV) instead of the employee's base.
type bases struct{ rv, kv, pvAdj int64 }

// pvRates split the employee PV rate (ppm) into the regular share and the adjustment for
// children: +surcharge when childless, -discount per child from the 2nd to the 5th.
type pvRates struct{ main, adj int64 }

// pvRatesFor returns the employee PV rates; main + adj equals the PAP's pvsatzan.
func (p *Params) pvRatesFor(pvs, pvz bool, pva int) pvRates {
	r := pvRates{main: p.pvBase}
	if pvs {
		r.main = p.pvSachsen
	}
	if pvz {
		r.adj = p.pvSurcharge
	} else {
		r.adj = -int64(pva) * p.pvDiscount
	}
	return r
}

// roundHalfUp divides non-negative num by den, rounding half up.
func roundHalfUp(num, den int64) int64 { return (num + den/2) / den }

// midijobBase is the beitragspflichtige Einnahme the employee's share is calculated on in
// the Uebergangsbereich (§ 20 Abs. 2a SGB IV), for gross ae in cents with
// minijobMax < ae <= midijobMax:
//
//	AE_AN = OG / (OG - G) * (AE - G)
//
// It grows from 0 at the Minijob limit to the full gross at OG. The factor F only
// matters for the total contribution (and so for the employer's share), not here.
// Confirmed against payslips ("Beitragspfl. Einnahme AN"): 1,360.00 in 2026 gives 1,083.75.
func (p *Params) midijobBase(ae int64) int64 {
	g, og := p.minijobMax, p.midijobMax
	return roundHalfUp(og*(ae-g), og-g)
}

// midijobTotalBase is the beitragspflichtige Einnahme of the Gesamtbeitrag in the
// Uebergangsbereich (§ 20 Abs. 2a Satz 1 SGB IV):
//
//	BE = F*G + ((OG - F*G) / (OG - G)) * (AE - G)
//
// The employee pays the PV childless surcharge on it alone, and the child discount is
// calculated on it the same way (DRV, Uebergangsbereich). Evaluated with one common
// denominator and a single rounding. 1,360.00 in 2026 gives 1,266.60, as on a payslip.
func (p *Params) midijobTotalBase(ae int64) int64 {
	g, og, f := p.minijobMax, p.midijobMax, p.midijobF
	num := f*g*(og-g) + (og*10_000-g*f)*(ae-g)
	return roundHalfUp(num, (og-g)*10_000)
}

// contribBases returns the contribution bases of a gross (cents/month) within the
// given ceilings. In the Uebergangsbereich the reduced Midijob base applies when
// allowed. capped reports that a ceiling cut the base.
func (p *Params) contribBases(gross, capRV, capKV int64, allowMidijob bool) (b bases, reduced, capped bool) {
	if allowMidijob && gross > p.minijobMax && gross <= p.midijobMax {
		be := p.midijobBase(gross)
		return bases{rv: be, kv: be, pvAdj: p.midijobTotalBase(gross)}, true, false
	}
	b = bases{rv: min64(gross, capRV), kv: min64(gross, capKV)}
	b.pvAdj = b.kv
	return b, false, b.rv < gross || b.kv < gross
}

// kvRate is the employee KV rate in ppm: 7.3% + Zusatzbeitrag / 2.
func (p *Params) kvRate(kvzHundredths int64) int64 {
	return p.kvEmployee + kvzHundredths*50
}

// employeeContribs computes the employee shares. rvOnly is the Werkstudent case:
// exempt from KV, PV and AV, but still paying Rentenversicherung.
// PV is the regular share on the kv base plus the children adjustment on the pvAdj base,
// rounded once. Outside the Uebergangsbereich both bases are equal, so this is the plain
// rate times the base. A child discount on the larger Satz 1 base could exceed the regular
// share just above the Minijob limit; the line then stays at 0 (assumption, see readme).
func (p *Params) employeeContribs(b bases, rvOnly bool, kvzHundredths int64, pv pvRates) contribs {
	c := contribs{rv: roundHalfUp(b.rv*p.rvEmployee, e6)}
	if rvOnly {
		return c
	}
	c.av = roundHalfUp(b.rv*p.avEmployee, e6)
	c.kv = roundHalfUp(b.kv*p.kvRate(kvzHundredths), e6)
	c.pv = roundHalfUp(max64(b.kv*pv.main+b.pvAdj*pv.adj, 0), e6)
	return c
}

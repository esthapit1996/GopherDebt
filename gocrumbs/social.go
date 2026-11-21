package gocrumbs

// contribs are employee social-insurance shares in cents for one month.
type contribs struct{ rv, av, kv, pv int64 }

func (c contribs) total() int64 { return c.rv + c.av + c.kv + c.pv }

// bases are the monthly contribution bases in cents: rv also applies to av,
// kv also applies to pv.
type bases struct{ rv, kv int64 }

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

// contribBases returns the contribution bases of a gross (cents/month) within the
// given ceilings. In the Uebergangsbereich the reduced Midijob base applies when
// allowed. capped reports that a ceiling cut the base.
func (p *Params) contribBases(gross, capRV, capKV int64, allowMidijob bool) (b bases, reduced, capped bool) {
	if allowMidijob && gross > p.minijobMax && gross <= p.midijobMax {
		be := p.midijobBase(gross)
		return bases{rv: be, kv: be}, true, false
	}
	b = bases{rv: min64(gross, capRV), kv: min64(gross, capKV)}
	return b, false, b.rv < gross || b.kv < gross
}

// kvRate is the employee KV rate in ppm: 7.3% + Zusatzbeitrag / 2.
func (p *Params) kvRate(kvzHundredths int64) int64 {
	return p.kvEmployee + kvzHundredths*50
}

// employeeContribs computes the employee shares. rvOnly is the Werkstudent case:
// exempt from KV, PV and AV, but still paying Rentenversicherung.
func (p *Params) employeeContribs(b bases, rvOnly bool, kvzHundredths, pvRate int64) contribs {
	c := contribs{rv: roundHalfUp(b.rv*p.rvEmployee, e6)}
	if rvOnly {
		return c
	}
	c.av = roundHalfUp(b.rv*p.avEmployee, e6)
	c.kv = roundHalfUp(b.kv*p.kvRate(kvzHundredths), e6)
	c.pv = roundHalfUp(b.kv*pvRate, e6)
	return c
}

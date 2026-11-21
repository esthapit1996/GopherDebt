package gocrumbs

// contribs are employee social-insurance shares in cents for one month.
type contribs struct{ rv, av, kv, pv int64 }

func (c contribs) total() int64 { return c.rv + c.av + c.kv + c.pv }

// bases are the monthly contribution bases in cents: rv also applies to av,
// kv also applies to pv.
type bases struct{ rv, kv int64 }

// roundHalfUp divides non-negative num by den, rounding half up.
func roundHalfUp(num, den int64) int64 { return (num + den/2) / den }

// midijobBase is the reduced beitragspflichtige Einnahme of the employee in the
// Uebergangsbereich (§ 20 Abs. 2a SGB IV), for gross ae in cents with
// minijobMax < ae <= midijobMax:
//
//	BE = F*G + ((OG - G*F) / (OG - G)) * (AE - G)
//
// Evaluated with one common denominator and a single rounding.
func (p *Params) midijobBase(ae int64) int64 {
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
		return bases{rv: be, kv: be}, true, false
	}
	b = bases{rv: min64(gross, capRV), kv: min64(gross, capKV)}
	return b, false, b.rv < gross || b.kv < gross
}

// employeeContribs computes the employee shares. rvOnly is the Werkstudent case:
// exempt from KV, PV and AV, but still paying Rentenversicherung.
func (p *Params) employeeContribs(b bases, rvOnly bool, kvzHundredths, pvRate int64) contribs {
	c := contribs{rv: roundHalfUp(b.rv*p.rvEmployee, e6)}
	if rvOnly {
		return c
	}
	c.av = roundHalfUp(b.rv*p.avEmployee, e6)
	c.kv = roundHalfUp(b.kv*p.kvsatzan(kvzHundredths), e6)
	c.pv = roundHalfUp(b.kv*pvRate, e6)
	return c
}

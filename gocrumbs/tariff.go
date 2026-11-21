package gocrumbs

const (
	e6  int64 = 1_000_000
	e10 int64 = 10_000_000_000
)

// tariffEuro is the tarifliche Einkommensteuer of § 32a EStG in whole euros for a
// zu versteuerndes Einkommen x in whole euros (already floored).
// All arithmetic is exact integer math, so the floors at zone boundaries cannot
// be hit by float error. With d = x - zone start (y = d / 10,000):
//
//	zone 2: floor((914.51*y + 1400)*y)        = floor((a*d + b*1e6)*d / 1e10)
//	zone 3: floor((173.10*z + 2397)*z + 1034.87)
//	zone 4: floor(0.42*x - 11135.63)
//	zone 5: floor(0.45*x - 19470.38)
func (p *Params) tariffEuro(x int64) int64 {
	switch {
	case x <= p.grundfreibetrag:
		return 0
	case x <= p.zone2End:
		d := x - p.grundfreibetrag
		return ((p.zone2A*d + p.zone2B*e6) * d) / e10
	case x <= p.zone3End:
		d := x - p.zone2End
		return ((p.zone3A*d+p.zone3B*e6)*d + p.zone3C*100_000_000) / e10
	case x <= p.zone4End:
		return (p.zone4Pct*x - p.zone4Sub) / 100
	default:
		return (p.zone5Pct*x - p.zone5Sub) / 100
	}
}

// tariffSplitting is the Splittingtarif: twice the tariff of half the income.
// zve is in whole euros (the joint income, already floored).
func (p *Params) tariffSplitting(zve int64) int64 {
	return 2 * p.tariffEuro(zve/2)
}

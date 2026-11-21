package gocrumbs

// papIn is the subset of the BMF PAP input parameters the engine uses.
// Ported from "Programmablaufplan 2026, Anlage 1" (12.11.2025): modules MPARA,
// MRE4ABZ, MZTABFB, MLSTJAHR, UPEVP, MVSPKVPV, MVSPHB, UPMLST, MST5-6, UPTAB26, MSOLZ.
// Not modelled: Versorgungsbezuege, Altersentlastungsbetrag, Faktorverfahren,
// sonstige Bezuege (one-off payments).
type papIn struct {
	zre4j     int64 // annual taxable wage in cents (RE4 * 12 for a monthly period)
	stkl      int   // Steuerklasse 1-6
	zkfTenths int   // ZKF (Zahl der Kinderfreibetraege) in tenths, e.g. 5 = 0.5
	freibJahr int64 // annual Freibetrag in cents (JLFREIB)

	krvExempt bool // KRV = 1: not RV-insured
	alvExempt bool // ALV = 1: not AV-insured
	pkv       bool // PKV = 1: privately health insured

	kvzHundredths int64 // Zusatzbeitrag in 1/100 percent (2.90% -> 290)
	pva           int   // Beitragsabschlaege PV (children 2..5)
	pvs           bool  // Sachsen
	pvz           bool  // childless surcharge

	pkpvMonth    int64 // private KV/PV contributions per month in cents (only if pkv)
	pkpvagzMonth int64 // tax-free employer subsidy per month in cents (only if pkv)

	church bool // member of a church (R > 0)
}

// papOut holds the PAP results for a monthly period.
type papOut struct {
	lstYear   int64 // annual Lohnsteuer in euros (LSTJAHR)
	lstMonth  int64 // LSTLZZ in cents
	solzMonth int64 // SOLZLZZ in cents
	bkMonth   int64 // BK: Bemessungsgrundlage Kirchenlohnsteuer in cents
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// kvsatzan is the employee KV rate in ppm: 7% + Zusatzbeitrag / 2.
func (p *Params) kvsatzan(kvzHundredths int64) int64 {
	return p.kvBase + kvzHundredths*50
}

// pvsatzan is the employee PV rate in ppm (PAP MPARA).
func (p *Params) pvsatzan(pvs, pvz bool, pva int) int64 {
	rate := p.pvBase
	if pvs {
		rate = p.pvSachsen
	}
	if pvz {
		return rate + p.pvSurcharge
	}
	return rate - int64(pva)*p.pvDiscount
}

// vorsorgepauschale is the annual Vorsorgepauschale in cents (UPEVP).
func (p *Params) vorsorgepauschale(in papIn) int64 {
	zre4vp := in.zre4j

	var vspr int64
	if !in.krvExempt {
		vspr = min64(zre4vp, p.bbgRVAVYear) * p.rvEmployee / e6
	}

	var vspkvpv int64
	if in.pkv {
		if in.stkl != 6 {
			vspkvpv = in.pkpvMonth*12 - in.pkpvagzMonth*12
		}
		vspkvpv = max64(vspkvpv, 0)
	} else {
		rates := p.kvsatzan(in.kvzHundredths) + p.pvsatzan(in.pvs, in.pvz, in.pva)
		vspkvpv = min64(zre4vp, p.bbgKVPVYear) * rates / e6
	}

	vsp := vspkvpv + vspr
	if in.alvExempt || in.stkl == 6 {
		return vsp
	}

	// MVSPHB: Hoechstbetragsberechnung incl. Arbeitslosenversicherung.
	vspalv := min64(zre4vp, p.bbgRVAVYear) * p.avEmployee / e6
	vsphb := min64(vspalv+vspkvpv, p.vspHoechstbetragAV)
	vspn := vspr + vsphb
	return max64(vsp, vspn)
}

// taxFromZVE turns a zu versteuerndes Einkommen (cents) into the annual
// tarifliche Steuer in whole euros (UPMLST + MST5-6).
func (p *Params) taxFromZVE(zve int64, stkl int, kztab int64) int64 {
	var x int64
	if zve >= 100 {
		x = zve / (100 * kztab)
	}
	if stkl < 5 {
		return p.tariffEuro(x) * kztab
	}
	return p.mst56(x)
}

// up56 is UP5-6: difference method with 1.25x and 0.75x income, at least 14%.
func (p *Params) up56(zx int64) int64 {
	st1 := p.tariffEuro(zx * 125 / 100)
	st2 := p.tariffEuro(zx * 75 / 100)
	diff := (st1 - st2) * 2
	mist := zx * 14 / 100
	return max64(diff, mist)
}

// mst56 is MST5-6: Lohnsteuer for Steuerklasse V and VI.
func (p *Params) mst56(zzx int64) int64 {
	if zzx > p.w2Stkl5 {
		st := p.up56(p.w2Stkl5)
		if zzx > p.w3Stkl5 {
			st += (p.w3Stkl5 - p.w2Stkl5) * 42 / 100
			st += (zzx - p.w3Stkl5) * 45 / 100
			return st
		}
		return st + (zzx-p.w2Stkl5)*42/100
	}
	st := p.up56(zzx)
	if zzx > p.w1Stkl5 {
		vergl := st
		st = p.up56(p.w1Stkl5)
		hoch := st + (zzx-p.w1Stkl5)*42/100
		if hoch < vergl {
			return hoch
		}
		return vergl
	}
	return st
}

// solzYear is MSOLZ: annual Solidaritaetszuschlag in cents from the annual
// Bemessungsgrundlage jbmg in whole euros.
func (p *Params) solzYear(jbmg, kztab int64) int64 {
	freigrenze := p.solzFreigrenze * kztab
	if jbmg <= freigrenze {
		return 0
	}
	full := jbmg * 11 / 2                  // 5.5% in cents
	mild := (jbmg - freigrenze) * 119 / 10 // 11.9% of the excess in cents
	return min64(full, mild)
}

// pap runs the PAP for one monthly (or annual, via zre4j) wage.
func (p *Params) pap(in papIn) papOut {
	zre4 := max64(in.zre4j-in.freibJahr, 0)

	// MZTABFB: fixed table allowances in cents.
	var anp, sap, efa, kfb int64
	kztab := int64(1)
	if in.stkl < 6 {
		anp = min64(zre4, p.arbeitnehmerPauschbetrag*100)
		sap = p.sonderausgabenPausch * 100
	}
	kfbPerZKF := p.kinderfreibetrag * 10 // cents per 0.1 ZKF
	switch in.stkl {
	case 1, 2, 3:
		kfb = int64(in.zkfTenths) * kfbPerZKF
	case 4:
		kfb = int64(in.zkfTenths) * kfbPerZKF / 2
	}
	switch in.stkl {
	case 2:
		efa = p.entlastungAlleinerz * 100
	case 3:
		kztab = 2
	}
	ztabfb := efa + anp + sap

	vsp := p.vorsorgepauschale(in)

	lstYear := p.taxFromZVE(zre4-ztabfb-vsp, in.stkl, kztab)

	// Bemessungsgrundlage for Soli and Kirchensteuer: tax after Kinderfreibetraege.
	jbmg := lstYear
	if in.zkfTenths > 0 {
		jbmg = p.taxFromZVE(zre4-ztabfb-kfb-vsp, in.stkl, kztab)
	}

	out := papOut{
		lstYear:   lstYear,
		lstMonth:  lstYear * 100 / 12,
		solzMonth: p.solzYear(jbmg, kztab) / 12,
	}
	if in.church {
		out.bkMonth = jbmg * 100 / 12
	}
	return out
}

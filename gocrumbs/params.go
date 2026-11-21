// Package gocrumbs computes German net pay (Lohnsteuer, Soli, Kirchensteuer and
// employee social insurance) from a gross salary and what is left after monthly
// expenses. Pure functions: no I/O, no database, no HTTP.
//
// Lohnsteuer follows the BMF Programmablaufplan (PAP) 2026, Anlage 1, dated
// 12.11.2025. Every tax-year constant lives in Params so a new year is a data
// change, not a code change.
package gocrumbs

import "fmt"

// Params holds all legal constants of one tax year.
// Money is in cents, rates in ppm (1e-6), unless the name says otherwise.
type Params struct {
	Year int

	// Social insurance.
	bbgRVAVYear int64 // Beitragsbemessungsgrenze RV/AV, cents per year
	bbgKVPVYear int64 // Beitragsbemessungsgrenze KV/PV, cents per year
	rvEmployee  int64 // employee share of Rentenversicherung
	avEmployee  int64 // employee share of Arbeitslosenversicherung
	kvEmployee  int64 // employee share of KV without Zusatzbeitrag (half of the 14.6% general rate)
	kvBase      int64 // KV rate the PAP uses for the Vorsorgepauschale (7%, without the Krankengeld part)
	pvBase      int64 // employee share of PV
	pvSachsen   int64 // employee share of PV in Sachsen
	pvSurcharge int64 // Zuschlag for childless
	pvDiscount  int64 // Abschlag per child from the 2nd up to the 5th

	// Minijob / Midijob (Uebergangsbereich).
	minijobMax int64 // cents per month
	midijobMax int64 // cents per month
	midijobF   int64 // Faktor F in 1e-4

	// Income tax, § 32a EStG tariff (zvE in whole euros).
	grundfreibetrag int64
	zone2End        int64
	zone3End        int64
	zone4End        int64
	zone2A, zone2B  int64 // (914.51*y + 1400)*y        -> a in 1e-2, b in euros
	zone3A, zone3B  int64 // (173.10*z + 2397)*z + 1034.87 -> a in 1e-2, b in euros
	zone3C          int64 // 1034.87 in 1e-2
	zone4Pct        int64 // 42 (%)
	zone4Sub        int64 // 11135.63 in 1e-2
	zone5Pct        int64 // 45 (%)
	zone5Sub        int64 // 19470.38 in 1e-2

	// PAP Lohnsteuer.
	arbeitnehmerPauschbetrag  int64 // euros
	sonderausgabenPausch      int64 // euros
	entlastungAlleinerz       int64 // euros, Steuerklasse II
	entlastungWeiteresKind    int64 // euros per additional child (ELStAM Freibetrag)
	kinderfreibetrag          int64 // euros per child, both parents
	w1Stkl5, w2Stkl5, w3Stkl5 int64 // limits for Steuerklasse V/VI, euros
	vspHoechstbetragAV        int64 // cents, cap for the AV/KV/PV part of the Vorsorgepauschale

	// Solidaritaetszuschlag.
	solzFreigrenze int64 // euros of annual Lohnsteuer (single)
}

var params2026 = Params{
	Year: 2026,

	bbgRVAVYear: 10_140_000, // 101,400 EUR (PAP MPARA BBGRVALV)
	bbgKVPVYear: 6_975_000,  // 69,750 EUR (PAP MPARA BBGKVPV)
	rvEmployee:  93_000,     // 9.30%
	avEmployee:  13_000,     // 1.30%
	kvEmployee:  73_000,     // 7.30%
	kvBase:      70_000,     // 7.00% (PAP KVSATZAN, Vorsorgepauschale only)
	pvBase:      18_000,     // 1.80%
	pvSachsen:   23_000,     // 2.30%
	pvSurcharge: 6_000,      // 0.60%
	pvDiscount:  2_500,      // 0.25%

	minijobMax: 60_300,  // 603.00 EUR
	midijobMax: 200_000, // 2,000.00 EUR
	// VERIFY: F = 0.28 / (18.6 + 2.6 + 14.6 + 2.9 + 3.6)% = 0.28 / 0.423 = 0.66194, published as 0.6619.
	midijobF: 6_619,

	grundfreibetrag: 12_348,
	zone2End:        17_799,
	zone3End:        69_878,
	zone4End:        277_825,
	zone2A:          91_451,
	zone2B:          1_400,
	zone3A:          17_310,
	zone3B:          2_397,
	zone3C:          103_487,
	zone4Pct:        42,
	zone4Sub:        1_113_563,
	zone5Pct:        45,
	zone5Sub:        1_947_038,

	arbeitnehmerPauschbetrag: 1_230,
	sonderausgabenPausch:     36,
	entlastungAlleinerz:      4_260,
	entlastungWeiteresKind:   240, // VERIFY: not part of the PAP, entered as ELStAM Freibetrag by the Finanzamt
	kinderfreibetrag:         9_756,
	w1Stkl5:                  14_071,
	w2Stkl5:                  34_939,
	w3Stkl5:                  222_260,
	vspHoechstbetragAV:       190_000, // 1,900 EUR

	solzFreigrenze: 20_350,
}

// SupportedYears lists the tax years the engine has parameters for.
func SupportedYears() []int { return []int{2026} }

// ForYear returns the parameters of a tax year, or an error for unknown years.
func ForYear(year int) (*Params, error) {
	switch year {
	case 2026:
		p := params2026
		return &p, nil
	}
	return nil, fmt.Errorf("gocrumbs: tax year %d not supported", year)
}

// bbgRVAVMonth is the monthly RV/AV ceiling in cents.
func (p *Params) bbgRVAVMonth() int64 { return p.bbgRVAVYear / 12 }

// bbgKVPVMonth is the monthly KV/PV ceiling in cents.
func (p *Params) bbgKVPVMonth() int64 { return p.bbgKVPVYear / 12 }

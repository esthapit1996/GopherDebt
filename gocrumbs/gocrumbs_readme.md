# 🧮 GoCrumbs engine (`gocrumbs/`)

Pure Go package that turns a German **brutto** salary into **netto** and shows what is left after monthly expenses. No I/O, no database, no HTTP, no AI: only integer maths. It imports nothing from the rest of the app.

Served by `handlers/gocrumbs.go` (`/api/gocrumbs`) and saved by `db/gocrumbs.go`.

> Estimates, not tax advice. Lohnsteuer is the **payslip withholding** (what the employer deducts), not the final Einkommensteuer.

## Files

| File | Purpose |
|------|---------|
| `params.go` | `Params` per tax year (`params2026`), `ForYear`, `SupportedYears` |
| `tariff.go` | § 32a EStG tariff and Splitting, exact integer arithmetic |
| `lohnsteuer.go` | Port of the BMF Programmablaufplan (PAP) 2026: Vorsorgepauschale, Steuerklassen I-VI, Soli, Kirchensteuer base |
| `social.go` | Employee RV/AV/KV/PV, contribution ceilings, Midijob reduced base, Werkstudent |
| `calc.go` | `Input`, `Result`, `Calculate` |
| `testdata/pap2026_pruef.json` | The BMF's own check tables (see Tests) |

## API

```go
res, err := gocrumbs.Calculate(gocrumbs.Input{
    Year: 2026, WorkType: gocrumbs.WorkFTE, GrossMain: 4000, SteuerKlasse: 1,
    Bundesland: "NW", ZusatzbeitragPct: 2.9, Rent: 900,
})
```

- Money is EUR per month. Inside the engine everything is `int64` cents; each contribution line is rounded once (half up). Lohnsteuer follows the PAP's own truncation.
- `Input.Extra` is flat monthly pay on top of the salary (e.g. on-call). Taxable lines are added to the main job's gross, which is how a payslip adds them; `TaxFree` lines (§ 3b EStG surcharges) only add to the payout. `Result.Gross` is everything paid out, `ExtraTaxable` and `ExtraFree` show the two sums.
- `Result.Notes` holds warning codes (`minijob_main`, `midijob_reduced`, `bbg_capped`, ...) for the UI to translate.
- Validation failures wrap `ErrInvalidInput`. An unsupported `Year` is an error: the engine never falls back to another year.

## What it models

| Situation | Rule |
|-----------|------|
| Gross up to €603/month | Minijob: net = gross, no deductions |
| €603.01 to €2,000 | Midijob: employee shares on the reduced base `OG/(OG - G) * (AE - G)` (G = 603, OG = 2,000) |
| Above the ceilings | RV/AV capped at €8,450/month, KV/PV at €5,812.50/month |
| Werkstudent | Only Rentenversicherung (reduced in the Uebergangsbereich); Lohnsteuer as normal |
| Side income (FTE only) | Up to €603: net = gross. Above: second job in Steuerklasse VI, social insurance on the headroom left under the ceilings |
| Children | PV Abschlaege from the 2nd child (up to the 5th); childless surcharge unless `Under23`; Kinderfreibetrag for Soli and Kirchensteuer |
| Sachsen | Employer pays 1.3% of PV, so the employee share is higher |
| Kirchensteuer | 8% in BW/BY, else 9%, on the Lohnsteuer after Kinderfreibetraege |

## Adding a tax year

1. Read the new BMF Programmablaufplan (published each November). Copy its `MPARA` constants, the `UPTAB` tariff and the allowances into a new `params20xx` value.
2. Add the year to `ForYear` and `SupportedYears`.
3. Replace `testdata/pap2026_pruef.json` with the new year's check tables (the extraction is just the two tables at the end of Anlage 1) and add the new year to the tests. Tests must pass before the year is enabled.

## Tests

`go test ./gocrumbs`

- **Official reference:** the BMF publishes two check tables with the PAP (annual gross 5,000-110,000 for every Steuerklasse, "allgemeine" and "besondere" Lohnsteuer). `lohnsteuer_test.go` runs all 516 values against the port. They match exactly.
- **Hand-derived:** tariff zone boundaries, Soli and Steuerklasse V/VI limits are computed with exact fractions from the statute, independent of the integer code. Scenario tests show the arithmetic in comments.
- **Properties:** for every Steuerklasse and gross 0-15,000 in 1 EUR steps: no negative lines, deductions = sum of lines, net within [0, gross], left = net - expenses, net never falls as gross rises (except across the Minijob limit, where the law itself has a cliff).

## Verified against the PAP vs. assumptions

Taken directly from the PAP 2026 (final, 12.11.2025): social-insurance ceilings and rates, PV rules incl. Sachsen, Vorsorgepauschale method, allowances (Arbeitnehmer-Pauschbetrag 1,230; Sonderausgaben 36; Entlastungsbetrag 4,260), Kinderfreibetrag 9,756 (4,878 in Steuerklasse IV), Steuerklasse V/VI limits, Soli Freigrenze 20,350 and the 11.9% Milderungszone.

**Checked against real payslips (to the cent, in `calc_test.go`):** an employee in 2026 with on-call pay (class I, childless, Zusatzbeitrag 2.69%), and a Werkstudent in the Midijob range in 2026 (RV, Lohnsteuer, payout). The Midijob base formula also reproduces a September 2025 payslip, and the Werkstudent RV above 2,000 matches an August 2025 one.

**Assumptions to verify before relying on them:**

- The extra Entlastungsbetrag for a 2nd child and beyond in Steuerklasse II (+240 EUR/child) is entered by the Finanzamt as an ELStAM Freibetrag, not part of the PAP.
- KV is paid at 7.3% + Zusatzbeitrag / 2. The PAP's 7% (no Krankengeld part) is only used inside the Vorsorgepauschale, not for the contribution.
- For a Werkstudent the Vorsorgepauschale only has the pension part (no AV, KV or PV part): a Werkstudent bears no KV/PV contributions through the employer. This is inferred from two 2026 payslips, which it reproduces to the cent; the PAP itself doesn't say so.
- Kirchensteuer is the Bemessungsgrundlage x rate, rounded half up. Contribution lines are rounded half up. Expect ±1 cent versus a payslip.
- Classes I and IV share the Kinderfreibetrag with the other parent (4,878 per child); II and III get 9,756.
- All children are treated as under 25 for the PV Abschlag.

**Not modelled:** Lohnsteuerkarte Freibetraege, Faktorverfahren, private health insurance, 13th salary and bonuses, Kappung of Kirchensteuer, the Midijob employer side, final Einkommensteuer and refunds.

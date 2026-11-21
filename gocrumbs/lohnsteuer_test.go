package gocrumbs

import (
	"encoding/json"
	"os"
	"testing"
)

type pruefFile struct {
	Allgemein []pruefRow `json:"allgemein"`
	Besonders []pruefRow `json:"besonders"`
}

type pruefRow struct {
	Gross int64   `json:"gross"`
	Lst   []int64 `json:"lst"` // Steuerklasse I..VI
}

func loadPruef(t *testing.T) pruefFile {
	t.Helper()
	raw, err := os.ReadFile("testdata/pap2026_pruef.json")
	if err != nil {
		t.Fatal(err)
	}
	var f pruefFile
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

// The BMF publishes two check tables (Pruefttabellen) with the PAP 2026: annual
// Lohnsteuer per Steuerklasse for annual gross 5,000..110,000. They are the
// official reference for this port.

// "Allgemeine" Lohnsteuer: fully insured employee (ALV = KRV = PKV = 0),
// Zusatzbeitrag 2.90%, PVZ = 0 in Steuerklasse II and 1 otherwise.
func TestPAP2026_PruefTabelleAllgemein(t *testing.T) {
	p, _ := ForYear(2026)
	checked := 0
	defer func() {
		if checked != 43*6 {
			t.Errorf("checked %d values, want %d", checked, 43*6)
		}
	}()
	for _, row := range loadPruef(t).Allgemein {
		for i, want := range row.Lst {
			checked++
			stkl := i + 1
			got := p.pap(papIn{
				zre4j:         row.Gross * 100,
				stkl:          stkl,
				kvzHundredths: 290,
				pvz:           stkl != 2,
			}).lstYear
			if got != want {
				t.Errorf("gross %d Steuerklasse %d: got %d, want %d", row.Gross, stkl, got, want)
			}
		}
	}
}

// "Besondere" Lohnsteuer: not insured in any branch (ALV = KRV = PKV = 1), private
// health insurance with PKPV 500 EUR/month in class III, 0 in class VI, else 300.
func TestPAP2026_PruefTabelleBesondere(t *testing.T) {
	p, _ := ForYear(2026)
	pkpv := map[int]int64{1: 30000, 2: 30000, 3: 50000, 4: 30000, 5: 30000, 6: 0}
	checked := 0
	defer func() {
		if checked != 43*6 {
			t.Errorf("checked %d values, want %d", checked, 43*6)
		}
	}()
	for _, row := range loadPruef(t).Besonders {
		for i, want := range row.Lst {
			checked++
			stkl := i + 1
			got := p.pap(papIn{
				zre4j:     row.Gross * 100,
				stkl:      stkl,
				krvExempt: true,
				alvExempt: true,
				pkv:       true,
				pkpvMonth: pkpv[stkl],
			}).lstYear
			if got != want {
				t.Errorf("gross %d Steuerklasse %d: got %d, want %d", row.Gross, stkl, got, want)
			}
		}
	}
}

// The BMF check tables stop at 110,000 gross, so the upper Steuerklasse V/VI limits
// are covered here with hand-derived values (exact fractions, PAP MST5-6):
//
//	zzx > W3 (222,260): up56(W2) + floor((W3-W2)*0.42) + floor((zzx-W3)*0.45)
//	e.g. 300,000: 10,448 + 78,674 + floor(77,740*0.45 = 34,983) = 124,105
func TestMst56Limits(t *testing.T) {
	p, _ := ForYear(2026)
	cases := []struct{ zzx, want int64 }{
		{14071, 1969},    // W1
		{14072, 1969},    // just above W1: min(hoch, vergl)
		{34939, 10448},   // W2
		{34940, 10448},   // above W2: + floor(0.42) = 0
		{222260, 89122},  // W3
		{222261, 89122},  // above W3: + floor(0.45) = 0
		{250000, 101605}, // 89,122 + floor(27,740*0.45 = 12,483)
		{300000, 124105},
	}
	for _, c := range cases {
		if got := p.mst56(c.zzx); got != c.want {
			t.Errorf("mst56(%d) = %d, want %d", c.zzx, got, c.want)
		}
	}
}

func TestForYear(t *testing.T) {
	for _, y := range SupportedYears() {
		p, err := ForYear(y)
		if err != nil || p.Year != y {
			t.Errorf("ForYear(%d) = %v, %v", y, p, err)
		}
	}
	if _, err := ForYear(1999); err == nil {
		t.Error("ForYear(1999) must fail")
	}
}

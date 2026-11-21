package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

const goCrumbsMinimal = `"work_type":"fte","gross_amount":4000,"gross_period":"month","steuerklasse":1,"bundesland":"NW","zusatzbeitrag":2.9`

func goCrumbsBody(extra string) string {
	if extra == "" {
		return `{` + goCrumbsMinimal + `}`
	}
	return `{` + goCrumbsMinimal + `,` + extra + `}`
}

func goCrumbsRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// nil DB: the validation and calculate paths must never touch the database
	h := NewGoCrumbsHandler(nil)
	r.POST("/gocrumbs/bind", func(c *gin.Context) {
		if req, ok := bindGoCrumbs(c); ok {
			c.JSON(http.StatusOK, req)
		}
	})
	r.PUT("/gocrumbs", h.SaveGoCrumbs)
	r.POST("/gocrumbs/calculate", h.CalculateGoCrumbs)
	return r
}

func doJSON(r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func manyLines(n int) string {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = `{"name":"x","amount":1}`
	}
	return "[" + strings.Join(lines, ",") + "]"
}

func TestBindGoCrumbs(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{"minimal", goCrumbsBody(""), http.StatusOK},
		{"full", goCrumbsBody(`"side_gross":300,"church_tax":true,"children":2,"under_23":false,"rent":900,"savings":250,` +
			`"subscriptions":[{"name":"Netflix","amount":12.99,"category":"media"}],"custom_expenses":[{"name":"Phone","amount":20}]`), http.StatusOK},
		{"empty body", `{}`, http.StatusBadRequest},
		{"missing work type", `{"gross_amount":1,"gross_period":"month","steuerklasse":1,"bundesland":"NW"}`, http.StatusBadRequest},
		{"unknown work type", strings.Replace(goCrumbsBody(""), `"fte"`, `"freelancer"`, 1), http.StatusBadRequest},
		{"unknown gross period", strings.Replace(goCrumbsBody(""), `"month"`, `"week"`, 1), http.StatusBadRequest},
		{"steuerklasse 0", strings.Replace(goCrumbsBody(""), `"steuerklasse":1`, `"steuerklasse":0`, 1), http.StatusBadRequest},
		{"steuerklasse 7", strings.Replace(goCrumbsBody(""), `"steuerklasse":1`, `"steuerklasse":7`, 1), http.StatusBadRequest},
		{"unknown bundesland", strings.Replace(goCrumbsBody(""), `"NW"`, `"XX"`, 1), http.StatusBadRequest},
		{"lowercase bundesland", strings.Replace(goCrumbsBody(""), `"NW"`, `"nw"`, 1), http.StatusBadRequest},
		{"negative gross", strings.Replace(goCrumbsBody(""), `"gross_amount":4000`, `"gross_amount":-1`, 1), http.StatusBadRequest},
		{"gross above limit", strings.Replace(goCrumbsBody(""), `"gross_amount":4000`, `"gross_amount":10000001`, 1), http.StatusBadRequest},
		{"negative side income", goCrumbsBody(`"side_gross":-5`), http.StatusBadRequest},
		{"negative children", goCrumbsBody(`"children":-1`), http.StatusBadRequest},
		{"21 children", goCrumbsBody(`"children":21`), http.StatusBadRequest},
		{"20 children", goCrumbsBody(`"children":20`), http.StatusOK},
		{"negative rent", goCrumbsBody(`"rent":-1`), http.StatusBadRequest},
		{"negative savings", goCrumbsBody(`"savings":-1`), http.StatusBadRequest},
		{"zusatzbeitrag 11", strings.Replace(goCrumbsBody(""), `"zusatzbeitrag":2.9`, `"zusatzbeitrag":11`, 1), http.StatusBadRequest},
		{"zusatzbeitrag 0", strings.Replace(goCrumbsBody(""), `"zusatzbeitrag":2.9`, `"zusatzbeitrag":0`, 1), http.StatusOK},
		{"empty expense name", goCrumbsBody(`"subscriptions":[{"name":"","amount":5}]`), http.StatusBadRequest},
		{"whitespace expense name", goCrumbsBody(`"custom_expenses":[{"name":"   ","amount":5}]`), http.StatusBadRequest},
		{"negative expense amount", goCrumbsBody(`"subscriptions":[{"name":"x","amount":-5}]`), http.StatusBadRequest},
		{"expense name at limit", goCrumbsBody(`"subscriptions":[{"name":"` + strings.Repeat("a", 60) + `","amount":5}]`), http.StatusOK},
		{"expense name at limit multibyte", goCrumbsBody(`"subscriptions":[{"name":"` + strings.Repeat("è", 60) + `","amount":5}]`), http.StatusOK},
		{"expense name too long", goCrumbsBody(`"subscriptions":[{"name":"` + strings.Repeat("a", 61) + `","amount":5}]`), http.StatusBadRequest},
		{"category too long", goCrumbsBody(`"subscriptions":[{"name":"x","amount":5,"category":"` + strings.Repeat("a", 31) + `"}]`), http.StatusBadRequest},
		{"50 lines in total", goCrumbsBody(`"subscriptions":` + manyLines(25) + `,"custom_expenses":` + manyLines(25)), http.StatusOK},
		{"51 lines in total", goCrumbsBody(`"subscriptions":` + manyLines(26) + `,"custom_expenses":` + manyLines(25)), http.StatusBadRequest},
		{"51 subscriptions", goCrumbsBody(`"subscriptions":` + manyLines(51)), http.StatusBadRequest},
		{"ws wage at limit", goCrumbsBody(`"ws_hourly_wage":1000`), http.StatusOK},
		{"ws wage too high", goCrumbsBody(`"ws_hourly_wage":1001`), http.StatusBadRequest},
		{"ws wage negative", goCrumbsBody(`"ws_hourly_wage":-1`), http.StatusBadRequest},
		{"ws hours per month at limit", goCrumbsBody(`"ws_hours":744`), http.StatusOK},
		{"ws hours per month too high", goCrumbsBody(`"ws_hours":745`), http.StatusBadRequest},
		{"ws hours per week at limit", goCrumbsBody(`"ws_hours_period":"week","ws_hours":168`), http.StatusOK},
		{"ws hours per week too high", goCrumbsBody(`"ws_hours_period":"week","ws_hours":169`), http.StatusBadRequest},
		{"ws hours negative", goCrumbsBody(`"ws_hours":-1`), http.StatusBadRequest},
		{"ws hours period invalid", goCrumbsBody(`"ws_hours_period":"year"`), http.StatusBadRequest},
		{"extra pay ok", goCrumbsBody(`"extra_pay":[{"name":"On-call","amount":500},{"name":"Sunday","amount":150,"tax_free":true}]`), http.StatusOK},
		{"extra pay empty name", goCrumbsBody(`"extra_pay":[{"name":"","amount":5}]`), http.StatusBadRequest},
		{"extra pay whitespace name", goCrumbsBody(`"extra_pay":[{"name":"  ","amount":5}]`), http.StatusBadRequest},
		{"extra pay negative", goCrumbsBody(`"extra_pay":[{"name":"x","amount":-1}]`), http.StatusBadRequest},
		{"extra pay too large", goCrumbsBody(`"extra_pay":[{"name":"x","amount":1000001}]`), http.StatusBadRequest},
		{"extra pay name too long", goCrumbsBody(`"extra_pay":[{"name":"` + strings.Repeat("a", 61) + `","amount":5}]`), http.StatusBadRequest},
		{"20 extra pay lines", goCrumbsBody(`"extra_pay":` + manyLines(20)), http.StatusOK},
		{"21 extra pay lines", goCrumbsBody(`"extra_pay":` + manyLines(21)), http.StatusBadRequest},
		{"extra pay omitted", goCrumbsBody(""), http.StatusOK},
		{"invalid JSON", `{`, http.StatusBadRequest},
	}
	r := goCrumbsRouter()
	for _, c := range cases {
		if w := doJSON(r, "POST", "/gocrumbs/bind", c.body); w.Code != c.want {
			t.Errorf("%s: status %d, want %d (%s)", c.name, w.Code, c.want, w.Body.String())
		}
	}
}

func TestBindGoCrumbsTrimsNames(t *testing.T) {
	w := doJSON(goCrumbsRouter(), "POST", "/gocrumbs/bind",
		goCrumbsBody(`"subscriptions":[{"name":"  Netflix  ","amount":5,"category":" media "}]`))
	var got struct {
		Subscriptions []struct{ Name, Category string } `json:"subscriptions"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || len(got.Subscriptions) != 1 {
		t.Fatalf("bad response %s (%v)", w.Body.String(), err)
	}
	if got.Subscriptions[0].Name != "Netflix" || got.Subscriptions[0].Category != "media" {
		t.Errorf("not trimmed: %+v", got.Subscriptions[0])
	}
}

func TestBindGoCrumbsTrimsExtraPayNames(t *testing.T) {
	w := doJSON(goCrumbsRouter(), "POST", "/gocrumbs/bind",
		goCrumbsBody(`"extra_pay":[{"name":"  On-call  ","amount":500}]`))
	var got struct {
		ExtraPay []struct {
			Name    string
			TaxFree bool `json:"tax_free"`
		} `json:"extra_pay"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || len(got.ExtraPay) != 1 {
		t.Fatalf("bad response %s (%v)", w.Body.String(), err)
	}
	if got.ExtraPay[0].Name != "On-call" || got.ExtraPay[0].TaxFree {
		t.Errorf("got %+v, want trimmed name and taxable by default", got.ExtraPay[0])
	}
}

type goCrumbsResponse struct {
	Success bool `json:"success"`
	Data    struct {
		TaxYear      int      `json:"tax_year"`
		Gross        float64  `json:"gross"`
		ExtraTaxable float64  `json:"extra_taxable"`
		ExtraFree    float64  `json:"extra_free"`
		Net          float64  `json:"net"`
		Left         float64  `json:"left"`
		Notes        []string `json:"notes"`
	} `json:"data"`
	Error string `json:"error"`
}

func calcResponse(t *testing.T, body string) (int, goCrumbsResponse) {
	t.Helper()
	w := doJSON(goCrumbsRouter(), "POST", "/gocrumbs/calculate", body)
	var resp goCrumbsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad JSON %q: %v", w.Body.String(), err)
	}
	return w.Code, resp
}

// 4,000 gross class I, childless, Zusatzbeitrag 2.9%: net 2,617.50 (gocrumbs scenario A).
func TestCalculateGoCrumbsEndToEnd(t *testing.T) {
	code, resp := calcResponse(t, goCrumbsBody(`"rent":900,"savings":300`))
	if code != http.StatusOK || !resp.Success {
		t.Fatalf("status %d: %+v", code, resp)
	}
	if resp.Data.TaxYear != 2026 || resp.Data.Gross != 4000 || resp.Data.Net != 2605.5 || resp.Data.Left != 1405.5 {
		t.Errorf("got %+v, want year 2026, gross 4000, net 2605.50, left 1405.50", resp.Data)
	}
	if len(resp.Data.Notes) == 0 || resp.Data.Notes[0] != "childless_surcharge" {
		t.Errorf("notes = %v, want childless_surcharge", resp.Data.Notes)
	}
}

func TestCalculateGoCrumbsYearlyGross(t *testing.T) {
	// 48,000 per year is the same as 4,000 per month
	body := strings.Replace(goCrumbsBody(""), `"gross_amount":4000,"gross_period":"month"`, `"gross_amount":48000,"gross_period":"year"`, 1)
	code, resp := calcResponse(t, body)
	if code != http.StatusOK || resp.Data.Net != 2605.5 {
		t.Errorf("status %d net %v, want 200 / 2605.50", code, resp.Data.Net)
	}
}

func TestCalculateGoCrumbsEngineRejections(t *testing.T) {
	// Passes the binding rules but not the engine's: 2,000,000 per month is above its range.
	cases := map[string]string{
		"monthly gross above range": strings.Replace(goCrumbsBody(""), `"gross_amount":4000`, `"gross_amount":2000000`, 1),
	}
	for name, body := range cases {
		code, resp := calcResponse(t, body)
		if code != http.StatusBadRequest || resp.Success || resp.Error == "" {
			t.Errorf("%s: status %d resp %+v, want 400 with an error", name, code, resp)
		}
	}
}

// Save validates before it touches the database (the handler here has a nil DB)
func TestSaveGoCrumbsRejectsInvalidBeforeDB(t *testing.T) {
	r := goCrumbsRouter()
	bodies := []string{
		`{}`,
		goCrumbsBody(`"ws_hours_period":"week","ws_hours":169`),
		goCrumbsBody(`"subscriptions":[{"name":" ","amount":5}]`),
	}
	for _, body := range bodies {
		if w := doJSON(r, "PUT", "/gocrumbs", body); w.Code != http.StatusBadRequest {
			t.Errorf("body %s: status %d, want 400", body, w.Code)
		}
	}
}

func TestGoCrumbsTaxYear(t *testing.T) {
	at := func(y int) time.Time { return time.Date(y, time.June, 1, 0, 0, 0, 0, time.UTC) }
	cases := map[int]int{
		2025: 2026, // before the first supported year: use the first
		2026: 2026,
		2027: 2026, // newer years fall back to the latest supported year, shown in the result
		2040: 2026,
	}
	for now, want := range cases {
		if got := goCrumbsTaxYear(at(now)); got != want {
			t.Errorf("goCrumbsTaxYear(%d) = %d, want %d", now, got, want)
		}
	}
}

// 4,000 gross plus a taxable 500 on-call line equals 4,500 gross; a tax-free 150 is added to the payout.
func TestCalculateGoCrumbsExtraPay(t *testing.T) {
	_, salary := calcResponse(t, strings.Replace(goCrumbsBody(""), `"gross_amount":4000`, `"gross_amount":4500`, 1))
	code, resp := calcResponse(t, goCrumbsBody(`"extra_pay":[{"name":"On-call","amount":500},{"name":"Sunday","amount":150,"tax_free":true}]`))
	if code != http.StatusOK || !resp.Success {
		t.Fatalf("status %d: %+v", code, resp)
	}
	if resp.Data.Net != salary.Data.Net+150 || resp.Data.Gross != 4650 || resp.Data.ExtraTaxable != 500 || resp.Data.ExtraFree != 150 {
		t.Errorf("got %+v, want net %v, gross 4650, extras 500 / 150", resp.Data, salary.Data.Net+150)
	}
}

func TestBindGoCrumbsDefaultsHoursPeriod(t *testing.T) {
	w := doJSON(goCrumbsRouter(), "POST", "/gocrumbs/bind", goCrumbsBody(`"ws_hours":80`))
	var got struct {
		WsHoursPeriod string `json:"ws_hours_period"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || got.WsHoursPeriod != "month" {
		t.Errorf("ws_hours_period = %q (%v), want month", got.WsHoursPeriod, err)
	}
}

// werkstudentBody is a Werkstudent profile (class I, Berlin, Zusatzbeitrag 2.69%). The employee
// fields (gross, side income, extra pay) are set to values that must not count.
func werkstudentBody(extra string) string {
	body := `{"work_type":"werkstudent","gross_amount":4916.67,"gross_period":"month","side_gross":500,` +
		`"extra_pay":[{"name":"On-call","amount":500}],"steuerklasse":1,"bundesland":"BE","zusatzbeitrag":2.69`
	if extra != "" {
		body += "," + extra
	}
	return body + "}"
}

// A June 2026 Werkstudent payslip: 80 hours at 17.00 = 1,360.00 gross, payout 1,244.30.
func TestCalculateGoCrumbsWerkstudentHours(t *testing.T) {
	code, resp := calcResponse(t, werkstudentBody(`"ws_hourly_wage":17,"ws_hours":80,"ws_hours_period":"month"`))
	if code != http.StatusOK || !resp.Success {
		t.Fatalf("status %d: %+v", code, resp)
	}
	if resp.Data.Gross != 1360 || resp.Data.Net != 1244.30 || resp.Data.ExtraTaxable != 0 {
		t.Errorf("got %+v, want gross 1360, net 1244.30, no extra pay", resp.Data)
	}
}

// 20 hours a week at 17.00 = 340.00 a week = 1,473.33 in an average month (52/12 weeks)
func TestCalculateGoCrumbsWerkstudentWeeklyHours(t *testing.T) {
	_, resp := calcResponse(t, werkstudentBody(`"ws_hourly_wage":17,"ws_hours":20,"ws_hours_period":"week"`))
	if resp.Data.Gross != 1473.33 {
		t.Errorf("gross = %v, want 1473.33", resp.Data.Gross)
	}
}

// Switching work type must not matter for the other type: a Werkstudent ignores gross, side income
// and extra pay, an employee ignores wage and hours.
func TestCalculateGoCrumbsWorkTypesAreSeparate(t *testing.T) {
	_, plain := calcResponse(t, `{"work_type":"werkstudent","gross_amount":0,"gross_period":"month","steuerklasse":1,"bundesland":"BE","zusatzbeitrag":2.69,"ws_hourly_wage":17,"ws_hours":80}`)
	_, withEmployeeData := calcResponse(t, werkstudentBody(`"ws_hourly_wage":17,"ws_hours":80`))
	if !reflect.DeepEqual(plain.Data, withEmployeeData.Data) || plain.Data.Net != 1244.30 {
		t.Errorf("employee fields leaked into a Werkstudent: %+v vs %+v", plain.Data, withEmployeeData.Data)
	}

	_, fte := calcResponse(t, goCrumbsBody(""))
	_, fteWithStudentData := calcResponse(t, goCrumbsBody(`"ws_hourly_wage":17,"ws_hours":80`))
	if !reflect.DeepEqual(fte.Data, fteWithStudentData.Data) || fte.Data.Net != 2605.5 {
		t.Errorf("Werkstudent fields leaked into an employee: %+v vs %+v", fte.Data, fteWithStudentData.Data)
	}
}

// The inactive employee fields are stored too, so a Werkstudent profile with an employee gross the
// engine rejects (above 1,000,000 a month) must fail now, not only after switching to employee
func TestGoCrumbsRejectsInvalidInactiveIncome(t *testing.T) {
	body := strings.Replace(werkstudentBody(`"ws_hourly_wage":17,"ws_hours":80`), `"gross_amount":4916.67`, `"gross_amount":2000000`, 1)
	if code, resp := calcResponse(t, body); code != http.StatusBadRequest || resp.Error == "" {
		t.Errorf("calculate: status %d resp %+v, want 400 with an error", code, resp)
	}
	// Save validates before it touches the database (nil DB in this router)
	if w := doJSON(goCrumbsRouter(), "PUT", "/gocrumbs", body); w.Code != http.StatusBadRequest {
		t.Errorf("save: status %d, want 400", w.Code)
	}
	// A yearly gross of the same amount is fine (166,666.67 a month)
	yearly := strings.Replace(body, `"gross_period":"month"`, `"gross_period":"year"`, 1)
	if code, resp := calcResponse(t, yearly); code != http.StatusOK || resp.Data.Net != 1244.30 {
		t.Errorf("yearly: status %d net %v, want 200 / 1244.30", code, resp.Data.Net)
	}
}

package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func goCrumbsPageBody(name, extra string) string {
	body := `{"name":` + name + `,` + goCrumbsMinimal
	if extra != "" {
		body += "," + extra
	}
	return body + "}"
}

// nil DB: everything here must be rejected before the database is touched
func goCrumbsPagesRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewGoCrumbsHandler(nil)
	r.POST("/pages/bind", func(c *gin.Context) {
		if req, ok := bindGoCrumbsPage(c); ok {
			c.JSON(http.StatusOK, req)
		}
	})
	r.POST("/pages", h.CreateGoCrumbsPage)
	r.GET("/pages/:id", h.GetGoCrumbsPage)
	r.PUT("/pages/:id", h.UpdateGoCrumbsPage)
	r.DELETE("/pages/:id", h.DeleteGoCrumbsPage)
	return r
}

func TestBindGoCrumbsPage(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{"ok", goCrumbsPageBody(`"January"`, ""), http.StatusOK},
		{"missing name", `{` + goCrumbsMinimal + `}`, http.StatusBadRequest},
		{"empty name", goCrumbsPageBody(`""`, ""), http.StatusBadRequest},
		{"whitespace name", goCrumbsPageBody(`"   "`, ""), http.StatusBadRequest},
		{"name at limit", goCrumbsPageBody(`"`+strings.Repeat("a", 40)+`"`, ""), http.StatusOK},
		{"name at limit multibyte", goCrumbsPageBody(`"`+strings.Repeat("ä", 40)+`"`, ""), http.StatusOK},
		{"name too long", goCrumbsPageBody(`"`+strings.Repeat("a", 41)+`"`, ""), http.StatusBadRequest},
		{"invalid inputs", goCrumbsPageBody(`"Jan"`, `"rent":-1`), http.StatusBadRequest},
		{"blank expense name", goCrumbsPageBody(`"Jan"`, `"subscriptions":[{"name":" ","amount":5}]`), http.StatusBadRequest},
		{"not calculable", strings.Replace(goCrumbsPageBody(`"Jan"`, ""), `"gross_amount":4000`, `"gross_amount":2000000`, 1), http.StatusBadRequest},
		{"invalid JSON", `{`, http.StatusBadRequest},
	}
	r := goCrumbsPagesRouter()
	for _, c := range cases {
		if w := doJSON(r, "POST", "/pages/bind", c.body); w.Code != c.want {
			t.Errorf("%s: status %d, want %d (%s)", c.name, w.Code, c.want, w.Body.String())
		}
	}
}

func TestBindGoCrumbsPageTrims(t *testing.T) {
	w := doJSON(goCrumbsPagesRouter(), "POST", "/pages/bind",
		goCrumbsPageBody(`"  Feb  "`, `"subscriptions":[{"name":"  Netflix ","amount":5}]`))
	var got struct {
		Name          string `json:"name"`
		WsHoursPeriod string `json:"ws_hours_period"`
		Subscriptions []struct{ Name string }
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || len(got.Subscriptions) != 1 {
		t.Fatalf("bad response %s (%v)", w.Body.String(), err)
	}
	if got.Name != "Feb" || got.Subscriptions[0].Name != "Netflix" || got.WsHoursPeriod != "month" {
		t.Errorf("not cleaned: %+v", got)
	}
}

func TestGoCrumbsPagesRejectBeforeDB(t *testing.T) {
	r := goCrumbsPagesRouter()
	cases := []struct {
		method, path, body string
		want               int
	}{
		{"POST", "/pages", goCrumbsPageBody(`""`, ""), http.StatusBadRequest},
		{"GET", "/pages/abc", "", http.StatusBadRequest},
		{"GET", "/pages/0", "", http.StatusBadRequest},
		{"PUT", "/pages/-1", goCrumbsPageBody(`"Jan"`, ""), http.StatusBadRequest},
		{"PUT", "/pages/1", goCrumbsPageBody(`" "`, ""), http.StatusBadRequest},
		{"DELETE", "/pages/x", "", http.StatusBadRequest},
	}
	for _, c := range cases {
		if w := doJSON(r, c.method, c.path, c.body); w.Code != c.want {
			t.Errorf("%s %s: status %d, want %d (%s)", c.method, c.path, w.Code, c.want, w.Body.String())
		}
	}
}

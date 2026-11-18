package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

func TestIsPublicIP(t *testing.T) {
	cases := map[string]bool{
		"93.184.216.34": true, "2606:4700::1111": true, "8.8.8.8": true,
		"127.0.0.1": false, "10.1.2.3": false, "172.16.0.1": false, "192.168.1.1": false,
		"169.254.169.254": false, // cloud metadata
		"100.64.0.1":      false, // CGNAT
		"0.0.0.0":         false, "255.255.255.255": false, "224.0.0.1": false,
		"::1": false, "fdaa::3": false, // Fly private network
		"fe80::1": false, "::ffff:127.0.0.1": false, "64:ff9b::7f00:1": false,
	}
	for ip, want := range cases {
		if got := isPublicIP(net.ParseIP(ip)); got != want {
			t.Errorf("isPublicIP(%s) = %v, want %v", ip, got, want)
		}
	}
}

func TestValidateLookupURL(t *testing.T) {
	ok := []string{"https://shop.example/p/1", "http://shop.example:80/x", "https://shop.example:443/x?y=1", "  https://shop.example  "}
	bad := []string{"", "shop.example/p", "ftp://shop.example/f", "javascript:alert(1)", "https://user:pw@shop.example/",
		"https://shop.example:8080/", "http://127.0.0.1/", "http://169.254.169.254/latest/meta-data",
		"http://[::1]/", "http://localhost/", "http://api.localhost/", "http://metadata.google.internal/",
		"https://" + strings.Repeat("a", 2050) + ".example/"}
	for _, u := range ok {
		if _, err := validateLookupURL(u); err != nil {
			t.Errorf("%q should be allowed: %v", u, err)
		}
	}
	for _, u := range bad {
		if _, err := validateLookupURL(u); err == nil {
			t.Errorf("%q should be rejected", u)
		}
	}
}

// The dial-time check is what stops DNS names / redirects that lead to private addresses
func TestCheckLookupDial(t *testing.T) {
	cases := map[string]bool{
		"93.184.216.34:443": true, "93.184.216.34:80": true,
		"93.184.216.34:8080": false, "127.0.0.1:443": false, "10.0.0.5:80": false,
		"169.254.169.254:80": false, "[fdaa::3]:443": false, "[::1]:443": false,
	}
	for addr, want := range cases {
		err := checkLookupDial("tcp", addr, nil)
		if (err == nil) != want {
			t.Errorf("dial %s: allowed=%v, want %v", addr, err == nil, want)
		}
	}
	// Redirects are re-validated too
	req, _ := http.NewRequest("GET", "http://169.254.169.254/latest/meta-data", nil)
	if lookupClient.CheckRedirect(req, []*http.Request{{}}) == nil {
		t.Error("redirect to the metadata address should be refused")
	}
}

func TestParsePrice(t *testing.T) {
	cases := map[string]float64{
		"1299": 1299, "1299.00": 1299, "1,299.00": 1299, "1.299,00": 1299, "12,50": 12.5, "€ 12,50": 12.5,
		"$1,299": 1299, "0.99": 0.99, "19.999": 20, // rounded to cents
	}
	for in, want := range cases {
		p := parsePrice(in)
		if p == nil || *p != want {
			t.Errorf("parsePrice(%q) = %v, want %v", in, p, want)
		}
	}
	for _, in := range []string{"", "free", "0", "0,00", "-5", "999999999"} {
		if p := parsePrice(in); p != nil && in != "-5" {
			t.Errorf("parsePrice(%q) = %v, want nil", in, *p)
		}
	}
}

func TestExtractProductInfo(t *testing.T) {
	cases := []struct {
		name, html   string
		wantName     string
		wantPrice    float64
		wantCurrency string
	}{
		{"JSON-LD Product with Offer", `<script type="application/ld+json">{"@context":"https://schema.org","@type":"Product","name":"Nintendo Switch 2","offers":{"@type":"Offer","price":"449.99","priceCurrency":"EUR"}}</script>`,
			"Nintendo Switch 2", 449.99, "EUR"},
		{"JSON-LD @graph + AggregateOffer + numeric price", `<script type="application/ld+json">{"@graph":[{"@type":"WebPage"},{"@type":["Product","Thing"],"name":"Desk Lamp","offers":{"@type":"AggregateOffer","lowPrice":39.9,"priceCurrency":"usd"}}]}</script>`,
			"Desk Lamp", 39.9, "USD"},
		{"JSON-LD offers list + priceSpecification", `<script type="application/ld+json">[{"@type":"Product","name":"Sofa","offers":[{"@type":"Offer","priceSpecification":{"price":"1.299,00","priceCurrency":"EUR"}}]}]</script>`,
			"Sofa", 1299, "EUR"},
		{"Open Graph price meta tags", `<head><meta property="og:title" content="Bluetooth Speaker"><meta property="product:price:amount" content="59,95"><meta property="product:price:currency" content="EUR"></head>`,
			"Bluetooth Speaker", 59.95, "EUR"},
		{"Microdata itemprop", `<h1>Running Shoes</h1><span itemprop="price" content="120.00"></span><meta itemprop="priceCurrency" content="GBP">`,
			"Running Shoes", 120, "GBP"},
	}
	for _, tc := range cases {
		info := extractProductInfo(&productPage{body: []byte("<html>" + tc.html + "</html>")}, []string{"shop.example"})
		if info.name != tc.wantName || info.price == nil || *info.price != tc.wantPrice || info.currency != tc.wantCurrency {
			got := "nil"
			if info.price != nil {
				b, _ := json.Marshal(*info.price)
				got = string(b)
			}
			t.Errorf("%s: got name=%q price=%s currency=%q", tc.name, info.name, got, info.currency)
		}
	}
	// No product data: name from the heading, no price, and the text excerpt is collected for the AI
	info := extractProductInfo(&productPage{body: []byte(`<html><head><title>Shop</title><style>.x{}</style></head><body><h1>Handmade Mug</h1><p>Only 24,90 € today</p><script>var secret=1</script></body></html>`)}, []string{"shop.example"})
	if info.name != "Handmade Mug" || info.price != nil || !strings.Contains(info.excerpt, "24,90") || strings.Contains(info.excerpt, "secret") {
		t.Errorf("plain page: %+v", info)
	}
}

// End to end against a local fake shop (private addresses allowed only in tests) and a fake OpenAI
func TestLookupWishEndpoint(t *testing.T) {
	allowPrivateLookups = true
	defer func() { allowPrivateLookups = false }()

	shop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Error("lookup must not send cookies or auth headers")
		}
		switch r.URL.Path {
		case "/structured":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			io.WriteString(w, `<script type="application/ld+json">{"@type":"Product","name":"Espresso Machine","offers":{"price":"299.00","priceCurrency":"EUR"}}</script>`)
		case "/plain":
			w.Header().Set("Content-Type", "text/html")
			io.WriteString(w, `<h1>Wool Scarf</h1><p>Now only £35.00 (was £50.00)</p>`)
		case "/blocked":
			w.WriteHeader(http.StatusForbidden)
		case "/captcha":
			w.Header().Set("Content-Type", "text/html")
			io.WriteString(w, `<title>Robot Check</title><p>Please solve this captcha</p>`)
		case "/pdf":
			w.Header().Set("Content-Type", "application/pdf")
		case "/redirect":
			http.Redirect(w, r, "/structured", http.StatusFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer shop.Close()

	t.Setenv("OPENAI_API_KEY", "test-key")
	var aiCalls int
	var aiRequest map[string]any
	ai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		aiCalls++
		json.NewDecoder(r.Body).Decode(&aiRequest)
		io.WriteString(w, `{"output":[{"type":"message","content":[{"type":"output_text","text":"{\"name\":\"Wool Scarf\",\"price\":35,\"currency\":\"GBP\"}"}]}]}`)
	}))
	defer ai.Close()
	old := openaiEndpoint
	openaiEndpoint = ai.URL
	defer func() { openaiEndpoint = old }()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewWishHandler(nil)
	r.POST("/api/wish/lookup", h.LookupWish)
	lookup := func(link string) (int, wishLookupResult) {
		body, _ := json.Marshal(map[string]string{"url": link})
		req := httptest.NewRequest("POST", "/api/wish/lookup", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var out struct {
			Data wishLookupResult `json:"data"`
		}
		json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out.Data
	}

	if code, res := lookup(shop.URL + "/structured"); code != 200 || res.Status != lookupOK || res.Name != "Espresso Machine" ||
		res.Price == nil || *res.Price != 299 || res.Currency != "EUR" || res.Source != "page" {
		t.Errorf("structured: %d %+v", code, res)
	}
	if aiCalls != 0 {
		t.Error("shop data was available: the AI must not be called")
	}
	if _, res := lookup(shop.URL + "/redirect"); res.Name != "Espresso Machine" {
		t.Errorf("redirect: %+v", res)
	}
	if _, res := lookup(shop.URL + "/plain"); res.Status != lookupOK || res.Name != "Wool Scarf" || res.Price == nil ||
		*res.Price != 35 || res.Currency != "GBP" || res.Source != "ai" || aiCalls != 1 {
		t.Errorf("plain page (AI fallback): %+v, AI calls %d", res, aiCalls)
	}
	if text := aiRequest["input"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string); !strings.Contains(text, "Now only £35.00") {
		t.Error("AI didn't get the page excerpt")
	}
	for path, want := range map[string]string{"/blocked": lookupBlocked, "/captcha": lookupBlocked, "/pdf": lookupUnreachable, "/missing": lookupUnreachable} {
		if _, res := lookup(shop.URL + path); res.Status != want {
			t.Errorf("%s: status %q, want %q", path, res.Status, want)
		}
	}
	if code, _ := lookup("not a url"); code != http.StatusBadRequest {
		t.Errorf("invalid URL: %d", code)
	}

	// With private addresses forbidden (production), the same local shop is refused
	allowPrivateLookups = false
	if code, _ := lookup(shop.URL + "/structured"); code != http.StatusBadRequest {
		t.Errorf("private address in URL should be rejected, got %d", code)
	}
}

func TestIsSiteName(t *testing.T) {
	yes := [][2]string{{"Amazon", "www.amazon.it"}, {"Amazon.it", "www.amazon.it"}, {"amazon.co.uk", "amazon.co.uk"}, {" AMAZON ", "smile.amazon.de"}}
	no := [][2]string{{"Echo Dot (4th gen)", "www.amazon.it"}, {"IT", "www.amazon.it"}, {"", "www.amazon.it"}, {"Amazon Kindle", "www.amazon.it"}}
	for _, c := range yes {
		if !isSiteName(c[0], c[1]) {
			t.Errorf("%q on %s should count as the site's own name", c[0], c[1])
		}
	}
	for _, c := range no {
		if isSiteName(c[0], c[1]) {
			t.Errorf("%q on %s is a real product name", c[0], c[1])
		}
	}
}

// Amazon-style pages: a bot wall served with 200 and a title, and a page whose only name is the shop
func TestLookupWishShopNameOnlyPages(t *testing.T) {
	allowPrivateLookups = true
	defer func() { allowPrivateLookups = false }()
	shop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/interstitial": // 200 + title, "continue shopping" button, no product
			io.WriteString(w, `<title>Amazon.it</title><h1>Click the button below to continue shopping</h1><form action="/errors/validateCaptcha"></form>`)
		case "/generic": // no bot markers, only the shop's name
			io.WriteString(w, `<html><head><meta property="og:title" content="127.0.0.1"></head><body><p>Welcome to our store</p></body></html>`)
		}
	}))
	defer shop.Close()

	t.Setenv("OPENAI_API_KEY", "test-key")
	aiCalls := 0
	ai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		aiCalls++
		// The AI also only finds the shop's name
		io.WriteString(w, `{"output":[{"type":"message","content":[{"type":"output_text","text":"{\"name\":\"127.0.0.1\",\"price\":null,\"currency\":null}"}]}]}`)
	}))
	defer ai.Close()
	old := openaiEndpoint
	openaiEndpoint = ai.URL
	defer func() { openaiEndpoint = old }()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/wish/lookup", NewWishHandler(nil).LookupWish)
	lookup := func(link string) wishLookupResult {
		body, _ := json.Marshal(map[string]string{"url": link})
		req := httptest.NewRequest("POST", "/api/wish/lookup", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var out struct {
			Data wishLookupResult `json:"data"`
		}
		json.Unmarshal(w.Body.Bytes(), &out)
		return out.Data
	}

	if res := lookup(shop.URL + "/interstitial"); res.Status != lookupBlocked || res.Name != "" || aiCalls != 0 {
		t.Errorf("bot interstitial: %+v (AI calls %d), want blocked without calling the AI", res, aiCalls)
	}
	if res := lookup(shop.URL + "/generic"); res.Status != lookupBlocked || res.Name != "" {
		t.Errorf("shop-name-only page: %+v, want blocked", res)
	}
}

func TestCleanTitle(t *testing.T) {
	hosts := []string{"amzn.eu", "www.amazon.de"}
	cases := map[string]string{
		"Amazon.de: Pressure Cooker 6L : Amazon.de: Küche, Haushalt & Wohnen":                  "Pressure Cooker 6L",
		"Running Shoes - Men - shop.example":                                                   "Running Shoes",
		"Amazon.de: CREATE/PRESSURE COOKER STUDIO/Multifunktioneller Schnellkochtopf 6L Mokka": "CREATE/PRESSURE COOKER STUDIO/Multifunktioneller Schnellkochtopf 6L Mokka",
		"Desk Lamp | Amazon.de": "Desk Lamp",
		"Amazon.de":             "Amazon.de", // only the shop: returned as is, recognised by the caller
	}
	for in, want := range cases {
		if got := cleanTitle(in, hosts); got != want {
			t.Errorf("cleanTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

// The real case: amzn.eu short link → amazon.de product page without structured data
func TestExtractAmazonStylePage(t *testing.T) {
	hosts := []string{"amzn.eu", "www.amazon.de"}
	page := `<html><head><title>Amazon.de: Pressure Cooker 6L Mokka : Amazon.de: Küche</title>
<meta name="title" content="Amazon.de: Pressure Cooker 6L Mokka : Amazon.de: Küche"></head><body>
<header><h1>Amazon</h1><a>Alle Kategorien</a><span class="nav-price">Prime 8,99 €</span></header>
<nav>Bestseller Angebote Kundenservice Neuerscheinungen</nav>
<h1 id="title"><span id="productTitle">  Pressure Cooker 6L Mokka  </span></h1>
<div id="corePrice"><span class="a-price"><span class="a-offscreen">89,99 €</span></span></div>
<footer>Impressum Amazon.de</footer></body></html>`
	info := extractProductInfo(&productPage{body: []byte(page), finalHost: "www.amazon.de"}, hosts)
	if info.name != "Pressure Cooker 6L Mokka" || info.onlySiteName {
		t.Errorf("name = %q (onlySiteName %v), want the product title", info.name, info.onlySiteName)
	}
	if info.price != nil {
		t.Errorf("no structured price on the page, got %v", *info.price)
	}
	if !strings.Contains(info.excerpt, "PRICE TEXT: 89,99 €") || !strings.Contains(info.excerpt, "PRODUCT TITLE: Pressure Cooker 6L Mokka") {
		t.Errorf("excerpt should lead with the product title and price:\n%s", info.excerpt[:min(len(info.excerpt), 300)])
	}
	for _, chrome := range []string{"Alle Kategorien", "Bestseller", "Impressum", "Prime 8,99"} {
		if strings.Contains(info.excerpt, chrome) {
			t.Errorf("excerpt should skip header/nav/footer text, found %q", chrome)
		}
	}

	// What a data-centre IP can get instead: a generic page that only says "Amazon"
	generic := extractProductInfo(&productPage{body: []byte(`<title>Amazon.de</title><h1>Amazon</h1><p>Hallo</p>`), finalHost: "www.amazon.de"}, hosts)
	if generic.name != "" || !generic.onlySiteName {
		t.Errorf("generic Amazon page: name %q, onlySiteName %v; want no name and onlySiteName", generic.name, generic.onlySiteName)
	}
}

// Long page titles (Amazon) give way to the AI's short name when the AI runs anyway
func TestLookupWishPrefersShortAIName(t *testing.T) {
	allowPrivateLookups = true
	defer func() { allowPrivateLookups = false }()
	long := strings.Repeat("Pressure Cooker Studio Multifunctional ", 4)
	shop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, `<h1>`+long+`</h1><span class="price">119,95 €</span>`)
	}))
	defer shop.Close()
	t.Setenv("OPENAI_API_KEY", "test-key")
	ai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"output":[{"type":"message","content":[{"type":"output_text","text":"{\"name\":\"Pressure Cooker Studio 6L\",\"price\":119.95,\"currency\":\"EUR\"}"}]}]}`)
	}))
	defer ai.Close()
	old := openaiEndpoint
	openaiEndpoint = ai.URL
	defer func() { openaiEndpoint = old }()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/wish/lookup", NewWishHandler(nil).LookupWish)
	body, _ := json.Marshal(map[string]string{"url": shop.URL + "/p"})
	req := httptest.NewRequest("POST", "/api/wish/lookup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var out struct {
		Data wishLookupResult `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &out)
	if out.Data.Name != "Pressure Cooker Studio 6L" || out.Data.Price == nil || *out.Data.Price != 119.95 {
		t.Errorf("got %+v, want the AI's short name and price", out.Data)
	}
}

func TestCutAtWord(t *testing.T) {
	long := "CREATE/PRESSURE COOKER STUDIO/Multifunktioneller programmierbarer elektrischer Schnellkochtopf 6L"
	got := cutAtWord(long, 64)
	if utf8.RuneCountInString(got) > 64 || strings.HasSuffix(got, " ") || strings.HasSuffix(got, "/") || got != "CREATE/PRESSURE COOKER STUDIO/Multifunktioneller" {
		t.Errorf("cutAtWord = %q (%d chars)", got, utf8.RuneCountInString(got))
	}
	if cutAtWord("Short name", 64) != "Short name" {
		t.Error("short names must stay as they are")
	}
	nospace := strings.Repeat("x", 80)
	if utf8.RuneCountInString(cutAtWord(nospace, 64)) != 64 {
		t.Error("a name without spaces is cut at the limit")
	}
}

// A shop with full product data but a very long name: one tiny AI call shortens it, price stays from the page
func TestLookupWishShortensLongNames(t *testing.T) {
	allowPrivateLookups = true
	defer func() { allowPrivateLookups = false }()
	long := "CREATE/PRESSURE COOKER STUDIO/Multifunktioneller programmierbarer elektrischer Schnellkochtopf 6L Mokka / 15 Funktionen, 24h Timer"
	shop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, `<script type="application/ld+json">{"@type":"Product","name":"`+long+`","offers":{"price":"119.95","priceCurrency":"EUR"}}</script>`)
	}))
	defer shop.Close()

	var aiReply string
	var aiPrompts []string
	ai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		aiPrompts = append(aiPrompts, req["input"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string))
		io.WriteString(w, `{"output":[{"type":"message","content":[{"type":"output_text","text":`+strconvQuote(aiReply)+`}]}]}`)
	}))
	defer ai.Close()
	old := openaiEndpoint
	openaiEndpoint = ai.URL
	defer func() { openaiEndpoint = old }()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/wish/lookup", NewWishHandler(nil).LookupWish)
	lookup := func() wishLookupResult {
		body, _ := json.Marshal(map[string]string{"url": shop.URL + "/p"})
		req := httptest.NewRequest("POST", "/api/wish/lookup", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var out struct {
			Data wishLookupResult `json:"data"`
		}
		json.Unmarshal(w.Body.Bytes(), &out)
		return out.Data
	}

	// AI keeps the important details
	t.Setenv("OPENAI_API_KEY", "test-key")
	aiReply = `{"name":"Create Pressure Cooker Studio 6L, Mokka"}`
	res := lookup()
	if res.Name != "Create Pressure Cooker Studio 6L, Mokka" || res.Price == nil || *res.Price != 119.95 || res.Source != "page" {
		t.Errorf("shortened by AI: %+v", res)
	}
	if len(aiPrompts) != 1 || !strings.Contains(aiPrompts[0], "at most 60 characters") || !strings.Contains(aiPrompts[0], long) {
		t.Errorf("expected exactly one shortening call with the long name, got %d", len(aiPrompts))
	}

	// AI answers too long: cut at a word instead
	aiReply = `{"name":"` + long + `"}`
	if res := lookup(); utf8.RuneCountInString(res.Name) > 64 || res.Name != cutAtWord(long, 64) {
		t.Errorf("AI too long → word cut, got %q", res.Name)
	}

	// No AI configured: cut at a word
	t.Setenv("OPENAI_API_KEY", "")
	if res := lookup(); res.Name != cutAtWord(long, 64) {
		t.Errorf("without AI → word cut, got %q", res.Name)
	}
}

func strconvQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

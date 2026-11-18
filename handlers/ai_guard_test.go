package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// Unit tests run without a database: allow every AI call unless a test says otherwise
func init() {
	takeAIQuota = func(*sql.DB, int, string, int) (bool, error) { return true, nil }
}

func withQuota(t *testing.T, allowed bool) {
	t.Helper()
	old := takeAIQuota
	takeAIQuota = func(*sql.DB, int, string, int) (bool, error) { return allowed, nil }
	t.Cleanup(func() { takeAIQuota = old })
}

func TestAIDailyLimitDefaultsAndOverrides(t *testing.T) {
	t.Setenv("AI_DAILY_RECEIPT_SCANS", "")
	t.Setenv("AI_DAILY_WISH_LOOKUPS", "")
	if aiDailyLimit(aiKindReceipt) != 20 || aiDailyLimit(aiKindWish) != 50 {
		t.Errorf("defaults: receipt %d, wish %d", aiDailyLimit(aiKindReceipt), aiDailyLimit(aiKindWish))
	}
	t.Setenv("AI_DAILY_RECEIPT_SCANS", "5")
	t.Setenv("AI_DAILY_WISH_LOOKUPS", "0") // 0 = AI off for wishes
	if aiDailyLimit(aiKindReceipt) != 5 || aiDailyLimit(aiKindWish) != 0 {
		t.Errorf("overrides: receipt %d, wish %d", aiDailyLimit(aiKindReceipt), aiDailyLimit(aiKindWish))
	}
	t.Setenv("AI_DAILY_RECEIPT_SCANS", "lots")
	if aiDailyLimit(aiKindReceipt) != 20 {
		t.Error("invalid value should fall back to the default")
	}
}

func TestOpenAIUsageParsed(t *testing.T) {
	_, _, usage, err := openAIOutputText([]byte(`{"usage":{"input_tokens":1234,"output_tokens":56,"total_tokens":1290},"output":[{"type":"message","content":[{"type":"output_text","text":"{}"}]}]}`))
	if err != nil || usage.InputTokens != 1234 || usage.OutputTokens != 56 {
		t.Errorf("usage = %+v, err %v", usage, err)
	}
}

func TestHasPriceText(t *testing.T) {
	yes := []string{"Now only £35.00", "119,95€", "€ 12,50", "$9.99", "Preis: 25 EUR", "CHF 49.90 chf"}
	no := []string{"Welcome to our store", "Top 10 gadgets of 2026", "Call 0800 123 456", ""}
	for _, s := range yes {
		if !hasPriceText(s) {
			t.Errorf("%q should look like a price", s)
		}
	}
	for _, s := range no {
		if hasPriceText(s) {
			t.Errorf("%q has no price", s)
		}
	}
}

// Over the daily limit, a receipt scan answers 429 (the frontend then uses on-device OCR) and OpenAI isn't called
func TestScanReceipt_DailyLimit(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	var got map[string]any
	fakeOpenAI(t, &got, http.StatusOK, openAIReply)
	withQuota(t, false)
	body, ct := receiptForm(t, pngHeader, "image/png")
	req := httptest.NewRequest("POST", "/receipt/scan", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	scanRouter().ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests || got != nil {
		t.Fatalf("expected 429 and no OpenAI call, got %d (OpenAI called: %v)", w.Code, got != nil)
	}
}

func lookupRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/wish/lookup", NewWishHandler(nil).LookupWish)
	return r
}

func doLookup(r *gin.Engine, link string) wishLookupResult {
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

func TestLookupWish_GuardrailsSaveAICalls(t *testing.T) {
	allowPrivateLookups = true
	defer func() { allowPrivateLookups = false }()
	resetLookupCache()
	shopHits := map[string]int{}
	shop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		shopHits[r.URL.Path]++
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/needs-ai": // product without structured data
			io.WriteString(w, `<h1>Wool Scarf</h1><p>Now only £35.00</p>`)
		case "/article": // no price anywhere
			io.WriteString(w, `<h1>Our favourite scarves this winter</h1><p>Read about wool, cashmere and more.</p>`)
		case "/nothing": // only a price, no name candidates: the AI would be needed for everything
			io.WriteString(w, `<p>Price: 12,50 €</p>`)
		case "/structured":
			io.WriteString(w, `<script type="application/ld+json">{"@type":"Product","name":"Lamp","offers":{"price":"39.90","priceCurrency":"EUR"}}</script>`)
		}
	}))
	defer shop.Close()
	t.Setenv("OPENAI_API_KEY", "test-key")
	aiCalls := 0
	ai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		aiCalls++
		io.WriteString(w, `{"usage":{"input_tokens":900,"output_tokens":20},"output":[{"type":"message","content":[{"type":"output_text","text":"{\"name\":\"Wool Scarf\",\"price\":35,\"currency\":\"GBP\"}"}]}]}`)
	}))
	defer ai.Close()
	old := openaiEndpoint
	openaiEndpoint = ai.URL
	defer func() { openaiEndpoint = old }()
	r := lookupRouter()

	// No price-looking text: answered without the AI
	if res := doLookup(r, shop.URL+"/article"); aiCalls != 0 || res.Price != nil {
		t.Errorf("article page: %+v, AI calls %d (want 0)", res, aiCalls)
	}

	// Daily limit used up: the name from the page is still returned, flagged so the form can say why the
	// price is missing; no AI call, and the partial answer isn't cached
	withQuota(t, false)
	if res := doLookup(r, shop.URL+"/needs-ai"); res.Status != lookupOK || res.Name != "Wool Scarf" || res.Price != nil || !res.AILimit || aiCalls != 0 {
		t.Errorf("over the limit: %+v, AI calls %d (want name, no price, ai_limit, 0 calls)", res, aiCalls)
	}
	// Nothing at all without the AI → status "limit"
	if res := doLookup(r, shop.URL+"/nothing"); res.Status != lookupLimit {
		t.Errorf("over the limit with nothing found: %+v, want status limit", res)
	}
	// …but pages with structured data still work (no AI needed)
	if res := doLookup(r, shop.URL+"/structured"); res.Status != lookupOK || res.Name != "Lamp" {
		t.Errorf("structured page over the limit: %+v", res)
	}

	// Within the limit: one AI call, then the same link comes from the cache (no fetch, no AI)
	withQuota(t, true)
	if res := doLookup(r, shop.URL+"/needs-ai"); res.Status != lookupOK || aiCalls != 1 {
		t.Errorf("within the limit: %+v, AI calls %d", res, aiCalls)
	}
	hits := shopHits["/needs-ai"]
	if res := doLookup(r, shop.URL+"/needs-ai"); res.Name != "Wool Scarf" || aiCalls != 1 || shopHits["/needs-ai"] != hits {
		t.Errorf("second lookup should come from the cache: %+v, AI calls %d, shop hits %d→%d", res, aiCalls, hits, shopHits["/needs-ai"])
	}
}

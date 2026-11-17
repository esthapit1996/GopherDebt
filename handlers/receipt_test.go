package handlers

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// Smallest valid PNG header, enough for http.DetectContentType
var pngHeader = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")

func receiptForm(t *testing.T, content []byte, contentType string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="image"; filename="receipt"`)
	h.Set("Content-Type", contentType)
	part, err := w.CreatePart(h)
	if err != nil {
		t.Fatal(err)
	}
	part.Write(content)
	w.Close()
	return &buf, w.FormDataContentType()
}

// fakeGemini records the request it got and answers like Gemini
func fakeGemini(t *testing.T, got *map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("key") != "test-key" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(got); err != nil {
			t.Errorf("Gemini got invalid JSON: %v", err)
		}
		io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"{\"total\": 1}"}]}}]}`)
	}))
	old := geminiEndpoint
	geminiEndpoint = srv.URL
	t.Cleanup(func() { geminiEndpoint = old; srv.Close() })
	return srv
}

func scanRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.MaxMultipartMemory = 8 << 20
	r.POST("/receipt/scan", NewReceiptHandler().ScanReceipt)
	return r
}

func TestScanReceipt_ForwardsImageToGemini(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")
	var got map[string]any
	fakeGemini(t, &got)

	image := append(append([]byte{}, pngHeader...), bytes.Repeat([]byte{7}, 5000)...)
	body, ct := receiptForm(t, image, "application/octet-stream") // header is ignored; content is sniffed
	req := httptest.NewRequest("POST", "/receipt/scan", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	scanRouter().ServeHTTP(w, req)

	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `{\"total\": 1}`) {
		t.Fatalf("expected 200 with Gemini text, got %d: %s", w.Code, w.Body.String())
	}
	parts := got["contents"].([]any)[0].(map[string]any)["parts"].([]any)
	inline := parts[0].(map[string]any)["inlineData"].(map[string]any)
	if inline["mimeType"] != "image/png" {
		t.Errorf("mimeType = %v, want image/png", inline["mimeType"])
	}
	if inline["data"] != base64.StdEncoding.EncodeToString(image) {
		t.Error("image data was not forwarded intact")
	}
	if text := parts[1].(map[string]any)["text"].(string); !strings.HasPrefix(text, "You are a receipt parsing expert.") {
		t.Errorf("prompt not forwarded: %.40q", text)
	}
}

func TestScanReceipt_SizeLimit(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")
	var got map[string]any
	fakeGemini(t, &got)
	r := scanRouter()

	// Just under the limit goes through
	under := append(append([]byte{}, pngHeader...), make([]byte, MaxReceiptBytes-len(pngHeader))...)
	body, ct := receiptForm(t, under, "image/png")
	req := httptest.NewRequest("POST", "/receipt/scan", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("exactly the max: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	over := append(append([]byte{}, pngHeader...), make([]byte, MaxReceiptBytes+1-len(pngHeader))...)

	// Over the limit with Content-Length: rejected up front
	body, ct = receiptForm(t, over, "image/png")
	req = httptest.NewRequest("POST", "/receipt/scan", body)
	req.Header.Set("Content-Type", ct)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("max + 1 byte: expected 413, got %d", w.Code)
	}

	// Way over the limit without Content-Length (chunked): stopped while reading
	big := append(append([]byte{}, pngHeader...), make([]byte, MaxReceiptBytes+2*receiptFormOverhead)...)
	body, ct = receiptForm(t, big, "image/png")
	req = httptest.NewRequest("POST", "/receipt/scan", io.MultiReader(body))
	req.ContentLength = -1
	req.Header.Set("Content-Type", ct)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("chunked oversize: expected 413, got %d: %s", w.Code, w.Body.String())
	}
}

func TestScanReceipt_RejectsNonImages(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")
	var got map[string]any
	fakeGemini(t, &got)

	body, ct := receiptForm(t, []byte("hello, this is not an image"), "image/png")
	req := httptest.NewRequest("POST", "/receipt/scan", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	scanRouter().ServeHTTP(w, req)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d", w.Code)
	}
	if got != nil {
		t.Error("non-image was sent to Gemini")
	}
}

func TestScanReceipt_BusyWhenSlotsFull(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")
	for i := 0; i < cap(receiptScanSlots); i++ {
		receiptScanSlots <- struct{}{}
	}
	defer func() {
		for i := 0; i < cap(receiptScanSlots); i++ {
			<-receiptScanSlots
		}
	}()

	body, ct := receiptForm(t, pngHeader, "image/png")
	req := httptest.NewRequest("POST", "/receipt/scan", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	scanRouter().ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", w.Code)
	}
}

// The largest allowed upload must still produce a request Gemini accepts
func TestMaxReceiptFitsGemini(t *testing.T) {
	image := bytes.NewReader(make([]byte, MaxReceiptBytes))
	body, err := buildGeminiRequest(image, MaxReceiptBytes, "image/jpeg")
	if err != nil {
		t.Fatal(err)
	}
	if body.Len() >= GeminiMaxRequestBytes {
		t.Fatalf("max upload builds a %d-byte request, over Gemini's %d limit", body.Len(), GeminiMaxRequestBytes)
	}
	t.Logf("largest request: %d bytes (%.1f%% of Gemini's limit)", body.Len(), 100*float64(body.Len())/GeminiMaxRequestBytes)
}

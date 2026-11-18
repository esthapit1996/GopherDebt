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

// A Responses API reply as OpenAI sends it: a reasoning item, then the message with output_text
const openAIReply = `{"id":"resp_1","object":"response","status":"completed","output":[` +
	`{"type":"reasoning","summary":[]},` +
	`{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"store_name\":null,\"items\":[],\"total\":1}","annotations":[]}]}]}`

// fakeOpenAI records the request it got and answers with the given status/body
func fakeOpenAI(t *testing.T, got *map[string]any, status int, reply string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" || r.URL.RawQuery != "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(got); err != nil {
			t.Errorf("OpenAI got invalid JSON: %v", err)
		}
		w.WriteHeader(status)
		io.WriteString(w, reply)
	}))
	old := openaiEndpoint
	openaiEndpoint = srv.URL
	t.Cleanup(func() { openaiEndpoint = old; srv.Close() })
	return srv
}

func scanRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.MaxMultipartMemory = 8 << 20
	r.POST("/receipt/scan", NewReceiptHandler(nil).ScanReceipt)
	return r
}

func TestScanReceipt_ForwardsImageToOpenAI(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	var got map[string]any
	fakeOpenAI(t, &got, http.StatusOK, openAIReply)

	image := append(append([]byte{}, pngHeader...), bytes.Repeat([]byte{7}, 5000)...)
	body, ct := receiptForm(t, image, "application/octet-stream") // header is ignored; content is sniffed
	req := httptest.NewRequest("POST", "/receipt/scan", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	scanRouter().ServeHTTP(w, req)

	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `\"total\":1`) {
		t.Fatalf("expected 200 with the model's JSON text, got %d: %s", w.Code, w.Body.String())
	}
	if got["model"] != DefaultReceiptModel {
		t.Errorf("model = %v, want %s", got["model"], DefaultReceiptModel)
	}
	format := got["text"].(map[string]any)["format"].(map[string]any)
	if format["type"] != "json_schema" || format["strict"] != true || format["schema"] == nil {
		t.Errorf("structured output not requested: %v", format)
	}
	content := got["input"].([]any)[0].(map[string]any)["content"].([]any)
	if text := content[0].(map[string]any)["text"].(string); !strings.HasPrefix(text, "You are a receipt parsing expert.") {
		t.Errorf("prompt not forwarded: %.40q", text)
	}
	img := content[1].(map[string]any)
	want := "data:image/png;base64," + base64.StdEncoding.EncodeToString(image)
	if img["type"] != "input_image" || img["image_url"] != want {
		t.Error("image was not forwarded intact as a PNG data URL")
	}
}

func TestScanReceipt_ModelOverride(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_RECEIPT_MODEL", "gpt-6.1-terra")
	var got map[string]any
	fakeOpenAI(t, &got, http.StatusOK, openAIReply)
	body, ct := receiptForm(t, pngHeader, "image/png")
	req := httptest.NewRequest("POST", "/receipt/scan", body)
	req.Header.Set("Content-Type", ct)
	scanRouter().ServeHTTP(httptest.NewRecorder(), req)
	if got["model"] != "gpt-6.1-terra" {
		t.Errorf("model = %v, want the OPENAI_RECEIPT_MODEL override", got["model"])
	}
}

func TestScanReceipt_OpenAIErrors(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	cases := []struct {
		name   string
		status int
		reply  string
		want   int
	}{
		{"rate limited → 429 so the frontend falls back to OCR", http.StatusTooManyRequests, `{"error":{"message":"Rate limit"}}`, http.StatusTooManyRequests},
		{"server error → 502", http.StatusInternalServerError, `{"error":{"message":"boom"}}`, http.StatusBadGateway},
		{"refusal → 422", http.StatusOK, `{"output":[{"type":"message","content":[{"type":"refusal","refusal":"I can't help with that."}]}]}`, http.StatusUnprocessableEntity},
		{"empty output → 500", http.StatusOK, `{"output":[]}`, http.StatusInternalServerError},
	}
	for _, tc := range cases {
		var got map[string]any
		fakeOpenAI(t, &got, tc.status, tc.reply)
		body, ct := receiptForm(t, pngHeader, "image/png")
		req := httptest.NewRequest("POST", "/receipt/scan", body)
		req.Header.Set("Content-Type", ct)
		w := httptest.NewRecorder()
		scanRouter().ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Errorf("%s: got %d (%s)", tc.name, w.Code, w.Body.String())
		}
	}
}

func TestScanReceipt_RejectsUnsupportedImageTypes(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	var got map[string]any
	fakeOpenAI(t, &got, http.StatusOK, openAIReply)
	// HEIC ("ftypheic") isn't accepted by OpenAI, even when declared as an image
	heic := append([]byte("\x00\x00\x00\x18ftypheic"), make([]byte, 100)...)
	body, ct := receiptForm(t, heic, "image/heic")
	req := httptest.NewRequest("POST", "/receipt/scan", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	scanRouter().ServeHTTP(w, req)
	if w.Code != http.StatusUnsupportedMediaType || got != nil {
		t.Fatalf("HEIC: expected 415 and nothing sent to OpenAI, got %d", w.Code)
	}
}

func TestScanReceipt_SizeLimit(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	var got map[string]any
	fakeOpenAI(t, &got, http.StatusOK, openAIReply)
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
	t.Setenv("OPENAI_API_KEY", "test-key")
	var got map[string]any
	fakeOpenAI(t, &got, http.StatusOK, openAIReply)

	body, ct := receiptForm(t, []byte("hello, this is not an image"), "image/png")
	req := httptest.NewRequest("POST", "/receipt/scan", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	scanRouter().ServeHTTP(w, req)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d", w.Code)
	}
	if got != nil {
		t.Error("non-image was sent to OpenAI")
	}
}

func TestScanReceipt_BusyWhenSlotsFull(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
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

// The largest allowed upload still builds a valid JSON request
func TestMaxReceiptBuildsValidRequest(t *testing.T) {
	image := bytes.NewReader(make([]byte, MaxReceiptBytes))
	body, err := buildOpenAIRequest(image, MaxReceiptBytes, "image/jpeg", DefaultReceiptModel)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(body.Bytes()) {
		t.Fatal("request body is not valid JSON")
	}
	t.Logf("largest request: %.1f MB", float64(body.Len())/(1<<20))
}

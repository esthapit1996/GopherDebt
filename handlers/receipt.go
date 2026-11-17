package handlers

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gopherdebt/models"
)

// GeminiMaxRequestBytes is Gemini's documented cap on a whole inline-data request (prompt + base64 image)
const GeminiMaxRequestBytes = 20_000_000

// MaxReceiptBytes caps receipt image uploads so the base64 request still fits GeminiMaxRequestBytes
// (14 MB → ~19.6 MB of base64; see TestMaxReceiptFitsGemini). Bigger uploads get 413 before they're read.
const MaxReceiptBytes = 14 << 20

// Room for multipart headers/boundaries on top of the image itself
const receiptFormOverhead = 1 << 20

// At most this many scans hold an image in memory at once (the VM has 256 MB); extra requests get 429
var receiptScanSlots = make(chan struct{}, 2)

// geminiEndpoint is a variable so tests can point it at a fake server
var geminiEndpoint = "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent"

// Bounds how long a scan can hold its encoded image while waiting on Gemini
var geminiClient = &http.Client{Timeout: 60 * time.Second}

// receiptPrompt tells Gemini how to parse the receipt (unchanged)
const receiptPrompt = `You are a receipt parsing expert. Analyze this receipt image and extract structured data.
The receipt can be in ANY language (German, Dutch, English, French, etc.) — you must understand it regardless.

Return ONLY valid JSON in this exact format, nothing else:
{
  "store_name": "Store Name or null",
  "items": [
    {"name": "Item name in English", "price": 3.50}
  ],
  "total": 15.99
}

Rules:
- Extract ONLY purchased items/products with their prices
- Translate all item names to English
- Prices must be numbers (use . as decimal separator, e.g. 3.50 not "3,50")
- "total" = the final total amount (e.g. Total, Summe, Gesamtbetrag, Totaal, etc.) — NOT an item
- NEVER include these as items: totals, subtotals, tax (MwSt/BTW/VAT), discounts, payment method, change, tips, or any summary lines
- If multiple totals exist, use the largest (grand total)
- If no clear total line, set total to null
- Return ONLY the JSON object, no markdown fences, no explanation`

type ReceiptHandler struct{}

func NewReceiptHandler() *ReceiptHandler {
	return &ReceiptHandler{}
}

// ReceiptConfig returns whether server-side Gemini key is configured
func (h *ReceiptHandler) ReceiptConfig(c *gin.Context) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	enabled := apiKey != ""
	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: map[string]bool{"enabled": enabled}})
}

// ScanReceipt proxies a receipt image to Gemini API so the API key stays server-side
func (h *ReceiptHandler) ScanReceipt(c *gin.Context) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		c.JSON(http.StatusServiceUnavailable, models.APIResponse{Success: false, Error: "AI receipt scanning not configured"})
		return
	}

	if c.Request.ContentLength > MaxReceiptBytes+receiptFormOverhead {
		c.JSON(http.StatusRequestEntityTooLarge, models.APIResponse{Success: false, Error: "Image too large (max 14 MB)"})
		return
	}
	// Also enforced while reading, for requests without a Content-Length
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, MaxReceiptBytes+receiptFormOverhead)

	select {
	case receiptScanSlots <- struct{}{}:
		defer func() { <-receiptScanSlots }()
	default:
		c.JSON(http.StatusTooManyRequests, models.APIResponse{Success: false, Error: "Receipt scanner is busy, please try again in a moment"})
		return
	}

	// c.FormFile honours the engine's MaxMultipartMemory (8 MB); c.Request.FormFile would buffer 32 MB
	header, err := c.FormFile("image")
	if err != nil {
		var too_large *http.MaxBytesError
		if errors.As(err, &too_large) {
			c.JSON(http.StatusRequestEntityTooLarge, models.APIResponse{Success: false, Error: "Image too large (max 14 MB)"})
			return
		}
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "No image provided"})
		return
	}
	file, err := header.Open()
	if err != nil {
		log.Printf("ERROR ScanReceipt: opening upload: %v", err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to read image"})
		return
	}
	defer file.Close()

	if header.Size > MaxReceiptBytes {
		c.JSON(http.StatusRequestEntityTooLarge, models.APIResponse{Success: false, Error: "Image too large (max 14 MB)"})
		return
	}

	mime_type, err := receiptMimeType(file, header)
	if err != nil {
		c.JSON(http.StatusUnsupportedMediaType, models.APIResponse{Success: false, Error: "File must be an image"})
		return
	}

	body, err := buildGeminiRequest(file, header.Size, mime_type)
	if err != nil {
		log.Printf("ERROR ScanReceipt: building request: %v", err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to read image"})
		return
	}

	// Call Gemini API
	geminiURL := fmt.Sprintf("%s?key=%s", geminiEndpoint, apiKey)
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, geminiURL, body)
	if err != nil {
		log.Printf("ERROR ScanReceipt: building Gemini request: %v", err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to build AI request"})
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := geminiClient.Do(req)
	if err != nil {
		log.Printf("ERROR ScanReceipt: calling Gemini: %v", err)
		c.JSON(http.StatusBadGateway, models.APIResponse{Success: false, Error: "Failed to reach AI service"})
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("ERROR ScanReceipt: reading Gemini response: %v", err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to read AI response"})
		return
	}

	if resp.StatusCode != 200 {
		log.Printf("ERROR ScanReceipt: Gemini returned %d: %s", resp.StatusCode, string(respBody))
		c.JSON(http.StatusBadGateway, models.APIResponse{Success: false, Error: fmt.Sprintf("AI service error: %d", resp.StatusCode)})
		return
	}

	// Parse Gemini response to extract the text
	var geminiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(respBody, &geminiResp); err != nil {
		log.Printf("ERROR ScanReceipt: parsing Gemini response: %v", err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to parse AI response"})
		return
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "AI returned empty response"})
		return
	}

	aiText := geminiResp.Candidates[0].Content.Parts[0].Text

	// Forward the AI's JSON text directly — frontend will parse it
	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: map[string]string{"text": aiText}})
}

// receiptMimeType sniffs the file's real type (the client-sent header is not trusted) and rewinds it
func receiptMimeType(file multipart.File, header *multipart.FileHeader) (string, error) {
	head := make([]byte, 512)
	n, err := io.ReadFull(file, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return "", err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	detected := http.DetectContentType(head[:n])
	if strings.HasPrefix(detected, "image/") {
		return detected, nil
	}
	// Go can't sniff HEIC/HEIF (iPhone photos); trust the declared type only for those
	declared := header.Header.Get("Content-Type")
	if declared == "image/heic" || declared == "image/heif" {
		return declared, nil
	}
	return "", fmt.Errorf("not an image: %s", detected)
}

// buildGeminiRequest streams the image into the JSON body as base64, so only one encoded
// copy is held in memory (no raw []byte, no second copy from json.Marshal)
func buildGeminiRequest(file io.Reader, size int64, mimeType string) (*bytes.Buffer, error) {
	mime_json, _ := json.Marshal(mimeType)
	prompt_json, _ := json.Marshal(receiptPrompt)

	var buf bytes.Buffer
	buf.Grow(base64.StdEncoding.EncodedLen(int(size)) + len(prompt_json) + 256)
	buf.WriteString(`{"contents":[{"parts":[{"inlineData":{"mimeType":`)
	buf.Write(mime_json)
	buf.WriteString(`,"data":"`)
	enc := base64.NewEncoder(base64.StdEncoding, &buf)
	if _, err := io.Copy(enc, file); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	buf.WriteString(`"}},{"text":`)
	buf.Write(prompt_json)
	buf.WriteString(`}]}],"generationConfig":{"temperature":0.1,"maxOutputTokens":2048}}`)
	return &buf, nil
}

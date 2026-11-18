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

// MaxReceiptBytes caps receipt image uploads to keep memory bounded on the 256 MB VM (a 14 MB scan
// peaks around 46 MB of heap). Bigger uploads get 413 before they're read. OpenAI's own request
// limit is far higher (512 MB), and the frontend shrinks photos over 4 MB before uploading anyway.
const MaxReceiptBytes = 14 << 20

// Room for multipart headers/boundaries on top of the image itself
const receiptFormOverhead = 1 << 20

// At most this many scans hold an image in memory at once (the VM has 256 MB); extra requests get 429
var receiptScanSlots = make(chan struct{}, 2)

// openaiEndpoint is a variable so tests can point it at a fake server
var openaiEndpoint = "https://api.openai.com/v1/responses"

// DefaultReceiptModel reads receipts unless OPENAI_RECEIPT_MODEL overrides it
const DefaultReceiptModel = "gpt-6-luna"

// Bounds how long a scan can hold its encoded image while waiting on the AI
var openaiClient = &http.Client{Timeout: 60 * time.Second}

// Image types OpenAI accepts. Anything else (e.g. HEIC) gets 415; the frontend converts those to JPEG first.
var receiptImageTypes = map[string]bool{
	"image/jpeg": true, "image/png": true, "image/webp": true, "image/gif": true,
}

// receiptPrompt tells the model how to read the receipt; receiptSchema enforces the answer's shape
const receiptPrompt = `You are a receipt parsing expert. Analyze this receipt image and extract structured data.
The receipt can be in ANY language (Italian, German, Dutch, English, French, etc.) — you must understand it regardless.

Rules:
- Extract ONLY purchased items/products with their prices
- Translate all item names to English
- Prices are numbers with . as decimal separator (3.50, not "3,50")
- "total" = the final total amount (e.g. Total, Totale, Summe, Gesamtbetrag, Totaal, etc.) — NOT an item
- NEVER include these as items: totals, subtotals, tax (IVA/MwSt/BTW/VAT), discounts, payment method, change, tips, or any summary lines
- If multiple totals exist, use the largest (grand total)
- If there is no clear total line, set total to null
- If there is no store name, set store_name to null`

// Strict JSON schema (Structured Outputs): every field required, nulls allowed where a value may be missing
const receiptSchema = `{
	"type": "object",
	"properties": {
		"store_name": {"type": ["string", "null"]},
		"items": {
			"type": "array",
			"items": {
				"type": "object",
				"properties": {
					"name": {"type": "string"},
					"price": {"type": "number"}
				},
				"required": ["name", "price"],
				"additionalProperties": false
			}
		},
		"total": {"type": ["number", "null"]}
	},
	"required": ["store_name", "items", "total"],
	"additionalProperties": false
}`

type ReceiptHandler struct{}

func NewReceiptHandler() *ReceiptHandler {
	return &ReceiptHandler{}
}

func receiptModel() string {
	if m := strings.TrimSpace(os.Getenv("OPENAI_RECEIPT_MODEL")); m != "" {
		return m
	}
	return DefaultReceiptModel
}

// ReceiptConfig returns whether AI scanning is available (OPENAI_API_KEY is set)
func (h *ReceiptHandler) ReceiptConfig(c *gin.Context) {
	enabled := os.Getenv("OPENAI_API_KEY") != ""
	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: map[string]bool{"enabled": enabled}})
}

// ScanReceipt sends a receipt image to OpenAI so the API key stays server-side
func (h *ReceiptHandler) ScanReceipt(c *gin.Context) {
	apiKey := os.Getenv("OPENAI_API_KEY")
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

	mime_type, err := receiptMimeType(file)
	if err != nil {
		c.JSON(http.StatusUnsupportedMediaType, models.APIResponse{Success: false, Error: "Unsupported image type (use JPEG, PNG, WEBP or GIF)"})
		return
	}

	body, err := buildOpenAIRequest(file, header.Size, mime_type, receiptModel())
	if err != nil {
		log.Printf("ERROR ScanReceipt: building request: %v", err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to read image"})
		return
	}

	// The key goes in a header, never the URL (URLs end up in error logs)
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, openaiEndpoint, body)
	if err != nil {
		log.Printf("ERROR ScanReceipt: building OpenAI request: %v", err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to build AI request"})
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := openaiClient.Do(req)
	if err != nil {
		log.Printf("ERROR ScanReceipt: calling OpenAI: %v", err)
		c.JSON(http.StatusBadGateway, models.APIResponse{Success: false, Error: "Failed to reach AI service"})
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		log.Printf("ERROR ScanReceipt: reading OpenAI response: %v", err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to read AI response"})
		return
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("ERROR ScanReceipt: OpenAI returned %d: %s", resp.StatusCode, string(respBody))
		status := http.StatusBadGateway
		if resp.StatusCode == http.StatusTooManyRequests {
			status = http.StatusTooManyRequests // rate/quota limit: the frontend falls back to on-device OCR
		}
		c.JSON(status, models.APIResponse{Success: false, Error: fmt.Sprintf("AI service error: %d", resp.StatusCode)})
		return
	}

	text, refusal, err := openAIOutputText(respBody)
	if err != nil {
		log.Printf("ERROR ScanReceipt: parsing OpenAI response: %v", err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to parse AI response"})
		return
	}
	if refusal != "" {
		log.Printf("ScanReceipt: model refused: %s", refusal)
		c.JSON(http.StatusUnprocessableEntity, models.APIResponse{Success: false, Error: "The AI couldn't read this image as a receipt"})
		return
	}
	if text == "" {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "AI returned empty response"})
		return
	}

	// Forward the AI's JSON text directly — the frontend parses it
	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: map[string]string{"text": text}})
}

// openAIOutputText pulls the answer out of a Responses API reply: the text of the "output_text"
// parts of the message output items (or a refusal). A top-level "output_text" is used if present.
func openAIOutputText(body []byte) (text, refusal string, err error) {
	var r struct {
		OutputText string `json:"output_text"`
		Output     []struct {
			Type    string `json:"type"`
			Content []struct {
				Type    string `json:"type"`
				Text    string `json:"text"`
				Refusal string `json:"refusal"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return "", "", err
	}
	if r.OutputText != "" {
		return r.OutputText, "", nil
	}
	var sb strings.Builder
	for _, item := range r.Output {
		if item.Type != "message" {
			continue // e.g. reasoning items
		}
		for _, part := range item.Content {
			switch part.Type {
			case "output_text":
				sb.WriteString(part.Text)
			case "refusal":
				refusal = part.Refusal
			}
		}
	}
	return sb.String(), refusal, nil
}

// receiptMimeType sniffs the file's real type (the client-sent header is not trusted) and rewinds it
func receiptMimeType(file multipart.File) (string, error) {
	head := make([]byte, 512)
	n, err := io.ReadFull(file, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return "", err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	detected := http.DetectContentType(head[:n])
	if !receiptImageTypes[detected] {
		return "", fmt.Errorf("unsupported image type: %s", detected)
	}
	return detected, nil
}

// buildOpenAIRequest streams the image into the JSON body as a base64 data URL, so only one encoded
// copy is held in memory (no raw []byte, no second copy from json.Marshal)
func buildOpenAIRequest(file io.Reader, size int64, mimeType, model string) (*bytes.Buffer, error) {
	model_json, _ := json.Marshal(model)
	prompt_json, _ := json.Marshal(receiptPrompt)
	var schema bytes.Buffer
	if err := json.Compact(&schema, []byte(receiptSchema)); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	buf.Grow(base64.StdEncoding.EncodedLen(int(size)) + len(prompt_json) + schema.Len() + 512)
	buf.WriteString(`{"model":`)
	buf.Write(model_json)
	buf.WriteString(`,"text":{"format":{"type":"json_schema","name":"receipt","strict":true,"schema":`)
	buf.Write(schema.Bytes())
	buf.WriteString(`}},"input":[{"role":"user","content":[{"type":"input_text","text":`)
	buf.Write(prompt_json)
	// "high" detail: receipt text is small
	buf.WriteString(`},{"type":"input_image","detail":"high","image_url":"data:` + mimeType + `;base64,`)
	enc := base64.NewEncoder(base64.StdEncoding, &buf)
	if _, err := io.Copy(enc, file); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	buf.WriteString(`"}]}]}`)
	return &buf, nil
}

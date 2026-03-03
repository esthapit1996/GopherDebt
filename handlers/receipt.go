package handlers

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"

	"gopherdebt/models"
)

type ReceiptHandler struct{}

func NewReceiptHandler() *ReceiptHandler {
	return &ReceiptHandler{}
}

// ScanReceipt proxies a receipt image to Gemini API so the API key stays server-side
func (h *ReceiptHandler) ScanReceipt(c *gin.Context) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		c.JSON(http.StatusServiceUnavailable, models.APIResponse{Success: false, Error: "AI receipt scanning not configured"})
		return
	}

	// Read the uploaded image
	file, header, err := c.Request.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{Success: false, Error: "No image provided"})
		return
	}
	defer file.Close()

	// Read file bytes
	imageBytes, err := io.ReadAll(file)
	if err != nil {
		log.Printf("ERROR ScanReceipt: reading file: %v", err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to read image"})
		return
	}

	// Determine mime type
	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "image/jpeg"
	}

	// Build Gemini request
	geminiReq := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{
						"inlineData": map[string]interface{}{
							"mimeType": mimeType,
							"data":     base64.StdEncoding.EncodeToString(imageBytes),
						},
					},
					{
						"text": `You are a receipt parsing expert. Analyze this receipt image and extract structured data.
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
- Return ONLY the JSON object, no markdown fences, no explanation`,
					},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature":     0.1,
			"maxOutputTokens": 2048,
		},
	}

	body, err := json.Marshal(geminiReq)
	if err != nil {
		log.Printf("ERROR ScanReceipt: marshaling request: %v", err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Error: "Failed to build AI request"})
		return
	}

	// Call Gemini API
	geminiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent?key=%s", apiKey)
	resp, err := http.Post(geminiURL, "application/json", bytes.NewReader(body))
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

package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type CurrencyHandler struct {
	cache        map[string]*rateCache
	historyCache map[string]*historyCache
	cacheMutex   sync.RWMutex
}

type rateCache struct {
	rates     map[string]float64
	fetchedAt time.Time
}

type historyCache struct {
	data      []HistoricalRate
	fetchedAt time.Time
}

type HistoricalRate struct {
	Date string  `json:"date"`
	Rate float64 `json:"rate"`
}

type exchangeRateResponse struct {
	Result string             `json:"result"`
	Rates  map[string]float64 `json:"rates"`
}

type frankfurterTimeSeriesResponse struct {
	Base  string                        `json:"base"`
	Rates map[string]map[string]float64 `json:"rates"`
}

func NewCurrencyHandler() *CurrencyHandler {
	return &CurrencyHandler{
		cache:        make(map[string]*rateCache),
		historyCache: make(map[string]*historyCache),
	}
}

// GetRates returns cached or freshly fetched exchange rates for a base currency
func (h *CurrencyHandler) GetRates(c *gin.Context) {
	base := c.DefaultQuery("base", "USD")

	rates, err := h.fetchRates(base)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Failed to fetch exchange rates"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"base":  base,
		"rates": rates,
	})
}

// Convert converts an amount from one currency to another
func (h *CurrencyHandler) Convert(c *gin.Context) {
	from := c.Query("from")
	to := c.Query("to")
	amountStr := c.Query("amount")

	if from == "" || to == "" || amountStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing required parameters: from, to, amount"})
		return
	}

	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid amount"})
		return
	}

	rates, err := h.fetchRates(from)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Failed to fetch exchange rates"})
		return
	}

	rate, ok := rates[to]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown target currency"})
		return
	}

	converted := amount * rate

	c.JSON(http.StatusOK, gin.H{
		"from":      from,
		"to":        to,
		"amount":    amount,
		"rate":      rate,
		"converted": converted,
	})
}

func (h *CurrencyHandler) fetchRates(base string) (map[string]float64, error) {
	// Check cache first (valid for 1 hour)
	h.cacheMutex.RLock()
	if cached, ok := h.cache[base]; ok && time.Since(cached.fetchedAt) < time.Hour {
		h.cacheMutex.RUnlock()
		return cached.rates, nil
	}
	h.cacheMutex.RUnlock()

	// Fetch from API
	url := fmt.Sprintf("https://open.er-api.com/v6/latest/%s", base)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result exchangeRateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if result.Result != "success" {
		return nil, fmt.Errorf("API returned non-success result")
	}

	// Cache the result
	h.cacheMutex.Lock()
	h.cache[base] = &rateCache{
		rates:     result.Rates,
		fetchedAt: time.Now(),
	}
	h.cacheMutex.Unlock()

	return result.Rates, nil
}

// GetHistory returns historical exchange rates from Frankfurter API
func (h *CurrencyHandler) GetHistory(c *gin.Context) {
	from := c.Query("from")
	to := c.Query("to")
	daysStr := c.DefaultQuery("days", "7")

	if from == "" || to == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing required parameters: from, to"})
		return
	}

	days, err := strconv.Atoi(daysStr)
	if err != nil || days < 1 || days > 365 {
		days = 7
	}

	// Create cache key
	cacheKey := fmt.Sprintf("%s-%s-%d", from, to, days)

	// Check cache first (valid for 24 hours - historical data doesn't change)
	h.cacheMutex.RLock()
	if cached, ok := h.historyCache[cacheKey]; ok && time.Since(cached.fetchedAt) < 24*time.Hour {
		h.cacheMutex.RUnlock()
		c.JSON(http.StatusOK, gin.H{
			"from":    from,
			"to":      to,
			"days":    days,
			"history": cached.data,
		})
		return
	}
	h.cacheMutex.RUnlock()

	// Calculate date range
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -days)

	// Fetch from Frankfurter API
	url := fmt.Sprintf("https://api.frankfurter.app/%s..%s?from=%s&to=%s",
		startDate.Format("2006-01-02"),
		endDate.Format("2006-01-02"),
		from,
		to,
	)

	resp, err := http.Get(url)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Failed to fetch historical rates"})
		return
	}
	defer resp.Body.Close()

	var result frankfurterTimeSeriesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Failed to parse historical rates"})
		return
	}

	// If Frankfurter returned no rates, the currency pair is not supported
	if len(result.Rates) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"from":    from,
			"to":      to,
			"days":    days,
			"history": []HistoricalRate{},
			"error":   fmt.Sprintf("Historical data not available — Frankfurter (ECB) does not support %s and/or %s", from, to),
		})
		return
	}

	// Convert to sorted slice
	var history []HistoricalRate
	for date, rates := range result.Rates {
		if rate, ok := rates[to]; ok {
			history = append(history, HistoricalRate{
				Date: date,
				Rate: rate,
			})
		}
	}

	// Sort by date
	sort.Slice(history, func(i, j int) bool {
		return history[i].Date < history[j].Date
	})

	// Cache the result
	h.cacheMutex.Lock()
	h.historyCache[cacheKey] = &historyCache{
		data:      history,
		fetchedAt: time.Now(),
	}
	h.cacheMutex.Unlock()

	c.JSON(http.StatusOK, gin.H{
		"from":    from,
		"to":      to,
		"days":    days,
		"history": history,
	})
}

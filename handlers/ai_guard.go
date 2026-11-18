package handlers

import (
	"database/sql"
	"errors"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopherdebt/db"
)

// AI guardrails, so OpenAI is only called when it's useful and never without a limit:
//   - a daily limit of AI calls per user and kind (counted in the ai_usage table)
//   - every call's real token usage is logged
//   - wish link lookups are cached for 24 h (see lookupCache)
//   - wish pages without any price-looking text never go to the AI (see hasPriceText)

// AI call kinds and their default daily limits per user (override with the env vars)
const (
	aiKindReceipt = "receipt" // AI_DAILY_RECEIPT_SCANS, default 20
	aiKindWish    = "wish"    // AI_DAILY_WISH_LOOKUPS, default 30 (page reading + name shortening)
)

var errAIQuota = errors.New("daily AI limit reached")

// takeAIQuota is a variable so tests can run without a database
var takeAIQuota = db.TakeAIQuota

func aiDailyLimit(kind string) int {
	env, def := "AI_DAILY_WISH_LOOKUPS", 30
	if kind == aiKindReceipt {
		env, def = "AI_DAILY_RECEIPT_SCANS", 20
	}
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(env))); err == nil && v >= 0 {
		return v
	}
	return def
}

// allowAICall takes one call from the user's daily allowance. Errors fail closed (no AI call).
func allowAICall(d *sql.DB, userID int, kind string) error {
	ok, err := takeAIQuota(d, userID, kind, aiDailyLimit(kind))
	if err != nil {
		log.Printf("ERROR AI quota: user %d, %s: %v", userID, kind, err)
		return err
	}
	if !ok {
		log.Printf("AI quota: user %d reached the daily %s limit (%d)", userID, kind, aiDailyLimit(kind))
		return errAIQuota
	}
	return nil
}

// openAIUsage is the "usage" block of a Responses API reply
type openAIUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

func logAIUsage(kind string, userID int, model string, u openAIUsage) {
	log.Printf("AI usage: %s user %d model %s: %d input + %d output tokens", kind, userID, model, u.InputTokens, u.OutputTokens)
}

// lookupCache remembers successful wish link lookups for 24 h, so the same link costs nothing twice
var lookupCache = struct {
	sync.Mutex
	entries map[string]lookupCacheEntry
}{entries: map[string]lookupCacheEntry{}}

type lookupCacheEntry struct {
	result wishLookupResult
	at     time.Time
}

const (
	lookupCacheTTL = 24 * time.Hour
	lookupCacheMax = 500
)

func cachedLookup(key string) (wishLookupResult, bool) {
	lookupCache.Lock()
	defer lookupCache.Unlock()
	e, ok := lookupCache.entries[key]
	if !ok || time.Since(e.at) > lookupCacheTTL {
		return wishLookupResult{}, false
	}
	return e.result, true
}

func storeLookup(key string, r wishLookupResult) {
	// Only complete answers: blocked/unreachable pages, or a missing name/price (AI limit, AI error),
	// may work next time
	if r.Status != lookupOK || r.Name == "" || r.Price == nil || r.AILimit {
		return
	}
	lookupCache.Lock()
	defer lookupCache.Unlock()
	if len(lookupCache.entries) >= lookupCacheMax {
		for k, e := range lookupCache.entries {
			if time.Since(e.at) > lookupCacheTTL {
				delete(lookupCache.entries, k)
			}
		}
		for k := range lookupCache.entries { // still full: drop an arbitrary entry
			if len(lookupCache.entries) < lookupCacheMax {
				break
			}
			delete(lookupCache.entries, k)
		}
	}
	lookupCache.entries[key] = lookupCacheEntry{result: r, at: time.Now()}
}

func resetLookupCache() {
	lookupCache.Lock()
	lookupCache.entries = map[string]lookupCacheEntry{}
	lookupCache.Unlock()
}

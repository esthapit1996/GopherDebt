package handlers

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gopherdebt/db"
	"gopherdebt/gocrumbs"
	"gopherdebt/models"
)

type GoCrumbsHandler struct {
	DB *sql.DB
}

func NewGoCrumbsHandler(database *sql.DB) *GoCrumbsHandler {
	return &GoCrumbsHandler{DB: database}
}

// goCrumbsTaxYear is the latest tax year the engine supports that is not after now. Before the
// first supported year it uses the first one, so the result's tax_year always says what was used.
func goCrumbsTaxYear(now time.Time) int {
	years := gocrumbs.SupportedYears()
	year := years[0]
	for _, y := range years {
		if y <= now.Year() && y > year {
			year = y
		}
	}
	return year
}

// bindGoCrumbs parses and validates a GoCrumbs payload, writing a 400 on failure
func bindGoCrumbs(c *gin.Context) (*models.GoCrumbsRequest, bool) {
	var req models.GoCrumbsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   "Invalid request: " + err.Error(),
		})
		return nil, false
	}

	for _, lines := range [][]models.GoCrumbsExpenseLine{req.Subscriptions, req.CustomExpenses} {
		for i := range lines {
			lines[i].Name = strings.TrimSpace(lines[i].Name)
			lines[i].Category = strings.TrimSpace(lines[i].Category)
			if lines[i].Name == "" {
				c.JSON(http.StatusBadRequest, models.APIResponse{
					Success: false,
					Error:   "Expense name is required",
				})
				return nil, false
			}
		}
	}
	if len(req.Subscriptions)+len(req.CustomExpenses) > db.MaxGoCrumbsLines {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   "Too many expense lines",
		})
		return nil, false
	}
	return &req, true
}

// goCrumbsInput turns a profile into engine input; yearly gross is converted to monthly
func goCrumbsInput(req *models.GoCrumbsRequest, year int) gocrumbs.Input {
	monthly := req.GrossAmount
	if req.GrossPeriod == "year" {
		monthly /= 12
	}
	lines := func(in []models.GoCrumbsExpenseLine) []gocrumbs.Expense {
		out := make([]gocrumbs.Expense, len(in))
		for i, l := range in {
			out[i] = gocrumbs.Expense{Name: l.Name, Amount: l.Amount, Category: l.Category}
		}
		return out
	}
	return gocrumbs.Input{
		Year:             year,
		WorkType:         req.WorkType,
		GrossMain:        monthly,
		GrossSide:        req.SideGross,
		SteuerKlasse:     req.SteuerKlasse,
		ChurchTax:        req.ChurchTax,
		Bundesland:       req.Bundesland,
		Children:         req.Children,
		Under23:          req.Under23,
		ZusatzbeitragPct: req.Zusatzbeitrag,
		Rent:             req.Rent,
		Savings:          req.Savings,
		Subscriptions:    lines(req.Subscriptions),
		Custom:           lines(req.CustomExpenses),
	}
}

// calculateGoCrumbs runs the engine for a profile, writing a 400/500 on failure
func calculateGoCrumbs(c *gin.Context, req *models.GoCrumbsRequest) (gocrumbs.Result, bool) {
	res, err := gocrumbs.Calculate(goCrumbsInput(req, goCrumbsTaxYear(time.Now())))
	if errors.Is(err, gocrumbs.ErrInvalidInput) {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Error:   "Invalid request: " + strings.TrimPrefix(err.Error(), gocrumbs.ErrInvalidInput.Error()+": "),
		})
		return res, false
	}
	if err != nil {
		log.Printf("ERROR GoCrumbs calculate: %v", err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   "Failed to calculate",
		})
		return res, false
	}
	return res, true
}

// GetGoCrumbs returns the user's saved profile; a first-time user gets success without data
func (h *GoCrumbsHandler) GetGoCrumbs(c *gin.Context) {
	userID := c.GetInt("userID")

	profile, err := db.GetGoCrumbsProfile(h.DB, userID)
	if err == db.ErrNotFound {
		c.JSON(http.StatusOK, models.APIResponse{Success: true})
		return
	}
	if err != nil {
		log.Printf("ERROR GetGoCrumbs: user %d: %v", userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   "Failed to load your budget",
		})
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: profile})
}

// SaveGoCrumbs creates or replaces the user's profile. The profile must be calculable, so every
// saved profile can be shown again.
func (h *GoCrumbsHandler) SaveGoCrumbs(c *gin.Context) {
	userID := c.GetInt("userID")

	req, ok := bindGoCrumbs(c)
	if !ok {
		return
	}
	if _, ok := calculateGoCrumbs(c, req); !ok {
		return
	}

	profile, err := db.SaveGoCrumbsProfile(h.DB, userID, req)
	if err != nil {
		log.Printf("ERROR SaveGoCrumbs: user %d: %v", userID, err)
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Error:   "Failed to save your budget",
		})
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: profile})
}

// CalculateGoCrumbs computes a result for the given inputs without saving anything
func (h *GoCrumbsHandler) CalculateGoCrumbs(c *gin.Context) {
	req, ok := bindGoCrumbs(c)
	if !ok {
		return
	}
	res, ok := calculateGoCrumbs(c, req)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{Success: true, Data: res})
}

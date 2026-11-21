package db

import (
	"database/sql"

	"gopherdebt/models"
)

const (
	// MaxGoCrumbsLines is the most expense lines (subscriptions + custom) one profile may hold
	MaxGoCrumbsLines = 50
	// MaxGoCrumbsExtraPayLines is the most extra pay lines one profile may hold
	MaxGoCrumbsExtraPayLines = 20
)

const goCrumbsColumns = `work_type, gross_amount, gross_period, side_gross, steuerklasse, church_tax,
	bundesland, children, under_23, zusatzbeitrag, rent, savings, ws_hourly_wage, ws_hours, ws_hours_period, updated_at`

// GetGoCrumbsProfile returns the user's saved profile, or ErrNotFound if none was saved yet
func GetGoCrumbsProfile(d *sql.DB, userID int) (*models.GoCrumbsProfile, error) {
	return retry("GetGoCrumbsProfile", func() (*models.GoCrumbsProfile, error) {
		p := models.GoCrumbsProfile{}
		err := d.QueryRow(
			`SELECT `+goCrumbsColumns+` FROM gocrumbs_profiles WHERE user_id = $1`, userID,
		).Scan(&p.WorkType, &p.GrossAmount, &p.GrossPeriod, &p.SideGross, &p.SteuerKlasse, &p.ChurchTax,
			&p.Bundesland, &p.Children, &p.Under23, &p.Zusatzbeitrag, &p.Rent, &p.Savings,
			&p.WsHourlyWage, &p.WsHours, &p.WsHoursPeriod, &p.UpdatedAt)
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}

		// Empty (not nil) slices so the API returns [] instead of null
		p.Subscriptions = []models.GoCrumbsExpenseLine{}
		p.CustomExpenses = []models.GoCrumbsExpenseLine{}
		rows, err := d.Query(
			`SELECT kind, name, amount, category FROM gocrumbs_expenses
			 WHERE user_id = $1 ORDER BY kind, position, id`, userID,
		)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var kind string
			var line models.GoCrumbsExpenseLine
			if err := rows.Scan(&kind, &line.Name, &line.Amount, &line.Category); err != nil {
				return nil, err
			}
			if kind == "subscription" {
				p.Subscriptions = append(p.Subscriptions, line)
			} else {
				p.CustomExpenses = append(p.CustomExpenses, line)
			}
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}

		p.ExtraPay = []models.GoCrumbsExtraPayLine{}
		extraRows, err := d.Query(
			`SELECT name, amount, tax_free FROM gocrumbs_extra_pay
			 WHERE user_id = $1 ORDER BY position, id`, userID,
		)
		if err != nil {
			return nil, err
		}
		defer extraRows.Close()
		for extraRows.Next() {
			var line models.GoCrumbsExtraPayLine
			if err := extraRows.Scan(&line.Name, &line.Amount, &line.TaxFree); err != nil {
				return nil, err
			}
			p.ExtraPay = append(p.ExtraPay, line)
		}
		return &p, extraRows.Err()
	})
}

// SaveGoCrumbsProfile creates or replaces the user's profile, expense lines and extra pay lines in one
// transaction. The profile upsert comes first and holds the row lock until commit, so two
// parallel saves by the same user run one after the other and can't duplicate expense lines.
func SaveGoCrumbsProfile(d *sql.DB, userID int, req *models.GoCrumbsRequest) (*models.GoCrumbsProfile, error) {
	tx, err := d.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		`INSERT INTO gocrumbs_profiles (user_id, work_type, gross_amount, gross_period, side_gross, steuerklasse,
			church_tax, bundesland, children, under_23, zusatzbeitrag, rent, savings,
			ws_hourly_wage, ws_hours, ws_hours_period, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, CURRENT_TIMESTAMP)
		 ON CONFLICT (user_id) DO UPDATE SET work_type = $2, gross_amount = $3, gross_period = $4,
			side_gross = $5, steuerklasse = $6, church_tax = $7, bundesland = $8, children = $9,
			under_23 = $10, zusatzbeitrag = $11, rent = $12, savings = $13, ws_hourly_wage = $14,
			ws_hours = $15, ws_hours_period = $16, updated_at = CURRENT_TIMESTAMP`,
		userID, req.WorkType, req.GrossAmount, req.GrossPeriod, req.SideGross, req.SteuerKlasse,
		req.ChurchTax, req.Bundesland, req.Children, req.Under23, req.Zusatzbeitrag, req.Rent, req.Savings,
		req.WsHourlyWage, req.WsHours, req.WsHoursPeriod,
	)
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(`DELETE FROM gocrumbs_expenses WHERE user_id = $1`, userID); err != nil {
		return nil, err
	}
	groups := []struct {
		kind  string
		lines []models.GoCrumbsExpenseLine
	}{{"subscription", req.Subscriptions}, {"custom", req.CustomExpenses}}
	for _, g := range groups {
		for i, line := range g.lines {
			_, err := tx.Exec(
				`INSERT INTO gocrumbs_expenses (user_id, kind, name, amount, category, position)
				 VALUES ($1, $2, $3, $4, $5, $6)`,
				userID, g.kind, line.Name, line.Amount, line.Category, i,
			)
			if err != nil {
				return nil, err
			}
		}
	}

	if _, err := tx.Exec(`DELETE FROM gocrumbs_extra_pay WHERE user_id = $1`, userID); err != nil {
		return nil, err
	}
	for i, line := range req.ExtraPay {
		_, err := tx.Exec(
			`INSERT INTO gocrumbs_extra_pay (user_id, name, amount, tax_free, position)
			 VALUES ($1, $2, $3, $4, $5)`,
			userID, line.Name, line.Amount, line.TaxFree, i,
		)
		if err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return GetGoCrumbsProfile(d, userID)
}

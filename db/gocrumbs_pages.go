package db

import (
	"database/sql"
	"errors"

	"github.com/lib/pq"

	"gopherdebt/models"
)

// MaxGoCrumbsPages is the most pages one user may have per work type
const MaxGoCrumbsPages = 15

var (
	// ErrGoCrumbsPageLimit is returned when the user already has MaxGoCrumbsPages of that work type
	ErrGoCrumbsPageLimit = errors.New("gocrumbs page limit reached")
	// ErrGoCrumbsWorkTypeLocked is returned when a save would change a page's work type
	ErrGoCrumbsWorkTypeLocked = errors.New("gocrumbs page work type can't change")
)

// goCrumbsPagesCopiedFlag marks the one-time copy of the single profiles into pages as done
const goCrumbsPagesCopiedFlag = "gocrumbs_profiles_copied_to_pages"

// goCrumbsCopiedPageName is the name of the page a user's old single profile becomes
const goCrumbsCopiedPageName = "My budget"

const goCrumbsPageColumns = `id, name, work_type, gross_amount, gross_period, side_gross, steuerklasse, church_tax,
	bundesland, children, under_23, zusatzbeitrag, rent, savings, ws_hourly_wage, ws_hours, ws_hours_period,
	created_at, updated_at`

// copyGoCrumbsProfilesToPages copies every single GoCrumbs profile, with its expense and extra
// pay lines, into a page named goCrumbsCopiedPageName. It runs once: the flag row is inserted in
// the same transaction, so a second run (or a second machine starting at the same time, which
// waits on the flag's primary key) copies nothing. The source tables are only read.
func copyGoCrumbsProfilesToPages(d *sql.DB) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`INSERT INTO migration_flags (name) VALUES ($1) ON CONFLICT (name) DO NOTHING`, goCrumbsPagesCopiedFlag)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return err // already copied
	}

	// Before this flag existed no page could be created, so every page here is a fresh copy and
	// joining lines on user_id is exact (one page per user)
	steps := []string{
		`INSERT INTO gocrumbs_pages (user_id, name, work_type, gross_amount, gross_period, side_gross, steuerklasse,
			church_tax, bundesland, children, under_23, zusatzbeitrag, rent, savings,
			ws_hourly_wage, ws_hours, ws_hours_period, created_at, updated_at)
		 SELECT user_id, '` + goCrumbsCopiedPageName + `', work_type, gross_amount, gross_period, side_gross, steuerklasse,
			church_tax, bundesland, children, under_23, zusatzbeitrag, rent, savings,
			ws_hourly_wage, ws_hours, ws_hours_period,
			COALESCE(updated_at, CURRENT_TIMESTAMP), COALESCE(updated_at, CURRENT_TIMESTAMP)
		 FROM gocrumbs_profiles`,
		`INSERT INTO gocrumbs_page_expenses (page_id, kind, name, amount, category, position)
		 SELECT p.id, e.kind, e.name, e.amount, e.category, e.position
		 FROM gocrumbs_expenses e JOIN gocrumbs_pages p ON p.user_id = e.user_id
		 ORDER BY e.id`,
		`INSERT INTO gocrumbs_page_extra_pay (page_id, name, amount, tax_free, position)
		 SELECT p.id, x.name, x.amount, x.tax_free, x.position
		 FROM gocrumbs_extra_pay x JOIN gocrumbs_pages p ON p.user_id = x.user_id
		 ORDER BY x.id`,
	}
	for _, step := range steps {
		if _, err := tx.Exec(step); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListGoCrumbsPages returns the user's pages (both work types), oldest first
func ListGoCrumbsPages(d *sql.DB, userID int) ([]models.GoCrumbsPageSummary, error) {
	return retry("ListGoCrumbsPages", func() ([]models.GoCrumbsPageSummary, error) {
		rows, err := d.Query(
			`SELECT id, name, work_type, updated_at FROM gocrumbs_pages
			 WHERE user_id = $1 ORDER BY created_at, id`, userID,
		)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		pages := []models.GoCrumbsPageSummary{}
		for rows.Next() {
			var p models.GoCrumbsPageSummary
			if err := rows.Scan(&p.ID, &p.Name, &p.WorkType, &p.UpdatedAt); err != nil {
				return nil, err
			}
			pages = append(pages, p)
		}
		return pages, rows.Err()
	})
}

// GetGoCrumbsPage returns one of the user's pages, or ErrNotFound (also for another user's page)
func GetGoCrumbsPage(d *sql.DB, userID, pageID int) (*models.GoCrumbsPage, error) {
	return retry("GetGoCrumbsPage", func() (*models.GoCrumbsPage, error) {
		p := models.GoCrumbsPage{}
		err := d.QueryRow(
			`SELECT `+goCrumbsPageColumns+` FROM gocrumbs_pages WHERE id = $1 AND user_id = $2`, pageID, userID,
		).Scan(&p.ID, &p.Name, &p.WorkType, &p.GrossAmount, &p.GrossPeriod, &p.SideGross, &p.SteuerKlasse, &p.ChurchTax,
			&p.Bundesland, &p.Children, &p.Under23, &p.Zusatzbeitrag, &p.Rent, &p.Savings,
			&p.WsHourlyWage, &p.WsHours, &p.WsHoursPeriod, &p.CreatedAt, &p.UpdatedAt)
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
			`SELECT kind, name, amount, category FROM gocrumbs_page_expenses
			 WHERE page_id = $1 ORDER BY kind, position, id`, pageID,
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
			`SELECT name, amount, tax_free FROM gocrumbs_page_extra_pay
			 WHERE page_id = $1 ORDER BY position, id`, pageID,
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

// CreateGoCrumbsPage adds a page. The user's row is locked first, so two parallel creates are
// counted one after the other and can't go past MaxGoCrumbsPages. NO KEY UPDATE serializes the
// creates without blocking foreign-key checks on the user from other tables.
func CreateGoCrumbsPage(d *sql.DB, userID int, req *models.GoCrumbsPageRequest) (*models.GoCrumbsPage, error) {
	tx, err := d.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var locked int
	if err := tx.QueryRow(`SELECT id FROM users WHERE id = $1 FOR NO KEY UPDATE`, userID).Scan(&locked); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var count int
	err = tx.QueryRow(
		`SELECT COUNT(*) FROM gocrumbs_pages WHERE user_id = $1 AND work_type = $2`, userID, req.WorkType,
	).Scan(&count)
	if err != nil {
		return nil, err
	}
	if count >= MaxGoCrumbsPages {
		return nil, ErrGoCrumbsPageLimit
	}

	var pageID int
	err = tx.QueryRow(
		`INSERT INTO gocrumbs_pages (user_id, name, work_type, gross_amount, gross_period, side_gross, steuerklasse,
			church_tax, bundesland, children, under_23, zusatzbeitrag, rent, savings,
			ws_hourly_wage, ws_hours, ws_hours_period)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		 RETURNING id`,
		userID, req.Name, req.WorkType, req.GrossAmount, req.GrossPeriod, req.SideGross, req.SteuerKlasse,
		req.ChurchTax, req.Bundesland, req.Children, req.Under23, req.Zusatzbeitrag, req.Rent, req.Savings,
		req.WsHourlyWage, req.WsHours, req.WsHoursPeriod,
	).Scan(&pageID)
	if err != nil {
		return nil, duplicateName(err)
	}
	if err := writeGoCrumbsPageLines(tx, pageID, &req.GoCrumbsRequest); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return GetGoCrumbsPage(d, userID, pageID)
}

// UpdateGoCrumbsPage replaces a page's name, inputs and lines. The page row is locked first, so
// two parallel saves run one after the other and can't duplicate lines.
func UpdateGoCrumbsPage(d *sql.DB, userID, pageID int, req *models.GoCrumbsPageRequest) (*models.GoCrumbsPage, error) {
	tx, err := d.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var workType string
	err = tx.QueryRow(
		`SELECT work_type FROM gocrumbs_pages WHERE id = $1 AND user_id = $2 FOR UPDATE`, pageID, userID,
	).Scan(&workType)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if workType != req.WorkType {
		return nil, ErrGoCrumbsWorkTypeLocked
	}

	_, err = tx.Exec(
		`UPDATE gocrumbs_pages SET name = $2, gross_amount = $3, gross_period = $4, side_gross = $5,
			steuerklasse = $6, church_tax = $7, bundesland = $8, children = $9, under_23 = $10,
			zusatzbeitrag = $11, rent = $12, savings = $13, ws_hourly_wage = $14, ws_hours = $15,
			ws_hours_period = $16, updated_at = CURRENT_TIMESTAMP
		 WHERE id = $1`,
		pageID, req.Name, req.GrossAmount, req.GrossPeriod, req.SideGross, req.SteuerKlasse, req.ChurchTax,
		req.Bundesland, req.Children, req.Under23, req.Zusatzbeitrag, req.Rent, req.Savings,
		req.WsHourlyWage, req.WsHours, req.WsHoursPeriod,
	)
	if err != nil {
		return nil, duplicateName(err)
	}
	if _, err := tx.Exec(`DELETE FROM gocrumbs_page_expenses WHERE page_id = $1`, pageID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`DELETE FROM gocrumbs_page_extra_pay WHERE page_id = $1`, pageID); err != nil {
		return nil, err
	}
	if err := writeGoCrumbsPageLines(tx, pageID, &req.GoCrumbsRequest); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return GetGoCrumbsPage(d, userID, pageID)
}

// DeleteGoCrumbsPage deletes one of the user's pages and its lines, or returns ErrNotFound
func DeleteGoCrumbsPage(d *sql.DB, userID, pageID int) error {
	res, err := d.Exec(`DELETE FROM gocrumbs_pages WHERE id = $1 AND user_id = $2`, pageID, userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// writeGoCrumbsPageLines inserts a page's expense and extra pay lines in their given order
func writeGoCrumbsPageLines(tx *sql.Tx, pageID int, req *models.GoCrumbsRequest) error {
	groups := []struct {
		kind  string
		lines []models.GoCrumbsExpenseLine
	}{{"subscription", req.Subscriptions}, {"custom", req.CustomExpenses}}
	for _, g := range groups {
		for i, line := range g.lines {
			_, err := tx.Exec(
				`INSERT INTO gocrumbs_page_expenses (page_id, kind, name, amount, category, position)
				 VALUES ($1, $2, $3, $4, $5, $6)`,
				pageID, g.kind, line.Name, line.Amount, line.Category, i,
			)
			if err != nil {
				return err
			}
		}
	}
	for i, line := range req.ExtraPay {
		_, err := tx.Exec(
			`INSERT INTO gocrumbs_page_extra_pay (page_id, name, amount, tax_free, position)
			 VALUES ($1, $2, $3, $4, $5)`,
			pageID, line.Name, line.Amount, line.TaxFree, i,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// duplicateName turns a unique violation (the page name index) into ErrDuplicate
func duplicateName(err error) error {
	var pq_err *pq.Error
	if errors.As(err, &pq_err) && pq_err.Code == "23505" {
		return ErrDuplicate
	}
	return err
}

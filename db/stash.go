package db

import (
	"database/sql"

	"gopherdebt/models"
)

// CreateStashExpense adds a personal expense to the user's stash
func CreateStashExpense(db *sql.DB, userID int, amount float64, description, category string) (*models.StashExpense, error) {
	var expense models.StashExpense
	err := db.QueryRow(
		`INSERT INTO stash_expenses (user_id, amount, description, category)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, user_id, amount, description, category, created_at`,
		userID, amount, description, category,
	).Scan(&expense.ID, &expense.UserID, &expense.Amount, &expense.Description, &expense.Category, &expense.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &expense, nil
}

// GetStashExpenses returns all personal expenses for a user, newest first
func GetStashExpenses(d *sql.DB, userID int) ([]models.StashExpense, error) {
	return retry("GetStashExpenses", func() ([]models.StashExpense, error) {
		rows, err := d.Query(
			`SELECT id, user_id, amount, description, COALESCE(category, ''), created_at
			 FROM stash_expenses
			 WHERE user_id = $1
			 ORDER BY created_at DESC`,
			userID,
		)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var expenses []models.StashExpense
		for rows.Next() {
			var e models.StashExpense
			if err := rows.Scan(&e.ID, &e.UserID, &e.Amount, &e.Description, &e.Category, &e.CreatedAt); err != nil {
				return nil, err
			}
			expenses = append(expenses, e)
		}
		return expenses, rows.Err()
	})
}

// DeleteStashExpense removes a personal expense (only if owned by the user)
func DeleteStashExpense(db *sql.DB, expenseID, userID int) error {
	result, err := db.Exec(
		`DELETE FROM stash_expenses WHERE id = $1 AND user_id = $2`,
		expenseID, userID,
	)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateStashExpense updates an existing personal expense if it belongs to the user
func UpdateStashExpense(db *sql.DB, expenseID, userID int, amount float64, description, category string) (*models.StashExpense, error) {
	var expense models.StashExpense
	result, err := db.Exec(
		`UPDATE stash_expenses SET amount = $1, description = $2, category = $3 WHERE id = $4 AND user_id = $5`,
		amount, description, category, expenseID, userID,
	)
	if err != nil {
		return nil, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rows == 0 {
		return nil, ErrNotFound
	}

	err = db.QueryRow(
		`SELECT id, user_id, amount, description, COALESCE(category, ''), created_at
		 FROM stash_expenses
		 WHERE id = $1`,
		expenseID,
	).Scan(&expense.ID, &expense.UserID, &expense.Amount, &expense.Description, &expense.Category, &expense.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &expense, nil
}

// GetStashSummary returns the total spent and breakdown by category
func GetStashSummary(d *sql.DB, userID int) (*models.StashSummary, error) {
	return retry("GetStashSummary", func() (*models.StashSummary, error) {
		summary := &models.StashSummary{
			ByCategory: make(map[string]float64),
		}

		// Total + count
		err := d.QueryRow(
			`SELECT COALESCE(SUM(amount), 0), COUNT(*)
			 FROM stash_expenses
			 WHERE user_id = $1`,
			userID,
		).Scan(&summary.TotalSpent, &summary.ExpenseCount)
		if err != nil {
			return nil, err
		}

		// Breakdown by category
		rows, err := d.Query(
			`SELECT COALESCE(category, ''), SUM(amount)
			 FROM stash_expenses
			 WHERE user_id = $1
			 GROUP BY category`,
			userID,
		)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		for rows.Next() {
			var cat string
			var total float64
			if err := rows.Scan(&cat, &total); err != nil {
				return nil, err
			}
			if cat == "" {
				cat = "uncategorized"
			}
			summary.ByCategory[cat] = total
		}

		return summary, rows.Err()
	})
}

// ClearStashExpenses removes all personal expenses for a user
func ClearStashExpenses(db *sql.DB, userID int) error {
	_, err := db.Exec(`DELETE FROM stash_expenses WHERE user_id = $1`, userID)
	return err
}

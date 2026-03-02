package db

import (
	"database/sql"

	"gopherdebt/models"
)

func CreateExpense(db *sql.DB, groupID, paidBy int, amount float64, description, splitType string, splits []models.ExpenseSplitInput) (*models.Expense, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var expense models.Expense
	err = tx.QueryRow(
		`INSERT INTO expenses (group_id, paid_by, amount, description, split_type) VALUES ($1, $2, $3, $4, $5) RETURNING id, group_id, paid_by, amount, description, split_type, created_at, updated_at`,
		groupID, paidBy, amount, description, splitType,
	).Scan(&expense.ID, &expense.GroupID, &expense.PaidBy, &expense.Amount, &expense.Description, &expense.SplitType, &expense.CreatedAt, &expense.UpdatedAt)
	if err != nil {
		return nil, err
	}

	for _, split := range splits {
		_, err = tx.Exec(`INSERT INTO expense_splits (expense_id, user_id, amount) VALUES ($1, $2, $3)`, expense.ID, split.UserID, split.Amount)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &expense, nil
}

func GetExpenseByID(d *sql.DB, expenseID int) (*models.Expense, error) {
	return retry("GetExpenseByID", func() (*models.Expense, error) {
		var expense models.Expense
		err := d.QueryRow(
			`SELECT id, group_id, paid_by, amount, description, split_type, created_at, updated_at FROM expenses WHERE id = $1`,
			expenseID,
		).Scan(&expense.ID, &expense.GroupID, &expense.PaidBy, &expense.Amount, &expense.Description, &expense.SplitType, &expense.CreatedAt, &expense.UpdatedAt)
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}

		payer, err := GetUserByID(d, expense.PaidBy)
		if err == nil {
			expense.PaidByUser = payer
		}

		splits, err := GetExpenseSplits(d, expenseID)
		if err != nil {
			return nil, err
		}
		expense.Splits = splits
		return &expense, nil
	})
}

func GetGroupExpenses(d *sql.DB, groupID int) ([]models.Expense, error) {
	return retry("GetGroupExpenses", func() ([]models.Expense, error) {
		rows, err := d.Query(
			`SELECT e.id, e.group_id, e.paid_by, e.amount, e.description, e.split_type, e.created_at, e.updated_at, u.id, u.email, u.name, COALESCE(u.avatar, '')
			 FROM expenses e LEFT JOIN users u ON e.paid_by = u.id WHERE e.group_id = $1 ORDER BY e.created_at DESC`,
			groupID,
		)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var expenses []models.Expense
		for rows.Next() {
			var expense models.Expense
			var payer models.User
			if err := rows.Scan(&expense.ID, &expense.GroupID, &expense.PaidBy, &expense.Amount, &expense.Description, &expense.SplitType, &expense.CreatedAt, &expense.UpdatedAt, &payer.ID, &payer.Email, &payer.Name, &payer.Avatar); err != nil {
				return nil, err
			}
			expense.PaidByUser = &payer
			// Fetch splits for this expense
			splits, err := GetExpenseSplits(d, expense.ID)
			if err == nil {
				expense.Splits = splits
			}
			expenses = append(expenses, expense)
		}
		return expenses, rows.Err()
	})
}

func GetExpenseSplits(d *sql.DB, expenseID int) ([]models.ExpenseSplit, error) {
	return retry("GetExpenseSplits", func() ([]models.ExpenseSplit, error) {
		rows, err := d.Query(
			`SELECT es.id, es.expense_id, es.user_id, es.amount, u.id, u.email, u.name, COALESCE(u.avatar, '')
			 FROM expense_splits es LEFT JOIN users u ON es.user_id = u.id WHERE es.expense_id = $1`,
			expenseID,
		)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var splits []models.ExpenseSplit
		for rows.Next() {
			var split models.ExpenseSplit
			var user models.User
			if err := rows.Scan(&split.ID, &split.ExpenseID, &split.UserID, &split.Amount, &user.ID, &user.Email, &user.Name, &user.Avatar); err != nil {
				return nil, err
			}
			split.User = &user
			splits = append(splits, split)
		}
		return splits, rows.Err()
	})
}

// GetUnpaidExpensesForUser returns expenses in a group where the given user still owes money.
// An expense is "unpaid" for a user if their split amount minus their payments is > 0
// and they are not the one who originally paid for the expense.
func GetUnpaidExpensesForUser(d *sql.DB, groupID, userID int) ([]models.Expense, error) {
	return retry("GetUnpaidExpensesForUser", func() ([]models.Expense, error) {
		rows, err := d.Query(
			`SELECT e.id, e.group_id, e.paid_by, e.amount, e.description, e.split_type, e.created_at, e.updated_at,
				u.id, u.email, u.name, COALESCE(u.avatar, '')
			FROM expenses e
			LEFT JOIN users u ON e.paid_by = u.id
			INNER JOIN expense_splits es ON es.expense_id = e.id AND es.user_id = $2
			WHERE e.group_id = $1
			  AND e.paid_by != $2
			  AND es.amount - COALESCE((SELECT SUM(ep.amount) FROM expense_payments ep WHERE ep.expense_id = e.id AND ep.paid_by = $2), 0) > 0.01
			ORDER BY e.created_at DESC`,
			groupID, userID,
		)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var expenses []models.Expense
		for rows.Next() {
			var expense models.Expense
			var payer models.User
			if err := rows.Scan(&expense.ID, &expense.GroupID, &expense.PaidBy, &expense.Amount, &expense.Description, &expense.SplitType, &expense.CreatedAt, &expense.UpdatedAt, &payer.ID, &payer.Email, &payer.Name, &payer.Avatar); err != nil {
				return nil, err
			}
			expense.PaidByUser = &payer
			splits, err := GetExpenseSplits(d, expense.ID)
			if err == nil {
				expense.Splits = splits
			}
			expenses = append(expenses, expense)
		}
		return expenses, rows.Err()
	})
}

func DeleteExpense(db *sql.DB, expenseID int) error {
	result, err := db.Exec(`DELETE FROM expenses WHERE id = $1`, expenseID)
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

func DeleteAllGroupExpenses(db *sql.DB, groupID int) (int64, error) {
	result, err := db.Exec(`DELETE FROM expenses WHERE group_id = $1`, groupID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

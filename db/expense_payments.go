package db

import (
	"database/sql"

	"gopherdebt/models"
)

// CreateExpensePayment records a payment towards an expense
func CreateExpensePayment(db *sql.DB, expenseID, paidBy int, amount float64, note string) (*models.ExpensePayment, error) {
	var payment models.ExpensePayment
	err := db.QueryRow(
		`INSERT INTO expense_payments (expense_id, paid_by, amount, note) VALUES ($1, $2, $3, $4) RETURNING id, expense_id, paid_by, amount, note, created_at`,
		expenseID, paidBy, amount, note,
	).Scan(&payment.ID, &payment.ExpenseID, &payment.PaidBy, &payment.Amount, &payment.Note, &payment.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &payment, nil
}

// GetExpensePayments returns all payments for an expense
func GetExpensePayments(d *sql.DB, expenseID int) ([]models.ExpensePayment, error) {
	return retry("GetExpensePayments", func() ([]models.ExpensePayment, error) {
		rows, err := d.Query(
			`SELECT ep.id, ep.expense_id, ep.paid_by, ep.amount, COALESCE(ep.note, ''), ep.created_at,
				u.id, u.email, u.name, COALESCE(u.avatar, '')
			FROM expense_payments ep
			JOIN users u ON ep.paid_by = u.id
			WHERE ep.expense_id = $1
			ORDER BY ep.created_at DESC`,
			expenseID,
		)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var payments []models.ExpensePayment
		for rows.Next() {
			var payment models.ExpensePayment
			var user models.User
			err := rows.Scan(&payment.ID, &payment.ExpenseID, &payment.PaidBy, &payment.Amount, &payment.Note, &payment.CreatedAt,
				&user.ID, &user.Email, &user.Name, &user.Avatar)
			if err != nil {
				return nil, err
			}
			payment.PaidByUser = &user
			payments = append(payments, payment)
		}
		return payments, nil
	})
}

// GetTotalPaymentsForExpense returns the total amount paid towards an expense by a specific user
func GetTotalPaymentsForExpense(db *sql.DB, expenseID, userID int) (float64, error) {
	var total float64
	err := db.QueryRow(
		`SELECT COALESCE(SUM(amount), 0) FROM expense_payments WHERE expense_id = $1 AND paid_by = $2`,
		expenseID, userID,
	).Scan(&total)
	if err != nil {
		return 0, err
	}
	return total, nil
}

// GetAllPaymentsForExpense returns the total amount paid towards an expense by all users
func GetAllPaymentsForExpense(db *sql.DB, expenseID int) (float64, error) {
	var total float64
	err := db.QueryRow(
		`SELECT COALESCE(SUM(amount), 0) FROM expense_payments WHERE expense_id = $1`,
		expenseID,
	).Scan(&total)
	if err != nil {
		return 0, err
	}
	return total, nil
}

// DeleteExpensePayment deletes a payment record
func DeleteExpensePayment(db *sql.DB, paymentID int) error {
	result, err := db.Exec(`DELETE FROM expense_payments WHERE id = $1`, paymentID)
	if err != nil {
		return err
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// GetExpensePaymentByID returns a payment by ID
func GetExpensePaymentByID(d *sql.DB, paymentID int) (*models.ExpensePayment, error) {
	return retry("GetExpensePaymentByID", func() (*models.ExpensePayment, error) {
		var payment models.ExpensePayment
		err := d.QueryRow(
			`SELECT id, expense_id, paid_by, amount, COALESCE(note, ''), created_at FROM expense_payments WHERE id = $1`,
			paymentID,
		).Scan(&payment.ID, &payment.ExpenseID, &payment.PaidBy, &payment.Amount, &payment.Note, &payment.CreatedAt)
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		return &payment, nil
	})
}

// GetGroupExpensePaymentStatuses returns payment status for all expenses in a group in one query
func GetGroupExpensePaymentStatuses(d *sql.DB, groupID int) (map[int]models.ExpensePaymentStatus, error) {
	return retry("GetGroupExpensePaymentStatuses", func() (map[int]models.ExpensePaymentStatus, error) {
		// Single query to get: expense_id, total_owed (from splits excluding payer), total_paid
		rows, err := d.Query(`
			SELECT 
				e.id as expense_id,
				COALESCE((SELECT SUM(es.amount) FROM expense_splits es WHERE es.expense_id = e.id AND es.user_id != e.paid_by), 0) as total_owed,
				COALESCE((SELECT SUM(ep.amount) FROM expense_payments ep WHERE ep.expense_id = e.id), 0) as total_paid
			FROM expenses e
			WHERE e.group_id = $1
		`, groupID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		statuses := make(map[int]models.ExpensePaymentStatus)
		for rows.Next() {
			var expenseID int
			var status models.ExpensePaymentStatus
			if err := rows.Scan(&expenseID, &status.TotalOwed, &status.TotalPaid); err != nil {
				return nil, err
			}
			statuses[expenseID] = status
		}
		return statuses, rows.Err()
	})
}

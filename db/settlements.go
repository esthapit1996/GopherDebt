package db

import (
	"database/sql"
	"math"
	"time"

	"gopherdebt/models"
)

func CreateSettlement(db *sql.DB, groupID, paidBy, paidTo int, amount float64) (*models.Settlement, error) {
	var settlement models.Settlement
	err := db.QueryRow(
		`INSERT INTO settlements (group_id, paid_by, paid_to, amount) VALUES ($1, $2, $3, $4) RETURNING id, group_id, paid_by, paid_to, amount, created_at`,
		groupID, paidBy, paidTo, amount,
	).Scan(&settlement.ID, &settlement.GroupID, &settlement.PaidBy, &settlement.PaidTo, &settlement.Amount, &settlement.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &settlement, nil
}

func GetGroupSettlements(d *sql.DB, groupID int) ([]models.Settlement, error) {
	return retry("GetGroupSettlements", func() ([]models.Settlement, error) {
		rows, err := d.Query(
			`SELECT id, group_id, paid_by, paid_to, amount, created_at FROM settlements WHERE group_id = $1 ORDER BY created_at DESC`,
			groupID,
		)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var settlements []models.Settlement
		for rows.Next() {
			var settlement models.Settlement
			if err := rows.Scan(&settlement.ID, &settlement.GroupID, &settlement.PaidBy, &settlement.PaidTo, &settlement.Amount, &settlement.CreatedAt); err != nil {
				return nil, err
			}
			settlements = append(settlements, settlement)
		}
		return settlements, rows.Err()
	})
}

func DeleteSettlement(db *sql.DB, settlementID int) error {
	result, err := db.Exec(`DELETE FROM settlements WHERE id = $1`, settlementID)
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

func CalculateGroupBalances(d *sql.DB, groupID int) ([]models.Balance, error) {
	return retry("CalculateGroupBalances", func() ([]models.Balance, error) {
		members, err := GetGroupMembers(d, groupID)
		if err != nil {
			return nil, err
		}

		userMap := make(map[int]*models.User)
		for i := range members {
			userMap[members[i].ID] = &members[i]
		}

		netBalances := make(map[int]float64)
		for _, member := range members {
			netBalances[member.ID] = 0
		}

		rows, err := d.Query(
			`SELECT e.paid_by, es.user_id, es.amount FROM expenses e INNER JOIN expense_splits es ON e.id = es.expense_id WHERE e.group_id = $1`,
			groupID,
		)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		for rows.Next() {
			var paidBy, owedBy int
			var amount float64
			if err := rows.Scan(&paidBy, &owedBy, &amount); err != nil {
				return nil, err
			}
			netBalances[paidBy] += amount
			netBalances[owedBy] -= amount
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}

		settlements, err := GetGroupSettlements(d, groupID)
		if err != nil {
			return nil, err
		}
		for _, s := range settlements {
			netBalances[s.PaidBy] += s.Amount
			netBalances[s.PaidTo] -= s.Amount
		}

		// Factor in expense payments (partial repayments)
		// When someone pays back their share of an expense, it's like a mini-settlement
		paymentRows, err := d.Query(
			`SELECT ep.paid_by, e.paid_by, ep.amount 
		FROM expense_payments ep 
		INNER JOIN expenses e ON ep.expense_id = e.id 
		WHERE e.group_id = $1`,
			groupID,
		)
		if err != nil {
			return nil, err
		}
		defer paymentRows.Close()

		for paymentRows.Next() {
			var paidBy, paidTo int
			var amount float64
			if err := paymentRows.Scan(&paidBy, &paidTo, &amount); err != nil {
				return nil, err
			}
			// The person who made the payment increases their balance (they've paid back)
			netBalances[paidBy] += amount
			// The original payer receives this payment
			netBalances[paidTo] -= amount
		}
		if err := paymentRows.Err(); err != nil {
			return nil, err
		}

		var creditors []struct {
			userID int
			amount float64
		}
		var debtors []struct {
			userID int
			amount float64
		}

		for userID, balance := range netBalances {
			if balance > 0.01 {
				creditors = append(creditors, struct {
					userID int
					amount float64
				}{userID, balance})
			} else if balance < -0.01 {
				debtors = append(debtors, struct {
					userID int
					amount float64
				}{userID, -balance})
			}
		}

		var balances []models.Balance
		for len(creditors) > 0 && len(debtors) > 0 {
			creditor := &creditors[0]
			debtor := &debtors[0]

			amount := creditor.amount
			if debtor.amount < amount {
				amount = debtor.amount
			}

			if amount > 0.01 {
				balances = append(balances, models.Balance{
					FromUser: userMap[debtor.userID],
					ToUser:   userMap[creditor.userID],
					Amount:   amount,
				})
			}

			creditor.amount -= amount
			debtor.amount -= amount

			if creditor.amount < 0.01 {
				creditors = creditors[1:]
			}
			if debtor.amount < 0.01 {
				debtors = debtors[1:]
			}
		}

		return balances, nil
	})
}

func GetUserBalanceInGroup(d *sql.DB, groupID, userID int) (float64, error) {
	return retry("GetUserBalanceInGroup", func() (float64, error) {
		var balance float64 = 0

		var paidTotal sql.NullFloat64
		err := d.QueryRow(`SELECT COALESCE(SUM(amount), 0) FROM expenses WHERE group_id = $1 AND paid_by = $2`, groupID, userID).Scan(&paidTotal)
		if err != nil {
			return 0, err
		}
		if paidTotal.Valid {
			balance += paidTotal.Float64
		}

		var owesTotal sql.NullFloat64
		err = d.QueryRow(
			`SELECT COALESCE(SUM(es.amount), 0) FROM expense_splits es INNER JOIN expenses e ON es.expense_id = e.id WHERE e.group_id = $1 AND es.user_id = $2`,
			groupID, userID,
		).Scan(&owesTotal)
		if err != nil {
			return 0, err
		}
		if owesTotal.Valid {
			balance -= owesTotal.Float64
		}

		var settlementsPaid sql.NullFloat64
		err = d.QueryRow(`SELECT COALESCE(SUM(amount), 0) FROM settlements WHERE group_id = $1 AND paid_by = $2`, groupID, userID).Scan(&settlementsPaid)
		if err != nil {
			return 0, err
		}
		if settlementsPaid.Valid {
			balance += settlementsPaid.Float64
		}

		var settlementsReceived sql.NullFloat64
		err = d.QueryRow(`SELECT COALESCE(SUM(amount), 0) FROM settlements WHERE group_id = $1 AND paid_to = $2`, groupID, userID).Scan(&settlementsReceived)
		if err != nil {
			return 0, err
		}
		if settlementsReceived.Valid {
			balance -= settlementsReceived.Float64
		}

		// Factor in expense payments made by this user (payments they made to repay someone)
		var paymentsMade sql.NullFloat64
		err = d.QueryRow(
			`SELECT COALESCE(SUM(ep.amount), 0) 
			FROM expense_payments ep 
			INNER JOIN expenses e ON ep.expense_id = e.id 
			WHERE e.group_id = $1 AND ep.paid_by = $2`,
			groupID, userID,
		).Scan(&paymentsMade)
		if err != nil {
			return 0, err
		}
		if paymentsMade.Valid {
			balance += paymentsMade.Float64
		}

		// Factor in expense payments received by this user (payments received as the original expense payer)
		var paymentsReceived sql.NullFloat64
		err = d.QueryRow(
			`SELECT COALESCE(SUM(ep.amount), 0) 
			FROM expense_payments ep 
			INNER JOIN expenses e ON ep.expense_id = e.id 
			WHERE e.group_id = $1 AND e.paid_by = $2`,
			groupID, userID,
		).Scan(&paymentsReceived)
		if err != nil {
			return 0, err
		}
		if paymentsReceived.Valid {
			balance -= paymentsReceived.Float64
		}

		return balance, nil
	})
}

// IsGroupSettled returns true if all balances in the group are 0
func IsGroupSettled(db *sql.DB, groupID int) (bool, error) {
	balances, err := CalculateGroupBalances(db, groupID)
	if err != nil {
		return false, err
	}
	return len(balances) == 0, nil
}

// DebtOverviewItem represents a debt relationship with another user
type DebtOverviewItem struct {
	User   *models.User `json:"user"`
	Amount float64      `json:"amount"` // Positive = they owe you, Negative = you owe them
}

// GetDebtOverview calculates how much each user owes/is owed by the current user across all groups
func GetDebtOverview(d *sql.DB, userID int) ([]DebtOverviewItem, error) {
	return retry("GetDebtOverview", func() ([]DebtOverviewItem, error) {
		// Get all groups the user is a member of
		groups, err := GetUserGroups(d, userID)
		if err != nil {
			return nil, err
		}

		// Map to accumulate net balances with each user
		// Positive = they owe us, Negative = we owe them
		userBalances := make(map[int]float64)
		userMap := make(map[int]*models.User)

		for _, group := range groups {
			// Get all members of this group for userMap
			members, err := GetGroupMembers(d, group.ID)
			if err != nil {
				return nil, err
			}
			for i := range members {
				if members[i].ID != userID {
					userMap[members[i].ID] = &members[i]
				}
			}

			// Calculate pairwise balances from expense splits
			rows, err := d.Query(
				`SELECT e.paid_by, es.user_id, es.amount 
				FROM expenses e 
				INNER JOIN expense_splits es ON e.id = es.expense_id 
				WHERE e.group_id = $1`,
				group.ID,
			)
			if err != nil {
				return nil, err
			}

			func() {
				defer rows.Close()
				for rows.Next() {
					var paidBy, splitUser int
					var amount float64
					if err := rows.Scan(&paidBy, &splitUser, &amount); err != nil {
						continue
					}
					if paidBy == userID && splitUser != userID {
						userBalances[splitUser] += amount
					}
					if paidBy != userID && splitUser == userID {
						userBalances[paidBy] -= amount
					}
				}
			}()

			// Factor in settlements
			settlements, err := GetGroupSettlements(d, group.ID)
			if err != nil {
				return nil, err
			}
			for _, s := range settlements {
				if s.PaidBy == userID && s.PaidTo != userID {
					userBalances[s.PaidTo] += s.Amount
				}
				if s.PaidBy != userID && s.PaidTo == userID {
					userBalances[s.PaidBy] -= s.Amount
				}
			}

			// Factor in expense payments (partial repayments)
			paymentRows, err := d.Query(
				`SELECT ep.paid_by, e.paid_by as expense_payer, ep.amount 
				FROM expense_payments ep 
				INNER JOIN expenses e ON ep.expense_id = e.id 
				WHERE e.group_id = $1`,
				group.ID,
			)
			if err != nil {
				return nil, err
			}

			func() {
				defer paymentRows.Close()
				for paymentRows.Next() {
					var paidBy, expensePayer int
					var amount float64
					if err := paymentRows.Scan(&paidBy, &expensePayer, &amount); err != nil {
						continue
					}
					if paidBy == userID && expensePayer != userID {
						userBalances[expensePayer] += amount
					}
					if paidBy != userID && expensePayer == userID {
						userBalances[paidBy] -= amount
					}
				}
			}()
		}

		// Convert to DebtOverviewItem slice, only include non-zero balances
		var result []DebtOverviewItem
		for uid, balance := range userBalances {
			if balance > 0.01 || balance < -0.01 {
				if userMap[uid] == nil {
					user, err := GetUserByID(d, uid)
					if err == nil && user != nil {
						userMap[uid] = user
					}
				}
				if userMap[uid] != nil {
					result = append(result, DebtOverviewItem{
						User:   userMap[uid],
						Amount: balance,
					})
				}
			}
		}

		return result, nil
	})
}

// DebtDetailItem represents a single expense/settlement contributing to a debt between two users
type DebtDetailItem struct {
	Type        string    `json:"type"` // "expense", "settlement", "payment"
	GroupName   string    `json:"group_name"`
	Description string    `json:"description"`
	Amount      float64   `json:"amount"` // Positive = they owe you, Negative = you owe them
	CreatedAt   time.Time `json:"created_at"`
}

// GetDebtDetails returns a detailed breakdown of debts between the current user and another user
func GetDebtDetails(d *sql.DB, userID, otherUserID int) ([]DebtDetailItem, error) {
	return retry("GetDebtDetails", func() ([]DebtDetailItem, error) {
		var items []DebtDetailItem

		// Get all groups the user is a member of
		groups, err := GetUserGroups(d, userID)
		if err != nil {
			return nil, err
		}

		for _, group := range groups {
			// Expenses: where one paid and the other has a split
			rows, err := d.Query(
				`SELECT e.description, es.amount, e.created_at, e.paid_by
				FROM expenses e
				INNER JOIN expense_splits es ON e.id = es.expense_id
				WHERE e.group_id = $1
				AND (
					(e.paid_by = $2 AND es.user_id = $3)
					OR (e.paid_by = $3 AND es.user_id = $2)
				)`,
				group.ID, userID, otherUserID,
			)
			if err != nil {
				return nil, err
			}

			func() {
				defer rows.Close()
				for rows.Next() {
					var desc string
					var amount float64
					var createdAt time.Time
					var paidBy int
					if err := rows.Scan(&desc, &amount, &createdAt, &paidBy); err != nil {
						continue
					}
					// If I paid and they have a split → they owe me (positive)
					// If they paid and I have a split → I owe them (negative)
					if paidBy == userID {
						items = append(items, DebtDetailItem{
							Type:        "expense",
							GroupName:   group.Name,
							Description: desc,
							Amount:      amount,
							CreatedAt:   createdAt,
						})
					} else {
						items = append(items, DebtDetailItem{
							Type:        "expense",
							GroupName:   group.Name,
							Description: desc,
							Amount:      -amount,
							CreatedAt:   createdAt,
						})
					}
				}
			}()

			// Settlements between the two users
			settlementRows, err := d.Query(
				`SELECT amount, created_at, paid_by
				FROM settlements
				WHERE group_id = $1
				AND ((paid_by = $2 AND paid_to = $3) OR (paid_by = $3 AND paid_to = $2))`,
				group.ID, userID, otherUserID,
			)
			if err != nil {
				return nil, err
			}
			func() {
				defer settlementRows.Close()
				for settlementRows.Next() {
					var amount float64
					var createdAt time.Time
					var paidBy int
					if err := settlementRows.Scan(&amount, &createdAt, &paidBy); err != nil {
						continue
					}
					// If I paid them → they owe me more (positive = reducing what I owe)
					// If they paid me → I owe them more
					if paidBy == userID {
						items = append(items, DebtDetailItem{
							Type:        "settlement",
							GroupName:   group.Name,
							Description: "Settlement",
							Amount:      amount,
							CreatedAt:   createdAt,
						})
					} else {
						items = append(items, DebtDetailItem{
							Type:        "settlement",
							GroupName:   group.Name,
							Description: "Settlement",
							Amount:      -amount,
							CreatedAt:   createdAt,
						})
					}
				}
			}()

			// Expense payments between the two users
			paymentRows, err := d.Query(
				`SELECT ep.amount, ep.created_at, ep.paid_by, e.description
				FROM expense_payments ep
				INNER JOIN expenses e ON ep.expense_id = e.id
				WHERE e.group_id = $1
				AND ((ep.paid_by = $2 AND e.paid_by = $3) OR (ep.paid_by = $3 AND e.paid_by = $2))`,
				group.ID, userID, otherUserID,
			)
			if err != nil {
				return nil, err
			}
			func() {
				defer paymentRows.Close()
				for paymentRows.Next() {
					var amount float64
					var createdAt time.Time
					var paidBy int
					var expenseDesc string
					if err := paymentRows.Scan(&amount, &createdAt, &paidBy, &expenseDesc); err != nil {
						continue
					}
					desc := "Payment: " + expenseDesc
					if paidBy == userID {
						items = append(items, DebtDetailItem{
							Type:        "payment",
							GroupName:   group.Name,
							Description: desc,
							Amount:      amount,
							CreatedAt:   createdAt,
						})
					} else {
						items = append(items, DebtDetailItem{
							Type:        "payment",
							GroupName:   group.Name,
							Description: desc,
							Amount:      -amount,
							CreatedAt:   createdAt,
						})
					}
				}
			}()
		}

		// Filter out items that round to zero
		var filtered []DebtDetailItem
		for _, item := range items {
			if math.Abs(item.Amount) >= 0.01 {
				filtered = append(filtered, item)
			}
		}

		return filtered, nil
	})
}

// PaymentHistoryItem represents a payment in the user's history
type PaymentHistoryItem struct {
	ID          int          `json:"id"`
	Type        string       `json:"type"` // "settlement" or "expense_payment"
	OtherUser   *models.User `json:"other_user"`
	Amount      float64      `json:"amount"`
	IsPayer     bool         `json:"is_payer"` // true if current user paid, false if received
	GroupName   string       `json:"group_name"`
	Description string       `json:"description,omitempty"`
	CreatedAt   string       `json:"created_at"`
}

// GetPaymentHistory returns all settlements and expense payments for a user
func GetPaymentHistory(d *sql.DB, userID int) ([]PaymentHistoryItem, error) {
	return retry("GetPaymentHistory", func() ([]PaymentHistoryItem, error) {
		var history []PaymentHistoryItem

		// Get settlements where user is payer or receiver
		rows, err := d.Query(
			`SELECT s.id, s.paid_by, s.paid_to, s.amount, s.created_at, g.name as group_name,
				u_payer.id, u_payer.email, u_payer.name, u_payer.created_at,
				u_receiver.id, u_receiver.email, u_receiver.name, u_receiver.created_at
			FROM settlements s
			JOIN groups g ON s.group_id = g.id
			JOIN users u_payer ON s.paid_by = u_payer.id
			JOIN users u_receiver ON s.paid_to = u_receiver.id
			WHERE s.paid_by = $1 OR s.paid_to = $1
			ORDER BY s.created_at DESC`,
			userID,
		)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		for rows.Next() {
			var id, payerID, receiverID int
			var amount float64
			var createdAt, groupName string
			var payer, receiver models.User

			if err := rows.Scan(
				&id, &payerID, &receiverID, &amount, &createdAt, &groupName,
				&payer.ID, &payer.Email, &payer.Name, &payer.CreatedAt,
				&receiver.ID, &receiver.Email, &receiver.Name, &receiver.CreatedAt,
			); err != nil {
				continue
			}

			isPayer := payerID == userID
			var otherUser *models.User
			if isPayer {
				otherUser = &receiver
			} else {
				otherUser = &payer
			}

			history = append(history, PaymentHistoryItem{
				ID:          id,
				Type:        "settlement",
				OtherUser:   otherUser,
				Amount:      amount,
				IsPayer:     isPayer,
				GroupName:   groupName,
				Description: "Settlement",
				CreatedAt:   createdAt,
			})
		}

		// Get expense payments where user is payer or the expense owner
		paymentRows, err := d.Query(
			`SELECT ep.id, ep.paid_by, e.paid_by as expense_owner, ep.amount, ep.created_at, g.name as group_name, e.description,
				u_payer.id, u_payer.email, u_payer.name, u_payer.created_at,
				u_owner.id, u_owner.email, u_owner.name, u_owner.created_at
			FROM expense_payments ep
			JOIN expenses e ON ep.expense_id = e.id
			JOIN groups g ON e.group_id = g.id
			JOIN users u_payer ON ep.paid_by = u_payer.id
			JOIN users u_owner ON e.paid_by = u_owner.id
			WHERE ep.paid_by = $1 OR e.paid_by = $1
			ORDER BY ep.created_at DESC`,
			userID,
		)
		if err != nil {
			return nil, err
		}
		defer paymentRows.Close()

		for paymentRows.Next() {
			var id, payerID, ownerID int
			var amount float64
			var createdAt, groupName, description string
			var payer, owner models.User

			if err := paymentRows.Scan(
				&id, &payerID, &ownerID, &amount, &createdAt, &groupName, &description,
				&payer.ID, &payer.Email, &payer.Name, &payer.CreatedAt,
				&owner.ID, &owner.Email, &owner.Name, &owner.CreatedAt,
			); err != nil {
				continue
			}

			isPayer := payerID == userID
			var otherUser *models.User
			if isPayer {
				otherUser = &owner
			} else {
				otherUser = &payer
			}

			history = append(history, PaymentHistoryItem{
				ID:          id,
				Type:        "expense_payment",
				OtherUser:   otherUser,
				Amount:      amount,
				IsPayer:     isPayer,
				GroupName:   groupName,
				Description: description,
				CreatedAt:   createdAt,
			})
		}

		// Sort by created_at descending (most recent first)
		for i := 0; i < len(history)-1; i++ {
			for j := 0; j < len(history)-i-1; j++ {
				if history[j].CreatedAt < history[j+1].CreatedAt {
					history[j], history[j+1] = history[j+1], history[j]
				}
			}
		}

		if len(history) > 250 {
			history = history[:250]
		}

		return history, nil
	})
}

// ClearPaymentHistory deletes all settlements and expense payments for a user
func ClearPaymentHistory(db *sql.DB, userID int) error {
	// Delete settlements where user is payer or receiver
	_, err := db.Exec(`DELETE FROM settlements WHERE paid_by = $1 OR paid_to = $1`, userID)
	if err != nil {
		return err
	}

	// Delete expense payments where user is the payer
	_, err = db.Exec(`DELETE FROM expense_payments WHERE paid_by = $1`, userID)
	if err != nil {
		return err
	}

	return nil
}

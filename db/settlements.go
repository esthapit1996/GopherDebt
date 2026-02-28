package db

import (
	"database/sql"

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

func GetGroupSettlements(db *sql.DB, groupID int) ([]models.Settlement, error) {
	rows, err := db.Query(
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
}

func CalculateGroupBalances(db *sql.DB, groupID int) ([]models.Balance, error) {
	members, err := GetGroupMembers(db, groupID)
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

	rows, err := db.Query(
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

	settlements, err := GetGroupSettlements(db, groupID)
	if err != nil {
		return nil, err
	}
	for _, s := range settlements {
		netBalances[s.PaidBy] += s.Amount
		netBalances[s.PaidTo] -= s.Amount
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
}

func GetUserBalanceInGroup(db *sql.DB, groupID, userID int) (float64, error) {
	var balance float64 = 0

	var paidTotal sql.NullFloat64
	err := db.QueryRow(`SELECT COALESCE(SUM(amount), 0) FROM expenses WHERE group_id = $1 AND paid_by = $2`, groupID, userID).Scan(&paidTotal)
	if err != nil {
		return 0, err
	}
	if paidTotal.Valid {
		balance += paidTotal.Float64
	}

	var owesTotal sql.NullFloat64
	err = db.QueryRow(
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
	err = db.QueryRow(`SELECT COALESCE(SUM(amount), 0) FROM settlements WHERE group_id = $1 AND paid_by = $2`, groupID, userID).Scan(&settlementsPaid)
	if err != nil {
		return 0, err
	}
	if settlementsPaid.Valid {
		balance += settlementsPaid.Float64
	}

	var settlementsReceived sql.NullFloat64
	err = db.QueryRow(`SELECT COALESCE(SUM(amount), 0) FROM settlements WHERE group_id = $1 AND paid_to = $2`, groupID, userID).Scan(&settlementsReceived)
	if err != nil {
		return 0, err
	}
	if settlementsReceived.Valid {
		balance -= settlementsReceived.Float64
	}

	return balance, nil
}

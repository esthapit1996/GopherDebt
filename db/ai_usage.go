package db

import "database/sql"

// TakeAIQuota counts one AI call of the given kind ("receipt", "wish") for the user today (UTC) and
// reports whether it was within the daily limit. It's a single atomic statement, so parallel requests
// can't push a user past the limit.
func TakeAIQuota(d *sql.DB, userID int, kind string, limit int) (bool, error) {
	if limit <= 0 {
		return false, nil
	}
	var count int
	err := d.QueryRow(
		`INSERT INTO ai_usage (user_id, day, kind, count) VALUES ($1, CURRENT_DATE, $2, 1)
		 ON CONFLICT (user_id, day, kind) DO UPDATE SET count = ai_usage.count + 1
		 WHERE ai_usage.count < $3
		 RETURNING count`,
		userID, kind, limit,
	).Scan(&count)
	if err == sql.ErrNoRows {
		return false, nil // already at the limit: the conditional update didn't run
	}
	return err == nil, err
}

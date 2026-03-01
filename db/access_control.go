package db

import (
	"database/sql"
	"time"
)

// WhitelistEntry represents an email in the whitelist
type WhitelistEntry struct {
	ID        int       `json:"id"`
	Email     string    `json:"email"`
	AddedBy   *int      `json:"added_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// BlacklistEntry represents an email in the blacklist
type BlacklistEntry struct {
	ID        int       `json:"id"`
	Email     string    `json:"email"`
	Reason    string    `json:"reason"`
	AddedBy   *int      `json:"added_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// IsEmailWhitelisted checks if an email is in the whitelist
func IsEmailWhitelisted(db *sql.DB, email string) (bool, error) {
	var exists bool
	err := db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM email_whitelist WHERE LOWER(email) = LOWER($1))`,
		email,
	).Scan(&exists)
	return exists, err
}

// IsEmailBlacklisted checks if an email is in the blacklist
func IsEmailBlacklisted(db *sql.DB, email string) (bool, error) {
	var exists bool
	err := db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM email_blacklist WHERE LOWER(email) = LOWER($1))`,
		email,
	).Scan(&exists)
	return exists, err
}

// GetWhitelist returns all whitelisted emails
func GetWhitelist(db *sql.DB) ([]WhitelistEntry, error) {
	rows, err := db.Query(
		`SELECT id, email, added_by, created_at FROM email_whitelist ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []WhitelistEntry
	for rows.Next() {
		var entry WhitelistEntry
		var addedBy sql.NullInt64
		if err := rows.Scan(&entry.ID, &entry.Email, &addedBy, &entry.CreatedAt); err != nil {
			return nil, err
		}
		if addedBy.Valid {
			addedByInt := int(addedBy.Int64)
			entry.AddedBy = &addedByInt
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

// GetBlacklist returns all blacklisted emails
func GetBlacklist(db *sql.DB) ([]BlacklistEntry, error) {
	rows, err := db.Query(
		`SELECT id, email, COALESCE(reason, ''), added_by, created_at FROM email_blacklist ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []BlacklistEntry
	for rows.Next() {
		var entry BlacklistEntry
		var addedBy sql.NullInt64
		if err := rows.Scan(&entry.ID, &entry.Email, &entry.Reason, &addedBy, &entry.CreatedAt); err != nil {
			return nil, err
		}
		if addedBy.Valid {
			addedByInt := int(addedBy.Int64)
			entry.AddedBy = &addedByInt
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

// AddToWhitelist adds an email to the whitelist
func AddToWhitelist(db *sql.DB, email string, addedBy int) (*WhitelistEntry, error) {
	var entry WhitelistEntry
	err := db.QueryRow(
		`INSERT INTO email_whitelist (email, added_by) VALUES (LOWER($1), $2) 
		 RETURNING id, email, added_by, created_at`,
		email, addedBy,
	).Scan(&entry.ID, &entry.Email, &entry.AddedBy, &entry.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &entry, nil
}

// AddToBlacklist adds an email to the blacklist
func AddToBlacklist(db *sql.DB, email string, reason string, addedBy int) (*BlacklistEntry, error) {
	var entry BlacklistEntry
	var addedByVal sql.NullInt64
	err := db.QueryRow(
		`INSERT INTO email_blacklist (email, reason, added_by) VALUES (LOWER($1), $2, $3) 
		 RETURNING id, email, COALESCE(reason, ''), added_by, created_at`,
		email, reason, addedBy,
	).Scan(&entry.ID, &entry.Email, &entry.Reason, &addedByVal, &entry.CreatedAt)
	if err != nil {
		return nil, err
	}
	if addedByVal.Valid {
		addedByInt := int(addedByVal.Int64)
		entry.AddedBy = &addedByInt
	}
	return &entry, nil
}

// RemoveFromWhitelist removes an email from the whitelist
func RemoveFromWhitelist(db *sql.DB, id int) error {
	result, err := db.Exec(`DELETE FROM email_whitelist WHERE id = $1`, id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// RemoveFromBlacklist removes an email from the blacklist
func RemoveFromBlacklist(db *sql.DB, id int) error {
	result, err := db.Exec(`DELETE FROM email_blacklist WHERE id = $1`, id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

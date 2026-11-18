package db

import (
	"database/sql"
	"errors"

	"github.com/lib/pq"

	"gopherdebt/models"
)

var ErrNotFound = errors.New("record not found")
var ErrDuplicate = errors.New("record already exists")

func CreateUser(db *sql.DB, email, passwordHash, name string) (*models.User, error) {
	var user models.User
	err := db.QueryRow(
		`INSERT INTO users (email, password_hash, name, theme_preference, avatar, language) VALUES ($1, $2, $3, 'darkknight', '', 'en') RETURNING id, email, name, COALESCE(avatar, ''), theme_preference, COALESCE(language, 'en'), created_at, updated_at`,
		email, passwordHash, name,
	).Scan(&user.ID, &user.Email, &user.Name, &user.Avatar, &user.ThemePreference, &user.Language, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetUserByEmail finds a user ignoring letter case (oldest account wins if case variants exist)
func GetUserByEmail(d *sql.DB, email string) (*models.User, error) {
	return retry("GetUserByEmail", func() (*models.User, error) {
		var user models.User
		err := d.QueryRow(
			`SELECT id, email, password_hash, name, COALESCE(avatar, ''), COALESCE(theme_preference, 'darkknight'), COALESCE(language, 'en'), created_at, updated_at, token_version FROM users
			 WHERE LOWER(email) = LOWER($1) AND deleted_at IS NULL ORDER BY id LIMIT 1`,
			email,
		).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Name, &user.Avatar, &user.ThemePreference, &user.Language, &user.CreatedAt, &user.UpdatedAt, &user.TokenVersion)
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		return &user, nil
	})
}

func GetUserByID(d *sql.DB, id int) (*models.User, error) {
	return retry("GetUserByID", func() (*models.User, error) {
		var user models.User
		err := d.QueryRow(
			`SELECT id, email, name, COALESCE(avatar, ''), COALESCE(theme_preference, 'darkknight'), COALESCE(language, 'en'), created_at, updated_at FROM users WHERE id = $1`,
			id,
		).Scan(&user.ID, &user.Email, &user.Name, &user.Avatar, &user.ThemePreference, &user.Language, &user.CreatedAt, &user.UpdatedAt)
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		return &user, nil
	})
}

func GetAllUsers(d *sql.DB) ([]models.User, error) {
	return retry("GetAllUsers", func() ([]models.User, error) {
		rows, err := d.Query(`SELECT id, email, name, COALESCE(avatar, ''), COALESCE(theme_preference, 'darkknight'), COALESCE(language, 'en'), created_at, updated_at FROM users WHERE deleted_at IS NULL ORDER BY name`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var users []models.User
		for rows.Next() {
			var user models.User
			if err := rows.Scan(&user.ID, &user.Email, &user.Name, &user.Avatar, &user.ThemePreference, &user.Language, &user.CreatedAt, &user.UpdatedAt); err != nil {
				return nil, err
			}
			users = append(users, user)
		}
		return users, rows.Err()
	})
}

func UpdateUserTheme(db *sql.DB, userID int, theme string) error {
	_, err := db.Exec(`UPDATE users SET theme_preference = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2`, theme, userID)
	return err
}

func UpdateUserAvatar(db *sql.DB, userID int, avatar string) error {
	_, err := db.Exec(`UPDATE users SET avatar = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2`, avatar, userID)
	return err
}

func UpdateUserLanguage(db *sql.DB, userID int, language string) error {
	_, err := db.Exec(`UPDATE users SET language = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2`, language, userID)
	return err
}

func GetUserPasswordHash(d *sql.DB, userID int) (string, error) {
	return retry("GetUserPasswordHash", func() (string, error) {
		var hash string
		err := d.QueryRow(`SELECT password_hash FROM users WHERE id = $1`, userID).Scan(&hash)
		if err == sql.ErrNoRows {
			return "", ErrNotFound
		}
		if err != nil {
			return "", err
		}
		return hash, nil
	})
}

// UpdateUserPassword sets a new password and bumps token_version, which logs out every other
// session. Returns the new version so the caller can issue a fresh token for the current one.
func UpdateUserPassword(d *sql.DB, userID int, newPasswordHash string) (int, error) {
	var version int
	err := d.QueryRow(
		`UPDATE users SET password_hash = $1, token_version = token_version + 1, updated_at = CURRENT_TIMESTAMP
		 WHERE id = $2 AND deleted_at IS NULL RETURNING token_version`,
		newPasswordHash, userID,
	).Scan(&version)
	if err == sql.ErrNoRows {
		return 0, ErrNotFound
	}
	return version, err
}

// GetSessionState reports the user's current token_version and whether they may use the app
// (not deleted, email not blacklisted). Checked on every authenticated request.
func GetSessionState(d *sql.DB, userID int) (tokenVersion int, active bool, err error) {
	type state struct {
		version int
		active  bool
	}
	st, err := retry("GetSessionState", func() (state, error) {
		var s state
		err := d.QueryRow(
			`SELECT u.token_version,
			        u.deleted_at IS NULL AND NOT EXISTS (
			          SELECT 1 FROM email_blacklist b WHERE LOWER(b.email) = LOWER(u.email))
			 FROM users u WHERE u.id = $1`,
			userID,
		).Scan(&s.version, &s.active)
		if err == sql.ErrNoRows {
			return s, ErrNotFound
		}
		return s, err
	})
	return st.version, st.active, err
}

const FounderEmail = "evansthapit20@gmail.com"

// DeletedUserName is shown wherever an anonymized account appears (old expenses, history, balances)
const DeletedUserName = "Deleted user"

// DeleteUser anonymizes an account instead of removing it, so everyone else's balances and the
// groups' history stay correct:
//   - the row is kept, renamed "Deleted user", with an unusable email/password and every token revoked
//   - personal data goes: GopherStash, wishes, wish-group memberships (a wish group left with no
//     members is deleted, as when the last member leaves) and the whitelist entry for their email
//   - they leave every debt group where they're settled; where they still owe or are owed money they
//     stay as "Deleted user" so the others can still record settling up with them
//   - groups they created, their expenses, settlements, payments, suggestions and comments are kept
func DeleteUser(d *sql.DB, userID int) error {
	// Settled groups are worked out first (balance queries use the pool, not the transaction)
	var settled []int
	rows, err := d.Query(`SELECT group_id FROM group_members WHERE user_id = $1`, userID)
	if err != nil {
		return err
	}
	var group_ids []int
	for rows.Next() {
		var g int
		if err := rows.Scan(&g); err != nil {
			rows.Close()
			return err
		}
		group_ids = append(group_ids, g)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, g := range group_ids {
		balance, err := GetUserBalanceInGroup(d, g, userID)
		if err != nil {
			return err
		}
		if balance <= 0.01 && balance >= -0.01 {
			settled = append(settled, g)
		}
	}

	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var email string
	err = tx.QueryRow(`SELECT email FROM users WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, userID).Scan(&email)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	steps := []struct {
		query string
		args  []any
	}{
		{`UPDATE users SET name = $2, email = 'deleted-' || id || '@deleted.invalid', password_hash = '!',
		  avatar = '', deleted_at = CURRENT_TIMESTAMP, token_version = token_version + 1, updated_at = CURRENT_TIMESTAMP
		  WHERE id = $1`, []any{userID, DeletedUserName}},
		{`DELETE FROM email_whitelist WHERE LOWER(email) = LOWER($1)`, []any{email}},
		{`DELETE FROM stash_expenses WHERE user_id = $1`, []any{userID}},
		{`DELETE FROM wish_items WHERE user_id = $1`, []any{userID}},
		{`DELETE FROM wish_list_members WHERE user_id = $1`, []any{userID}},
		{`DELETE FROM wish_lists wl WHERE NOT EXISTS (SELECT 1 FROM wish_list_members m WHERE m.list_id = wl.id)`, nil},
		{`DELETE FROM group_members WHERE user_id = $1 AND group_id = ANY($2)`, []any{userID, pq.Array(settled)}},
	}
	for _, st := range steps {
		if _, err := tx.Exec(st.query, st.args...); err != nil {
			return err
		}
	}
	return tx.Commit()
}

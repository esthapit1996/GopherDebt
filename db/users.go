package db

import (
	"database/sql"
	"errors"

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

func GetUserByEmail(d *sql.DB, email string) (*models.User, error) {
	return retry("GetUserByEmail", func() (*models.User, error) {
		var user models.User
		err := d.QueryRow(
			`SELECT id, email, password_hash, name, COALESCE(avatar, ''), COALESCE(theme_preference, 'darkknight'), COALESCE(language, 'en'), created_at, updated_at FROM users WHERE email = $1`,
			email,
		).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Name, &user.Avatar, &user.ThemePreference, &user.Language, &user.CreatedAt, &user.UpdatedAt)
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
		rows, err := d.Query(`SELECT id, email, name, COALESCE(avatar, ''), COALESCE(theme_preference, 'darkknight'), COALESCE(language, 'en'), created_at, updated_at FROM users ORDER BY name`)
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

func UpdateUserPassword(db *sql.DB, userID int, newPasswordHash string) error {
	_, err := db.Exec(`UPDATE users SET password_hash = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2`, newPasswordHash, userID)
	return err
}

const FounderEmail = "evansthapit20@gmail.com"

func DeleteUser(d *sql.DB, userID int) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete in proper order to satisfy foreign key constraints
	deletes := []string{
		`DELETE FROM expense_payments WHERE paid_by = $1`,
		`DELETE FROM expense_splits WHERE user_id = $1`,
		`DELETE FROM expenses WHERE paid_by = $1`,
		`DELETE FROM settlements WHERE paid_by = $1 OR paid_to = $1`,
		`DELETE FROM suggestion_votes WHERE user_id = $1`,
		`DELETE FROM suggestion_comments WHERE user_id = $1`,
		`DELETE FROM suggestions WHERE user_id = $1`,
		`DELETE FROM group_members WHERE user_id = $1`,
		`DELETE FROM groups WHERE created_by = $1`,
		`DELETE FROM users WHERE id = $1`,
	}
	for _, q := range deletes {
		if _, err := tx.Exec(q, userID); err != nil {
			return err
		}
	}

	return tx.Commit()
}

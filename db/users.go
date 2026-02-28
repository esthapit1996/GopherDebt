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
		`INSERT INTO users (email, password_hash, name, theme_preference) VALUES ($1, $2, $3, 'dark') RETURNING id, email, name, theme_preference, created_at, updated_at`,
		email, passwordHash, name,
	).Scan(&user.ID, &user.Email, &user.Name, &user.ThemePreference, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func GetUserByEmail(db *sql.DB, email string) (*models.User, error) {
	var user models.User
	err := db.QueryRow(
		`SELECT id, email, password_hash, name, COALESCE(theme_preference, 'dark'), created_at, updated_at FROM users WHERE email = $1`,
		email,
	).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Name, &user.ThemePreference, &user.CreatedAt, &user.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func GetUserByID(db *sql.DB, id int) (*models.User, error) {
	var user models.User
	err := db.QueryRow(
		`SELECT id, email, name, COALESCE(theme_preference, 'dark'), created_at, updated_at FROM users WHERE id = $1`,
		id,
	).Scan(&user.ID, &user.Email, &user.Name, &user.ThemePreference, &user.CreatedAt, &user.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func GetAllUsers(db *sql.DB) ([]models.User, error) {
	rows, err := db.Query(`SELECT id, email, name, COALESCE(theme_preference, 'dark'), created_at, updated_at FROM users ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var user models.User
		if err := rows.Scan(&user.ID, &user.Email, &user.Name, &user.ThemePreference, &user.CreatedAt, &user.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func UpdateUserTheme(db *sql.DB, userID int, theme string) error {
	_, err := db.Exec(`UPDATE users SET theme_preference = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2`, theme, userID)
	return err
}

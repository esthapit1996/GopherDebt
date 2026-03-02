package db

import (
	"database/sql"
	"fmt"
)

func RunMigrations(db *sql.DB) error {
	// Tables are created if they don't exist - data persists across restarts
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			email VARCHAR(255) UNIQUE NOT NULL,
			password_hash VARCHAR(255) NOT NULL,
			name VARCHAR(255) NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS groups (
			id SERIAL PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			description TEXT,
			created_by INTEGER REFERENCES users(id),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS group_members (
			id SERIAL PRIMARY KEY,
			group_id INTEGER REFERENCES groups(id) ON DELETE CASCADE,
			user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
			joined_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(group_id, user_id)
		)`,
		`CREATE TABLE IF NOT EXISTS expenses (
			id SERIAL PRIMARY KEY,
			group_id INTEGER REFERENCES groups(id) ON DELETE CASCADE,
			paid_by INTEGER REFERENCES users(id),
			amount DECIMAL(10, 2) NOT NULL,
			description VARCHAR(255) NOT NULL,
			split_type VARCHAR(20) NOT NULL DEFAULT 'equal',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS expense_splits (
			id SERIAL PRIMARY KEY,
			expense_id INTEGER REFERENCES expenses(id) ON DELETE CASCADE,
			user_id INTEGER REFERENCES users(id),
			amount DECIMAL(10, 2) NOT NULL,
			UNIQUE(expense_id, user_id)
		)`,
		`CREATE TABLE IF NOT EXISTS settlements (
			id SERIAL PRIMARY KEY,
			group_id INTEGER REFERENCES groups(id) ON DELETE CASCADE,
			paid_by INTEGER REFERENCES users(id),
			paid_to INTEGER REFERENCES users(id),
			amount DECIMAL(10, 2) NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS expense_payments (
			id SERIAL PRIMARY KEY,
			expense_id INTEGER REFERENCES expenses(id) ON DELETE CASCADE,
			paid_by INTEGER REFERENCES users(id),
			amount DECIMAL(10, 2) NOT NULL,
			note VARCHAR(255),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_group_members_group_id ON group_members(group_id)`,
		`CREATE INDEX IF NOT EXISTS idx_group_members_user_id ON group_members(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_expenses_group_id ON expenses(group_id)`,
		`CREATE INDEX IF NOT EXISTS idx_expense_splits_expense_id ON expense_splits(expense_id)`,
		`CREATE INDEX IF NOT EXISTS idx_settlements_group_id ON settlements(group_id)`,
		`CREATE INDEX IF NOT EXISTS idx_expense_payments_expense_id ON expense_payments(expense_id)`,
		`CREATE TABLE IF NOT EXISTS activity_log (
			id SERIAL PRIMARY KEY,
			group_id INTEGER REFERENCES groups(id) ON DELETE CASCADE,
			user_id INTEGER REFERENCES users(id),
			action_type VARCHAR(50) NOT NULL,
			description TEXT NOT NULL,
			amount DECIMAL(10, 2),
			related_user_id INTEGER REFERENCES users(id),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_activity_log_group_id ON activity_log(group_id)`,
		// Add emoji column to groups table
		`ALTER TABLE groups ADD COLUMN IF NOT EXISTS emoji VARCHAR(10) DEFAULT '💰'`,
		// Add theme_preference column to users table
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS theme_preference VARCHAR(20) DEFAULT 'dark'`,
		// Suggestions table (max 20 suggestions, 800 chars each)
		`CREATE TABLE IF NOT EXISTS suggestions (
			id SERIAL PRIMARY KEY,
			user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
			content VARCHAR(800) NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_suggestions_user_id ON suggestions(user_id)`,
		// Expand content column to 800 chars (for existing tables)
		`ALTER TABLE suggestions ALTER COLUMN content TYPE VARCHAR(800)`,
		// Suggestion votes table (anonymous voting)
		`CREATE TABLE IF NOT EXISTS suggestion_votes (
			id SERIAL PRIMARY KEY,
			suggestion_id INTEGER REFERENCES suggestions(id) ON DELETE CASCADE,
			user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
			vote_type VARCHAR(10) NOT NULL CHECK (vote_type IN ('like', 'dislike')),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(suggestion_id, user_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_suggestion_votes_suggestion_id ON suggestion_votes(suggestion_id)`,
		// Add status column to suggestions (open, wip, done)
		`ALTER TABLE suggestions ADD COLUMN IF NOT EXISTS status VARCHAR(20) DEFAULT 'open'`,
		// Clean up duplicate votes - keep only the most recent vote per user/suggestion
		`DELETE FROM suggestion_votes a USING suggestion_votes b 
		 WHERE a.id < b.id 
		 AND a.suggestion_id = b.suggestion_id 
		 AND a.user_id = b.user_id`,
		// Add unique constraint if it doesn't exist
		`CREATE UNIQUE INDEX IF NOT EXISTS unique_suggestion_user_vote ON suggestion_votes(suggestion_id, user_id)`,
		// Suggestion comments table (max 420 chars, 4 comments per user per suggestion)
		`CREATE TABLE IF NOT EXISTS suggestion_comments (
			id SERIAL PRIMARY KEY,
			suggestion_id INTEGER REFERENCES suggestions(id) ON DELETE CASCADE,
			user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
			content VARCHAR(420) NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_suggestion_comments_suggestion_id ON suggestion_comments(suggestion_id)`,
		// Update suggestion content column to 420 chars
		`ALTER TABLE suggestions ALTER COLUMN content TYPE VARCHAR(420)`,
		// Email whitelist table
		`CREATE TABLE IF NOT EXISTS email_whitelist (
			id SERIAL PRIMARY KEY,
			email VARCHAR(255) UNIQUE NOT NULL,
			added_by INTEGER REFERENCES users(id),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		// Email blacklist table
		`CREATE TABLE IF NOT EXISTS email_blacklist (
			id SERIAL PRIMARY KEY,
			email VARCHAR(255) UNIQUE NOT NULL,
			reason VARCHAR(255),
			added_by INTEGER REFERENCES users(id),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		// Seed default whitelist entries if table is empty
		`INSERT INTO email_whitelist (email) 
		 SELECT 'evansthapit20@gmail.com' 
		 WHERE NOT EXISTS (SELECT 1 FROM email_whitelist WHERE email = 'evansthapit20@gmail.com')`,
		`INSERT INTO email_whitelist (email) 
		 SELECT 'e.ivanishcheva@yandex.ru' 
		 WHERE NOT EXISTS (SELECT 1 FROM email_whitelist WHERE email = 'e.ivanishcheva@yandex.ru')`,
		// Add avatar column to users table
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS avatar VARCHAR(50) DEFAULT ''`,
		// Add type column to suggestions table
		`ALTER TABLE suggestions ADD COLUMN IF NOT EXISTS type VARCHAR(30) DEFAULT 'other'`,
	}

	for i, migration := range migrations {
		_, err := db.Exec(migration)
		if err != nil {
			return fmt.Errorf("migration %d failed: %w", i+1, err)
		}
	}

	fmt.Println("All migrations completed successfully!")
	return nil
}

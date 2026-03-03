package db

import (
	"database/sql"
	"time"
)

// Suggestion represents a user suggestion
type Suggestion struct {
	ID           int       `json:"id"`
	UserID       int       `json:"user_id"`
	UserName     string    `json:"user_name"`
	UserEmail    string    `json:"user_email"`
	Content      string    `json:"content"`
	Type         string    `json:"type"`   // "feature", "bug", "theme", "change", "complaint", "praise", "ux", "other"
	Status       string    `json:"status"` // "open", "wip", "done", "denied"
	CreatedAt    time.Time `json:"created_at"`
	Likes        int       `json:"likes"`
	Dislikes     int       `json:"dislikes"`
	UserVote     string    `json:"user_vote,omitempty"` // "like", "dislike", or ""
	CommentCount int       `json:"comment_count"`
}

// Vote represents a vote on a suggestion (for admin viewing)
type Vote struct {
	ID           int       `json:"id"`
	SuggestionID int       `json:"suggestion_id"`
	UserID       int       `json:"user_id"`
	UserName     string    `json:"user_name"`
	UserEmail    string    `json:"user_email"`
	VoteType     string    `json:"vote_type"`
	CreatedAt    time.Time `json:"created_at"`
}

const MaxSuggestions = 20
const MaxSuggestionLength = 420
const MaxCommentLength = 420
const MaxCommentsPerUser = 4

// GetSuggestionCount returns the current number of suggestions
func GetSuggestionCount(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM suggestions").Scan(&count)
	return count, err
}

// GetOpenSuggestionCount returns the number of open suggestions only
func GetOpenSuggestionCount(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM suggestions WHERE status IS NULL OR status = 'open'").Scan(&count)
	return count, err
}

// GetAllSuggestions returns all suggestions with user info and vote counts
func GetAllSuggestions(db *sql.DB, currentUserID int) ([]Suggestion, error) {
	rows, err := db.Query(`
		SELECT s.id, s.user_id, u.name, u.email, s.content, COALESCE(s.type, 'other'), COALESCE(s.status, 'open'), s.created_at,
			COALESCE((SELECT COUNT(*) FROM suggestion_votes WHERE suggestion_id = s.id AND vote_type = 'like'), 0) as likes,
			COALESCE((SELECT COUNT(*) FROM suggestion_votes WHERE suggestion_id = s.id AND vote_type = 'dislike'), 0) as dislikes,
			COALESCE((SELECT vote_type FROM suggestion_votes WHERE suggestion_id = s.id AND user_id = $1), '') as user_vote,
			COALESCE((SELECT COUNT(*) FROM suggestion_comments WHERE suggestion_id = s.id), 0) as comment_count
		FROM suggestions s
		JOIN users u ON s.user_id = u.id
		ORDER BY s.created_at DESC
	`, currentUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var suggestions []Suggestion
	for rows.Next() {
		var s Suggestion
		if err := rows.Scan(&s.ID, &s.UserID, &s.UserName, &s.UserEmail, &s.Content, &s.Type, &s.Status, &s.CreatedAt, &s.Likes, &s.Dislikes, &s.UserVote, &s.CommentCount); err != nil {
			return nil, err
		}
		suggestions = append(suggestions, s)
	}
	return suggestions, nil
}

// CreateSuggestion creates a new suggestion (enforces max 10 limit)
func CreateSuggestion(db *sql.DB, userID int, content, suggestionType string) (*Suggestion, error) {
	// Truncate content if too long
	if len(content) > MaxSuggestionLength {
		content = content[:MaxSuggestionLength]
	}
	if suggestionType == "" {
		suggestionType = "other"
	}

	var suggestion Suggestion
	err := db.QueryRow(`
		INSERT INTO suggestions (user_id, content, type) 
		VALUES ($1, $2, $3) 
		RETURNING id, user_id, content, COALESCE(type, 'other'), created_at
	`, userID, content, suggestionType).Scan(&suggestion.ID, &suggestion.UserID, &suggestion.Content, &suggestion.Type, &suggestion.CreatedAt)
	if err != nil {
		return nil, err
	}

	// Get user info
	err = db.QueryRow("SELECT name, email FROM users WHERE id = $1", userID).Scan(&suggestion.UserName, &suggestion.UserEmail)
	if err != nil {
		return nil, err
	}

	return &suggestion, nil
}

// DeleteSuggestion deletes a suggestion by ID
func DeleteSuggestion(db *sql.DB, suggestionID int) error {
	_, err := db.Exec("DELETE FROM suggestions WHERE id = $1", suggestionID)
	return err
}

// GetSuggestionByID returns a suggestion by ID
func GetSuggestionByID(db *sql.DB, suggestionID int) (*Suggestion, error) {
	var s Suggestion
	err := db.QueryRow(`
		SELECT s.id, s.user_id, u.name, u.email, s.content, s.created_at
		FROM suggestions s
		JOIN users u ON s.user_id = u.id
		WHERE s.id = $1
	`, suggestionID).Scan(&s.ID, &s.UserID, &s.UserName, &s.UserEmail, &s.Content, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// VoteSuggestion adds or updates a vote on a suggestion
// Deletes any existing vote first to ensure only one vote per user
func VoteSuggestion(db *sql.DB, suggestionID, userID int, voteType string) error {
	// First, remove any existing vote by this user on this suggestion
	_, err := db.Exec(`DELETE FROM suggestion_votes WHERE suggestion_id = $1 AND user_id = $2`, suggestionID, userID)
	if err != nil {
		return err
	}
	// Then insert the new vote
	_, err = db.Exec(`
		INSERT INTO suggestion_votes (suggestion_id, user_id, vote_type)
		VALUES ($1, $2, $3)
	`, suggestionID, userID, voteType)
	return err
}

// RemoveVote removes a user's vote from a suggestion
func RemoveVote(db *sql.DB, suggestionID, userID int) error {
	_, err := db.Exec(`DELETE FROM suggestion_votes WHERE suggestion_id = $1 AND user_id = $2`, suggestionID, userID)
	return err
}

// GetSuggestionVotes returns all votes for a suggestion (for admin/founder viewing)
func GetSuggestionVotes(db *sql.DB, suggestionID int) ([]Vote, error) {
	rows, err := db.Query(`
		SELECT v.id, v.suggestion_id, v.user_id, u.name, u.email, v.vote_type, v.created_at
		FROM suggestion_votes v
		JOIN users u ON v.user_id = u.id
		WHERE v.suggestion_id = $1
		ORDER BY v.created_at DESC
	`, suggestionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var votes []Vote
	for rows.Next() {
		var v Vote
		if err := rows.Scan(&v.ID, &v.SuggestionID, &v.UserID, &v.UserName, &v.UserEmail, &v.VoteType, &v.CreatedAt); err != nil {
			return nil, err
		}
		votes = append(votes, v)
	}
	return votes, nil
}

// UpdateSuggestionStatus updates the status of a suggestion (open, wip, done, denied)
func UpdateSuggestionStatus(db *sql.DB, suggestionID int, status string) error {
	_, err := db.Exec("UPDATE suggestions SET status = $1 WHERE id = $2", status, suggestionID)
	return err
}

// Comment represents a comment on a suggestion
type Comment struct {
	ID           int       `json:"id"`
	SuggestionID int       `json:"suggestion_id"`
	UserID       int       `json:"user_id"`
	UserName     string    `json:"user_name"`
	Content      string    `json:"content"`
	CreatedAt    time.Time `json:"created_at"`
}

// GetSuggestionComments returns all comments for a suggestion
func GetSuggestionComments(db *sql.DB, suggestionID int) ([]Comment, error) {
	rows, err := db.Query(`
		SELECT c.id, c.suggestion_id, c.user_id, u.name, c.content, c.created_at
		FROM suggestion_comments c
		JOIN users u ON c.user_id = u.id
		WHERE c.suggestion_id = $1
		ORDER BY c.created_at ASC
	`, suggestionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []Comment
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.SuggestionID, &c.UserID, &c.UserName, &c.Content, &c.CreatedAt); err != nil {
			return nil, err
		}
		comments = append(comments, c)
	}
	return comments, nil
}

// GetUserCommentCount returns how many comments a user has on a suggestion
func GetUserCommentCount(db *sql.DB, suggestionID, userID int) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM suggestion_comments WHERE suggestion_id = $1 AND user_id = $2", suggestionID, userID).Scan(&count)
	return count, err
}

// CreateComment adds a comment to a suggestion
func CreateComment(db *sql.DB, suggestionID, userID int, content string) (*Comment, error) {
	var c Comment
	err := db.QueryRow(`
		INSERT INTO suggestion_comments (suggestion_id, user_id, content)
		VALUES ($1, $2, $3)
		RETURNING id, suggestion_id, user_id, content, created_at
	`, suggestionID, userID, content).Scan(&c.ID, &c.SuggestionID, &c.UserID, &c.Content, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	// Get user name
	db.QueryRow("SELECT name FROM users WHERE id = $1", userID).Scan(&c.UserName)
	return &c, nil
}

// DeleteComment removes a comment
func DeleteComment(db *sql.DB, commentID int) error {
	_, err := db.Exec("DELETE FROM suggestion_comments WHERE id = $1", commentID)
	return err
}

// UpdateSuggestion updates the content and/or type of a suggestion
func UpdateSuggestion(db *sql.DB, suggestionID int, content, suggestionType string) error {
	if len(content) > MaxSuggestionLength {
		content = content[:MaxSuggestionLength]
	}
	_, err := db.Exec("UPDATE suggestions SET content = $1, type = $2 WHERE id = $3", content, suggestionType, suggestionID)
	return err
}

// UpdateComment updates the content of a comment
func UpdateComment(db *sql.DB, commentID int, content string) error {
	if len(content) > MaxCommentLength {
		content = content[:MaxCommentLength]
	}
	_, err := db.Exec("UPDATE suggestion_comments SET content = $1 WHERE id = $2", content, commentID)
	return err
}

// GetCommentByID returns a comment by ID
func GetCommentByID(db *sql.DB, commentID int) (*Comment, error) {
	var c Comment
	err := db.QueryRow(`
		SELECT c.id, c.suggestion_id, c.user_id, u.name, c.content, c.created_at
		FROM suggestion_comments c
		JOIN users u ON c.user_id = u.id
		WHERE c.id = $1
	`, commentID).Scan(&c.ID, &c.SuggestionID, &c.UserID, &c.UserName, &c.Content, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

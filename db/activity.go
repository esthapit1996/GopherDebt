package db

import (
	"database/sql"

	"gopherdebt/models"
)

// Activity types
const (
	ActivityExpenseCreated = "expense_created"
	ActivityExpenseDeleted = "expense_deleted"
	ActivitySettlement     = "settlement"
	ActivityPayment        = "payment"
	ActivityMemberAdded    = "member_added"
	ActivityMemberRemoved  = "member_removed"
	ActivityGroupCreated   = "group_created"
)

// LogActivity records an activity in the group
func LogActivity(db *sql.DB, groupID, userID int, actionType, description string, amount *float64, relatedUserID *int) error {
	_, err := db.Exec(
		`INSERT INTO activity_log (group_id, user_id, action_type, description, amount, related_user_id) VALUES ($1, $2, $3, $4, $5, $6)`,
		groupID, userID, actionType, description, amount, relatedUserID,
	)
	return err
}

// GetGroupActivities returns all activities for a group, most recent first
func GetGroupActivities(d *sql.DB, groupID int, limit int) ([]models.Activity, error) {
	return retry("GetGroupActivities", func() ([]models.Activity, error) {
		if limit <= 0 {
			limit = 50 // Default limit
		}

		rows, err := d.Query(
			`SELECT a.id, a.group_id, a.user_id, a.action_type, a.description, a.amount, a.related_user_id, a.created_at,
			u.id, u.email, u.name,
			ru.id, ru.email, ru.name
		FROM activity_log a
		LEFT JOIN users u ON a.user_id = u.id
		LEFT JOIN users ru ON a.related_user_id = ru.id
		WHERE a.group_id = $1
		ORDER BY a.created_at DESC
		LIMIT $2`,
			groupID, limit,
		)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var activities []models.Activity
		for rows.Next() {
			var activity models.Activity
			var user models.User
			var relatedUserID, ruID sql.NullInt64
			var ruEmail, ruName sql.NullString
			var amount sql.NullFloat64

			err := rows.Scan(
				&activity.ID, &activity.GroupID, &activity.UserID, &activity.ActionType, &activity.Description, &amount, &relatedUserID, &activity.CreatedAt,
				&user.ID, &user.Email, &user.Name,
				&ruID, &ruEmail, &ruName,
			)
			if err != nil {
				return nil, err
			}

			activity.User = &user

			if amount.Valid {
				activity.Amount = &amount.Float64
			}

			if relatedUserID.Valid {
				rid := int(relatedUserID.Int64)
				activity.RelatedUserID = &rid
				if ruID.Valid {
					activity.RelatedUser = &models.User{
						ID:    int(ruID.Int64),
						Email: ruEmail.String,
						Name:  ruName.String,
					}
				}
			}

			activities = append(activities, activity)
		}

		return activities, rows.Err()
	})
}

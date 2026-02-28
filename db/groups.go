package db

import (
	"database/sql"

	"gopherdebt/models"
)

func CreateGroup(db *sql.DB, name, description string, createdBy int) (*models.Group, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var group models.Group
	err = tx.QueryRow(
		`INSERT INTO groups (name, description, created_by) VALUES ($1, $2, $3) RETURNING id, name, description, created_by, created_at, updated_at`,
		name, description, createdBy,
	).Scan(&group.ID, &group.Name, &group.Description, &group.CreatedBy, &group.CreatedAt, &group.UpdatedAt)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(`INSERT INTO group_members (group_id, user_id) VALUES ($1, $2)`, group.ID, createdBy)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &group, nil
}

func GetGroupByID(db *sql.DB, groupID int) (*models.Group, error) {
	var group models.Group
	var description sql.NullString

	err := db.QueryRow(
		`SELECT id, name, description, created_by, created_at, updated_at FROM groups WHERE id = $1`,
		groupID,
	).Scan(&group.ID, &group.Name, &description, &group.CreatedBy, &group.CreatedAt, &group.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if description.Valid {
		group.Description = description.String
	}

	members, err := GetGroupMembers(db, groupID)
	if err != nil {
		return nil, err
	}
	group.Members = members
	return &group, nil
}

func GetUserGroups(db *sql.DB, userID int) ([]models.Group, error) {
	rows, err := db.Query(
		`SELECT g.id, g.name, g.description, g.created_by, g.created_at, g.updated_at
		 FROM groups g INNER JOIN group_members gm ON g.id = gm.group_id
		 WHERE gm.user_id = $1 ORDER BY g.created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []models.Group
	for rows.Next() {
		var group models.Group
		var description sql.NullString
		if err := rows.Scan(&group.ID, &group.Name, &description, &group.CreatedBy, &group.CreatedAt, &group.UpdatedAt); err != nil {
			return nil, err
		}
		if description.Valid {
			group.Description = description.String
		}
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

func AddGroupMember(db *sql.DB, groupID, userID int) error {
	_, err := db.Exec(`INSERT INTO group_members (group_id, user_id) VALUES ($1, $2)`, groupID, userID)
	return err
}

func RemoveGroupMember(db *sql.DB, groupID, userID int) error {
	result, err := db.Exec(`DELETE FROM group_members WHERE group_id = $1 AND user_id = $2`, groupID, userID)
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

func GetGroupMembers(db *sql.DB, groupID int) ([]models.User, error) {
	rows, err := db.Query(
		`SELECT u.id, u.email, u.name, u.created_at, u.updated_at
		 FROM users u INNER JOIN group_members gm ON u.id = gm.user_id
		 WHERE gm.group_id = $1 ORDER BY u.name`,
		groupID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []models.User
	for rows.Next() {
		var user models.User
		if err := rows.Scan(&user.ID, &user.Email, &user.Name, &user.CreatedAt, &user.UpdatedAt); err != nil {
			return nil, err
		}
		members = append(members, user)
	}
	return members, rows.Err()
}

func IsGroupMember(db *sql.DB, groupID, userID int) (bool, error) {
	var exists bool
	err := db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM group_members WHERE group_id = $1 AND user_id = $2)`,
		groupID, userID,
	).Scan(&exists)
	return exists, err
}

func DeleteGroup(db *sql.DB, groupID int) error {
	result, err := db.Exec(`DELETE FROM groups WHERE id = $1`, groupID)
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

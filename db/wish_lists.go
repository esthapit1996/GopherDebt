package db

import (
	"database/sql"
	"errors"

	"gopherdebt/models"
)

// ErrAlreadyMember is returned when adding a user who is already in the wish list
var ErrAlreadyMember = errors.New("already a member")

type scanner interface{ Scan(...any) error }

const wishListSelect = `SELECT wl.id, wl.name, wl.created_at,
	(SELECT COUNT(*) FROM wish_items wi WHERE wi.list_id = wl.id),
	(SELECT COALESCE(SUM(wi.cost), 0) FROM wish_items wi WHERE wi.list_id = wl.id)
	FROM wish_lists wl`

func scanWishList(row scanner, l *models.WishList) error {
	return row.Scan(&l.ID, &l.Name, &l.CreatedAt, &l.ItemCount, &l.TotalCost)
}

// wishListMembers runs a (list_id, user id, name, avatar) query and groups the rows by list
func wishListMembers(d *sql.DB, query string, arg int) (map[int][]models.WishMember, error) {
	rows, err := d.Query(query, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	members := map[int][]models.WishMember{}
	for rows.Next() {
		var list_id int
		var m models.WishMember
		if err := rows.Scan(&list_id, &m.ID, &m.Name, &m.Avatar); err != nil {
			return nil, err
		}
		members[list_id] = append(members[list_id], m)
	}
	return members, rows.Err()
}

const wishMemberSelect = `SELECT m.list_id, u.id, u.name, COALESCE(u.avatar, '')
	FROM wish_list_members m JOIN users u ON u.id = m.user_id`

// CreateWishList creates a shared list with the creator as its first member
func CreateWishList(d *sql.DB, userID int, name string) (*models.WishList, error) {
	tx, err := d.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var list_id int
	if err := tx.QueryRow(
		`INSERT INTO wish_lists (name, created_by) VALUES ($1, $2) RETURNING id`,
		name, userID,
	).Scan(&list_id); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(
		`INSERT INTO wish_list_members (list_id, user_id) VALUES ($1, $2)`,
		list_id, userID,
	); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return GetWishList(d, list_id)
}

// GetUserWishLists returns the shared lists the user belongs to, newest first, with members
func GetUserWishLists(d *sql.DB, userID int) ([]models.WishList, error) {
	return retry("GetUserWishLists", func() ([]models.WishList, error) {
		rows, err := d.Query(wishListSelect+`
			JOIN wish_list_members m ON m.list_id = wl.id
			WHERE m.user_id = $1
			ORDER BY wl.created_at DESC`,
			userID,
		)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		lists := []models.WishList{}
		for rows.Next() {
			var l models.WishList
			if err := scanWishList(rows, &l); err != nil {
				return nil, err
			}
			lists = append(lists, l)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}

		members, err := wishListMembers(d, wishMemberSelect+`
			WHERE m.list_id IN (SELECT list_id FROM wish_list_members WHERE user_id = $1)
			ORDER BY m.joined_at`,
			userID,
		)
		if err != nil {
			return nil, err
		}
		for i := range lists {
			lists[i].Members = members[lists[i].ID]
		}
		return lists, nil
	})
}

// GetWishList returns one shared list with its members
func GetWishList(d *sql.DB, listID int) (*models.WishList, error) {
	return retry("GetWishList", func() (*models.WishList, error) {
		var l models.WishList
		err := scanWishList(d.QueryRow(wishListSelect+` WHERE wl.id = $1`, listID), &l)
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}

		members, err := wishListMembers(d, wishMemberSelect+`
			WHERE m.list_id = $1
			ORDER BY m.joined_at`,
			listID,
		)
		if err != nil {
			return nil, err
		}
		l.Members = members[listID]
		return &l, nil
	})
}

// RenameWishList changes a shared list's name
func RenameWishList(d *sql.DB, listID int, name string) error {
	result, err := d.Exec(`UPDATE wish_lists SET name = $1 WHERE id = $2`, name, listID)
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

// IsWishListMember reports whether the user belongs to the shared list
func IsWishListMember(d *sql.DB, listID, userID int) (bool, error) {
	return retry("IsWishListMember", func() (bool, error) {
		var exists bool
		err := d.QueryRow(
			`SELECT EXISTS(SELECT 1 FROM wish_list_members WHERE list_id = $1 AND user_id = $2)`,
			listID, userID,
		).Scan(&exists)
		return exists, err
	})
}

// AddWishListMember adds a user to the shared list
func AddWishListMember(d *sql.DB, listID, userID int) error {
	result, err := d.Exec(
		`INSERT INTO wish_list_members (list_id, user_id) VALUES ($1, $2)
		 ON CONFLICT (list_id, user_id) DO NOTHING`,
		listID, userID,
	)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrAlreadyMember
	}
	return nil
}

// RemoveWishListMember removes a user from the shared list. When the last member
// leaves, the list and its items are deleted; listDeleted reports whether that happened.
func RemoveWishListMember(d *sql.DB, listID, userID int) (listDeleted bool, err error) {
	tx, err := d.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	// Lock the list so two members leaving at once can't both see the other still present
	var locked_id int
	err = tx.QueryRow(`SELECT id FROM wish_lists WHERE id = $1 FOR UPDATE`, listID).Scan(&locked_id)
	if err == sql.ErrNoRows {
		return false, ErrNotFound
	}
	if err != nil {
		return false, err
	}

	result, err := tx.Exec(
		`DELETE FROM wish_list_members WHERE list_id = $1 AND user_id = $2`,
		listID, userID,
	)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if rows == 0 {
		return false, ErrNotFound
	}

	result, err = tx.Exec(
		`DELETE FROM wish_lists WHERE id = $1
		 AND NOT EXISTS (SELECT 1 FROM wish_list_members WHERE list_id = $1)`,
		listID,
	)
	if err != nil {
		return false, err
	}
	rows, err = result.RowsAffected()
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return rows > 0, nil
}

const wishListItemColumns = `w.id, w.user_id, w.name, COALESCE(w.why, ''), COALESCE(w.link, ''), w.cost, w.created_at,
	w.list_id, u.id, u.name, COALESCE(u.avatar, '')`

func scanWishListItem(row scanner, w *models.WishItem) error {
	var m models.WishMember
	if err := row.Scan(&w.ID, &w.UserID, &w.Name, &w.Why, &w.Link, &w.Cost, &w.CreatedAt,
		&w.ListID, &m.ID, &m.Name, &m.Avatar); err != nil {
		return err
	}
	w.AddedBy = &m
	return nil
}

// GetWishListItems returns all items in a shared list, newest first
func GetWishListItems(d *sql.DB, listID int) ([]models.WishItem, error) {
	return retry("GetWishListItems", func() ([]models.WishItem, error) {
		rows, err := d.Query(
			`SELECT `+wishListItemColumns+`
			 FROM wish_items w JOIN users u ON u.id = w.user_id
			 WHERE w.list_id = $1
			 ORDER BY w.created_at DESC`,
			listID,
		)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		items := []models.WishItem{}
		for rows.Next() {
			var w models.WishItem
			if err := scanWishListItem(rows, &w); err != nil {
				return nil, err
			}
			items = append(items, w)
		}
		return items, rows.Err()
	})
}

// CreateWishListItem adds an item to a shared list on behalf of userID
func CreateWishListItem(d *sql.DB, listID, userID int, name, why, link string, cost *float64) (*models.WishItem, error) {
	var item models.WishItem
	err := scanWishListItem(d.QueryRow(
		`WITH w AS (
			INSERT INTO wish_items (user_id, list_id, name, why, link, cost)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING *
		 )
		 SELECT `+wishListItemColumns+` FROM w JOIN users u ON u.id = w.user_id`,
		userID, listID, name, why, link, cost,
	), &item)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// UpdateWishListItem edits an item in a shared list (any member may edit)
func UpdateWishListItem(d *sql.DB, listID, itemID int, name, why, link string, cost *float64) (*models.WishItem, error) {
	var item models.WishItem
	err := scanWishListItem(d.QueryRow(
		`WITH w AS (
			UPDATE wish_items SET name = $1, why = $2, link = $3, cost = $4
			WHERE id = $5 AND list_id = $6
			RETURNING *
		 )
		 SELECT `+wishListItemColumns+` FROM w JOIN users u ON u.id = w.user_id`,
		name, why, link, cost, itemID, listID,
	), &item)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// DeleteWishListItem removes an item from a shared list (any member may delete)
func DeleteWishListItem(d *sql.DB, listID, itemID int) error {
	result, err := d.Exec(`DELETE FROM wish_items WHERE id = $1 AND list_id = $2`, itemID, listID)
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

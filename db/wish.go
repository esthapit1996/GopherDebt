package db

import (
	"database/sql"

	"gopherdebt/models"
)

const wishColumns = `id, user_id, name, COALESCE(why, ''), COALESCE(link, ''), cost, created_at`

func scanWishItem(row interface{ Scan(...any) error }, w *models.WishItem) error {
	return row.Scan(&w.ID, &w.UserID, &w.Name, &w.Why, &w.Link, &w.Cost, &w.CreatedAt)
}

// CreateWishItem adds an item to the user's wishlist. A nil cost means no price set.
func CreateWishItem(d *sql.DB, userID int, name, why, link string, cost *float64) (*models.WishItem, error) {
	var item models.WishItem
	err := scanWishItem(d.QueryRow(
		`INSERT INTO wish_items (user_id, name, why, link, cost)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING `+wishColumns,
		userID, name, why, link, cost,
	), &item)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// GetWishItems returns all wishlist items for a user, newest first
func GetWishItems(d *sql.DB, userID int) ([]models.WishItem, error) {
	return retry("GetWishItems", func() ([]models.WishItem, error) {
		rows, err := d.Query(
			`SELECT `+wishColumns+`
			 FROM wish_items
			 WHERE user_id = $1 AND list_id IS NULL
			 ORDER BY created_at DESC`,
			userID,
		)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var items []models.WishItem
		for rows.Next() {
			var w models.WishItem
			if err := scanWishItem(rows, &w); err != nil {
				return nil, err
			}
			items = append(items, w)
		}
		return items, rows.Err()
	})
}

// UpdateWishItem updates a wishlist item if it belongs to the user
func UpdateWishItem(d *sql.DB, itemID, userID int, name, why, link string, cost *float64) (*models.WishItem, error) {
	var item models.WishItem
	err := scanWishItem(d.QueryRow(
		`UPDATE wish_items SET name = $1, why = $2, link = $3, cost = $4
		 WHERE id = $5 AND user_id = $6 AND list_id IS NULL
		 RETURNING `+wishColumns,
		name, why, link, cost, itemID, userID,
	), &item)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// DeleteWishItem removes a wishlist item (only if owned by the user)
func DeleteWishItem(d *sql.DB, itemID, userID int) error {
	result, err := d.Exec(
		`DELETE FROM wish_items WHERE id = $1 AND user_id = $2 AND list_id IS NULL`,
		itemID, userID,
	)
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

package db

import (
	"database/sql"

	"gopherdebt/models"
)

// CreateWishItem adds an item to the user's wishlist
func CreateWishItem(d *sql.DB, userID int, name, link string, cost float64) (*models.WishItem, error) {
	var item models.WishItem
	err := d.QueryRow(
		`INSERT INTO wish_items (user_id, name, link, cost)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, user_id, name, link, cost, created_at`,
		userID, name, link, cost,
	).Scan(&item.ID, &item.UserID, &item.Name, &item.Link, &item.Cost, &item.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// GetWishItems returns all wishlist items for a user, newest first
func GetWishItems(d *sql.DB, userID int) ([]models.WishItem, error) {
	return retry("GetWishItems", func() ([]models.WishItem, error) {
		rows, err := d.Query(
			`SELECT id, user_id, name, COALESCE(link, ''), cost, created_at
			 FROM wish_items
			 WHERE user_id = $1
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
			if err := rows.Scan(&w.ID, &w.UserID, &w.Name, &w.Link, &w.Cost, &w.CreatedAt); err != nil {
				return nil, err
			}
			items = append(items, w)
		}
		return items, rows.Err()
	})
}

// UpdateWishItem updates a wishlist item if it belongs to the user
func UpdateWishItem(d *sql.DB, itemID, userID int, name, link string, cost float64) (*models.WishItem, error) {
	var item models.WishItem
	err := d.QueryRow(
		`UPDATE wish_items SET name = $1, link = $2, cost = $3
		 WHERE id = $4 AND user_id = $5
		 RETURNING id, user_id, name, link, cost, created_at`,
		name, link, cost, itemID, userID,
	).Scan(&item.ID, &item.UserID, &item.Name, &item.Link, &item.Cost, &item.CreatedAt)
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
		`DELETE FROM wish_items WHERE id = $1 AND user_id = $2`,
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

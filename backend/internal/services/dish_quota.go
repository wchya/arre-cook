package services

import (
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ninimenu/internal/models"
)

const DishQuota = 500

var ErrDishQuota = errors.New("菜谱已达 500 道上限")

// InsertDish is the single insertion boundary for user/family recipes. The
// owner row serializes quota checks across processes, including on SQLite.
// Callers may pass an existing transaction; GORM uses a nested savepoint.
func InsertDish(db *gorm.DB, dish *models.Dish) error {
	return db.Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&models.Dish{})
		switch {
		case dish.FamilyID != 0:
			if err := tx.Model(&models.Family{}).Where("id = ?", dish.FamilyID).UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
				return err
			}
			var member models.FamilyMember
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("family_id = ? AND user_id = ?", dish.FamilyID, dish.OwnerID).First(&member).Error; err != nil {
				return err
			}
			query = query.Where("family_id = ?", dish.FamilyID)
		case dish.OwnerID != 0:
			if err := tx.Model(&models.User{}).Where("id = ?", dish.OwnerID).UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
				return err
			}
			var user models.User
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, dish.OwnerID).Error; err != nil {
				return err
			}
			query = query.Where("owner_id = ? AND family_id = 0", dish.OwnerID)
		default:
			return tx.Create(dish).Error // curated public catalog, administrators only
		}
		// A bounded current read also works when the caller already established
		// a MySQL REPEATABLE READ snapshot before entering this savepoint.
		var ids []uint
		if err := query.Clauses(clause.Locking{Strength: "UPDATE"}).Limit(DishQuota).Pluck("id", &ids).Error; err != nil {
			return err
		}
		if len(ids) >= DishQuota {
			return ErrDishQuota
		}
		return tx.Create(dish).Error
	})
}

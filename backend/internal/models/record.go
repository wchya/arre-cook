package models

import "time"

type MealRecord struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	UserID    uint      `json:"-" gorm:"not null;default:0;uniqueIndex:idx_meal_records_user_unique_day,priority:1;index:idx_meal_records_user_date,priority:1"`
	DishID    uint      `json:"dish_id" gorm:"not null;index;uniqueIndex:idx_meal_records_user_unique_day,priority:2"`
	DishName  string    `json:"dish_name" gorm:"not null"`
	MealType  string    `json:"meal_type" gorm:"not null;index;uniqueIndex:idx_meal_records_user_unique_day,priority:3;index:idx_meal_records_date_type,priority:2"`
	MealDate  string    `json:"meal_date" gorm:"not null;index;uniqueIndex:idx_meal_records_user_unique_day,priority:4;index:idx_meal_records_date_type,priority:1;index:idx_meal_records_user_date,priority:2"`
	Rating    int       `json:"rating" gorm:"default:0"`
	Remark    string    `json:"remark"`
	Mood      string    `json:"mood" gorm:"index"`
	Photo     string    `json:"photo"`
	CreatedAt time.Time `json:"created_at" gorm:"index"`
}

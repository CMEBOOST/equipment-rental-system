package model

type Role struct {
	ID          int16  `gorm:"primaryKey"`
	Name        string `gorm:"unique;size:20"`
	Description string `gorm:"size:255"`
}

func (Role) TableName() string { return "roles" }

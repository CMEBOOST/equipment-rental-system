package dto

type CreateRentalRequest struct {
	UserID    string `json:"user_id" binding:"required,uuid4"`
	ProductID string `json:"product_id" binding:"required,uuid4"`
	StartDate string `json:"start_date" binding:"required"`
	DueDate   string `json:"due_date" binding:"required"`
}

type RequestRentalRequest struct {
	ProductID string `json:"product_id" binding:"required,uuid4"`
	StartDate string `json:"start_date" binding:"required"`
	DueDate   string `json:"due_date" binding:"required"`
}

type ReturnRentalRequest struct {
	ReturnDate string `json:"return_date" binding:"required"`
}

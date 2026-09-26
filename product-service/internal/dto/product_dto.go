package dto

// CreateProductRequest is the body for POST /products (admin, staff —
// CONTRACT.md §8.2). CategoryID must reference an existing category; the
// handler checks that against the DB rather than trying to express it as a
// binding tag.
type CreateProductRequest struct {
	CategoryID  string  `json:"category_id" binding:"required,uuid"`
	Name        string  `json:"name" binding:"required,max=150"`
	Description string  `json:"description" binding:"max=10000"`
	PricePerDay float64 `json:"price_per_day" binding:"required,gt=0"`
	ImageURL    string  `json:"image_url" binding:"max=255"`
}

// UpdateProductRequest is the body for PUT /products/{id} (admin, staff).
// Every field is a pointer so "omitted" and "sent empty" stay distinguishable
// — the same reasoning as user-service/internal/dto/user_dto.go's
// UpdateProfileRequest.
type UpdateProductRequest struct {
	CategoryID  *string  `json:"category_id" binding:"omitempty,uuid"`
	Name        *string  `json:"name" binding:"omitempty,max=150"`
	Description *string  `json:"description" binding:"omitempty,max=10000"`
	PricePerDay *float64 `json:"price_per_day" binding:"omitempty,gt=0"`
	ImageURL    *string  `json:"image_url" binding:"omitempty,max=255"`
}

// ChangeStatusRequest is the body for PATCH /products/{id}/status
// (admin, staff, or an internal service call — CONTRACT.md §8.2).
type ChangeStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=available rented maintenance"`
}

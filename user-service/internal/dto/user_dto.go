package dto

// See auth_dto.go for why every field that maps to a bounded-width column
// carries a matching max= tag: email VARCHAR(255), username VARCHAR(50),
// full_name VARCHAR(120), phone VARCHAR(20) (migration 000001).
//
// On the pointer fields below, omitempty is what makes "field absent" and
// "field present" distinguishable: validator dereferences a non-nil pointer
// and applies the remaining rules to the value, and skips validation
// entirely when the field was not sent at all.

type UpdateProfileRequest struct {
	FullName *string `json:"full_name" binding:"omitempty,max=120"`
	Phone    *string `json:"phone" binding:"omitempty,max=20"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required,min=8,max=72"`
}

type CreateUserRequest struct {
	Email    string `json:"email" binding:"required,email,max=255"`
	Username string `json:"username" binding:"required,min=3,max=50"`
	Password string `json:"password" binding:"required,min=8,max=72"`
	FullName string `json:"full_name" binding:"max=120"`
	Phone    string `json:"phone" binding:"max=20"`
	Role     string `json:"role" binding:"required,oneof=admin staff customer"`
	IsActive *bool  `json:"is_active"`
}

// UpdateUserRequest previously had no validation at all on Email/Username,
// even though both are unique, bounded columns that this endpoint can
// change: an admin could PUT a malformed address and it would be stored.
type UpdateUserRequest struct {
	Email    *string `json:"email" binding:"omitempty,email,max=255"`
	Username *string `json:"username" binding:"omitempty,min=3,max=50"`
	FullName *string `json:"full_name" binding:"omitempty,max=120"`
	Phone    *string `json:"phone" binding:"omitempty,max=20"`
}

type ChangeRoleRequest struct {
	Role string `json:"role" binding:"required,oneof=admin staff customer"`
}

type ChangeStatusRequest struct {
	IsActive bool `json:"is_active"`
}

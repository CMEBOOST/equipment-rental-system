package dto

type UpdateProfileRequest struct {
	FullName *string `json:"full_name"`
	Phone    *string `json:"phone"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required,min=8"`
}

type CreateUserRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Username string `json:"username" binding:"required,min=3,max=50"`
	Password string `json:"password" binding:"required,min=8"`
	FullName string `json:"full_name"`
	Phone    string `json:"phone"`
	Role     string `json:"role" binding:"required,oneof=admin staff customer"`
	IsActive *bool  `json:"is_active"`
}

type UpdateUserRequest struct {
	Email    *string `json:"email"`
	Username *string `json:"username"`
	FullName *string `json:"full_name"`
	Phone    *string `json:"phone"`
}

type ChangeRoleRequest struct {
	Role string `json:"role" binding:"required,oneof=admin staff customer"`
}

type ChangeStatusRequest struct {
	IsActive bool `json:"is_active"`
}

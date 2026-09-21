package dto

// The max= tags below mirror the column widths in migration 000001
// (email VARCHAR(255), username VARCHAR(50), full_name VARCHAR(120),
// phone VARCHAR(20)). Without them an over-length value reached Postgres and
// came back as a raw driver error, which the handlers' default branch turned
// into 500 INTERNAL_ERROR -- reporting a client mistake as a server fault.
// Binding validation rejects it as 400 VALIDATION_ERROR before any DB call.
//
// Password is capped at 72 bytes for a different reason: bcrypt refuses
// inputs longer than that, so an over-long password would otherwise fail
// inside GenerateFromPassword and surface as a 500 as well.

type RegisterRequest struct {
	Email    string `json:"email" binding:"required,email,max=255"`
	Username string `json:"username" binding:"required,min=3,max=50"`
	Password string `json:"password" binding:"required,min=8,max=72"`
	FullName string `json:"full_name" binding:"max=120"`
	Phone    string `json:"phone" binding:"max=20"`
}

// LoginRequest's email is bounded because it is recorded verbatim in
// login_logs.email_attempted (VARCHAR(255)); an over-length value would make
// that audit insert fail silently. Password is deliberately *not* bounded
// here: a wrong password of any length must stay a 401, not become a 400
// that tells the caller something about the stored credential.
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email,max=255"`
	Password string `json:"password" binding:"required"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type LogoutRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

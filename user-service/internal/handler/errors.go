package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// respondNotFound writes the standard 404 body for a missing user. It is the
// only place the "ไม่พบผู้ใช้" message is spelled out, so every /users/{id}
// endpoint returns a byte-identical not-found response.
func respondNotFound(c *gin.Context) {
	c.JSON(http.StatusNotFound, gin.H{"success": false,
		"error": gin.H{"code": "NOT_FOUND", "message": "ไม่พบผู้ใช้", "details": nil}})
}

// respondInternalError writes the standard 500 body.
func respondInternalError(c *gin.Context, err error) {
	c.JSON(http.StatusInternalServerError, gin.H{"success": false,
		"error": gin.H{"code": "INTERNAL_ERROR", "message": err.Error(), "details": nil}})
}

// mapUserServiceError turns an error coming out of UserService into the
// correct HTTP response: only gorm.ErrRecordNotFound means "no such user"
// (404); anything else is a genuine failure and must be reported as 500.
//
// The handlers used to collapse every error to 404, which was not just
// imprecise but actively misleading for the multi-write operations. E.g.
// UserService.ChangeStatus writes is_active and *then* revokes the target's
// refresh tokens; if that second write fails, the account really has been
// disabled, yet the caller was told "user not found". Those services are
// still non-transactional (a known, accepted limitation), so the honest
// answer for a partial failure is 500 -- "something went wrong, re-read the
// state" -- never a 404 that denies the write happened at all.
func mapUserServiceError(c *gin.Context, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		respondNotFound(c)
		return
	}
	respondInternalError(c, err)
}

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

// respondValidationError writes the standard 400 body for a request the
// caller can fix.
func respondValidationError(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, gin.H{"success": false,
		"error": gin.H{"code": "VALIDATION_ERROR", "message": message, "details": nil}})
}

// parseBoolQuery reads an optional true/false query parameter. It returns
// (nil, true) when the parameter is absent or empty (meaning "no filter"),
// a pointer to the parsed value when it is well-formed, and ok=false when
// the caller sent something else.
//
// The `ok` result exists so a typo cannot be silently reinterpreted: the
// older `b := v == "true"` idiom turned ?is_active=1, ?is_active=yes and
// ?is_active=flase all into a filter for *false*, quietly answering a
// different question than the one asked.
func parseBoolQuery(c *gin.Context, name string) (*bool, bool) {
	raw, present := c.GetQuery(name)
	if !present || raw == "" {
		return nil, true
	}
	switch raw {
	case "true":
		v := true
		return &v, true
	case "false":
		v := false
		return &v, true
	default:
		return nil, false
	}
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

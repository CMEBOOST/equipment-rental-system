package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/equipment-rental-system/product-service/internal/repository"
	"github.com/equipment-rental-system/product-service/internal/service"
)

// respondNotFound writes the standard 404 body — CONTRACT.md §4.4.
func respondNotFound(c *gin.Context, message string) {
	c.JSON(http.StatusNotFound, gin.H{"success": false,
		"error": gin.H{"code": "NOT_FOUND", "message": message, "details": nil}})
}

// respondValidationError writes the standard 400 body for a request the
// caller can fix.
func respondValidationError(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, gin.H{"success": false,
		"error": gin.H{"code": "VALIDATION_ERROR", "message": message, "details": nil}})
}

// respondConflict writes the standard 409 body.
func respondConflict(c *gin.Context, message string) {
	c.JSON(http.StatusConflict, gin.H{"success": false,
		"error": gin.H{"code": "CONFLICT", "message": message, "details": nil}})
}

// respondInternalError writes the standard 500 body.
func respondInternalError(c *gin.Context, err error) {
	c.JSON(http.StatusInternalServerError, gin.H{"success": false,
		"error": gin.H{"code": "INTERNAL_ERROR", "message": err.Error(), "details": nil}})
}

// mapProductError turns an error coming out of ProductService into the
// correct HTTP response — mirrors user-service/internal/handler/errors.go's
// mapUserServiceError, extended with product-service's own sentinels.
func mapProductError(c *gin.Context, err error) {
	switch {
	case service.IsNotFound(err):
		respondNotFound(c, "ไม่พบสินค้า")
	case errors.Is(err, service.ErrCategoryNotFound):
		respondValidationError(c, "ไม่พบหมวดหมู่สินค้าที่ระบุ")
	case errors.Is(err, service.ErrProductRented):
		respondConflict(c, "ไม่สามารถลบสินค้าที่กำลังถูกเช่าอยู่ได้")
	case errors.Is(err, repository.ErrStatusConflict):
		respondConflict(c, "สินค้าไม่ได้อยู่ในสถานะ available จึงเปลี่ยนเป็น rented ไม่ได้")
	default:
		respondInternalError(c, err)
	}
}

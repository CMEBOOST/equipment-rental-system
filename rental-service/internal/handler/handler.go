package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/rental-service/internal/client"
	"github.com/equipment-rental-system/rental-service/internal/dto"
	"github.com/equipment-rental-system/rental-service/internal/repository"
	"github.com/equipment-rental-system/rental-service/internal/service"
)

type RentalHandler struct{ service *service.RentalService }

func NewRentalHandler(service *service.RentalService) *RentalHandler {
	return &RentalHandler{service: service}
}

func (h *RentalHandler) List(c *gin.Context) {
	page, limit := pageAndLimit(c)
	rentals, total, err := h.service.List(repository.RentalFilter{Status: c.Query("status"), Page: page, Limit: limit, Sort: c.Query("sort"), Order: c.Query("order")})
	if err != nil {
		internal(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rentals, "meta": meta(page, limit, total)})
}

func (h *RentalHandler) MyList(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		unauthenticated(c)
		return
	}
	page, limit := pageAndLimit(c)
	rentals, total, err := h.service.List(repository.RentalFilter{UserID: userID.String(), Status: c.Query("status"), Page: page, Limit: limit, Sort: c.Query("sort"), Order: c.Query("order")})
	if err != nil {
		internal(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rentals, "meta": meta(page, limit, total)})
}

func (h *RentalHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		notFound(c)
		return
	}
	rental, err := h.service.Get(id)
	if err != nil {
		mapError(c, err)
		return
	}
	role, _ := c.Get("role")
	userID, ok := currentUserID(c)
	if role != "admin" && role != "staff" && (!ok || rental.UserID != userID) {
		forbidden(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rental})
}

func (h *RentalHandler) Create(c *gin.Context) {
	var req dto.CreateRentalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		validation(c, err.Error())
		return
	}
	userID, _ := uuid.Parse(req.UserID)
	productID, _ := uuid.Parse(req.ProductID)
	rental, err := h.service.Create(userID, productID, req.StartDate, req.DueDate, accessToken(c), false)
	if err != nil {
		mapError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": rental})
}

func (h *RentalHandler) Request(c *gin.Context) {
	var req dto.RequestRentalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		validation(c, err.Error())
		return
	}
	userID, ok := currentUserID(c)
	if !ok {
		unauthenticated(c)
		return
	}
	productID, _ := uuid.Parse(req.ProductID)
	rental, err := h.service.Create(userID, productID, req.StartDate, req.DueDate, accessToken(c), true)
	if err != nil {
		mapError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": rental})
}

func (h *RentalHandler) Approve(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		notFound(c)
		return
	}
	rental, err := h.service.Approve(id, accessToken(c))
	if err != nil {
		mapError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rental})
}

func (h *RentalHandler) Return(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		notFound(c)
		return
	}
	var req dto.ReturnRentalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		validation(c, err.Error())
		return
	}
	rental, err := h.service.Return(id, req.ReturnDate, accessToken(c))
	if err != nil {
		mapError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rental})
}

func Health(database *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		sqlDB, err := database.DB()
		if err != nil || sqlDB.Ping() != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": gin.H{"code": "INTERNAL_ERROR", "message": "database unavailable", "details": nil}})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"status": "ok", "service": "rental-service", "db": "up"}})
	}
}

func pageAndLimit(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	return page, limit
}
func meta(page, limit int, total int64) gin.H {
	return gin.H{"page": page, "limit": limit, "total": total, "total_pages": (total + int64(limit) - 1) / int64(limit)}
}
func currentUserID(c *gin.Context) (uuid.UUID, bool) {
	value, ok := c.Get("user_id")
	id, valid := value.(uuid.UUID)
	return id, ok && valid
}
func accessToken(c *gin.Context) string {
	token, _ := c.Get("access_token")
	value, _ := token.(string)
	return value
}
func validation(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"code": "VALIDATION_ERROR", "message": message, "details": nil}})
}
func unauthenticated(c *gin.Context) {
	c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": gin.H{"code": "UNAUTHENTICATED", "message": "ไม่มี token หรือ token ไม่ถูกต้อง", "details": nil}})
}
func forbidden(c *gin.Context) {
	c.JSON(http.StatusForbidden, gin.H{"success": false, "error": gin.H{"code": "FORBIDDEN", "message": "บทบาทไม่มีสิทธิ์เข้าถึง resource นี้", "details": nil}})
}
func notFound(c *gin.Context) {
	c.JSON(http.StatusNotFound, gin.H{"success": false, "error": gin.H{"code": "NOT_FOUND", "message": "ไม่พบรายการเช่า", "details": nil}})
}
func internal(c *gin.Context, err error) {
	c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"code": "INTERNAL_ERROR", "message": err.Error(), "details": nil}})
}
func mapError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		notFound(c)
	case errors.Is(err, client.ErrProductNotFound):
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": gin.H{"code": "NOT_FOUND", "message": "ไม่พบสินค้า", "details": nil}})
	case errors.Is(err, client.ErrProductUnavailable), errors.Is(err, service.ErrInvalidState):
		c.JSON(http.StatusConflict, gin.H{"success": false, "error": gin.H{"code": "CONFLICT", "message": err.Error(), "details": nil}})
	case errors.Is(err, service.ErrInvalidDates), errors.Is(err, service.ErrInvalidDateFormat), errors.Is(err, service.ErrInvalidReturnDate):
		validation(c, err.Error())
	case errors.Is(err, client.ErrAccountInactive):
		c.JSON(http.StatusForbidden, gin.H{"success": false, "error": gin.H{"code": "ACCOUNT_DISABLED", "message": err.Error(), "details": nil}})
	case errors.Is(err, client.ErrDependency):
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": gin.H{"code": "INTERNAL_ERROR", "message": "dependent service unavailable", "details": nil}})
	default:
		internal(c, err)
	}
}

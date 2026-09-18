package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/equipment-rental-system/user-service/internal/dto"
	"github.com/equipment-rental-system/user-service/internal/service"
)

type UserHandler struct {
	svc *service.UserService
}

func NewUserHandler(svc *service.UserService) *UserHandler {
	return &UserHandler{svc: svc}
}

func currentUserID(c *gin.Context) (uuid.UUID, bool) {
	raw, ok := c.Get("user_id")
	if !ok {
		return uuid.UUID{}, false
	}
	id, err := uuid.Parse(raw.(string))
	return id, err == nil
}

func (h *UserHandler) Me(c *gin.Context) {
	id, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": gin.H{"code": "UNAUTHENTICATED", "message": "unauthenticated", "details": nil}})
		return
	}
	u, err := h.svc.GetProfile(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": gin.H{"code": "NOT_FOUND", "message": "ไม่พบผู้ใช้", "details": nil}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"id": u.ID, "email": u.Email, "username": u.Username, "full_name": u.FullName,
		"phone": u.Phone, "role": u.Role.Name, "is_active": u.IsActive,
		"created_at": u.CreatedAt, "updated_at": u.UpdatedAt,
	}})
}

func (h *UserHandler) UpdateMe(c *gin.Context) {
	id, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": gin.H{"code": "UNAUTHENTICATED", "message": "unauthenticated", "details": nil}})
		return
	}
	var req dto.UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"code": "VALIDATION_ERROR", "message": err.Error(), "details": nil}})
		return
	}
	u, err := h.svc.UpdateProfile(id, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"code": "INTERNAL_ERROR", "message": err.Error(), "details": nil}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"id": u.ID, "full_name": u.FullName, "phone": u.Phone, "updated_at": u.UpdatedAt,
	}})
}

func (h *UserHandler) ChangePassword(c *gin.Context) {
	id, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": gin.H{"code": "UNAUTHENTICATED", "message": "unauthenticated", "details": nil}})
		return
	}
	var req dto.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"code": "VALIDATION_ERROR", "message": err.Error(), "details": nil}})
		return
	}
	if err := h.svc.ChangePassword(id, req, 12); err != nil {
		if err == service.ErrInvalidCredentials {
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": gin.H{"code": "INVALID_CREDENTIALS", "message": "รหัสผ่านเดิมไม่ถูกต้อง", "details": nil}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"code": "INTERNAL_ERROR", "message": err.Error(), "details": nil}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"message": "เปลี่ยนรหัสผ่านเรียบร้อย กรุณาเข้าสู่ระบบใหม่"}})
}

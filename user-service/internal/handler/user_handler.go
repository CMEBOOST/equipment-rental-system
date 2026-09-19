package handler

import (
	"errors"
	"net/http"
	"strconv"

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
		mapUserServiceError(c, err)
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
	if err := h.svc.ChangePassword(id, req); err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidCredentials):
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": gin.H{"code": "INVALID_CREDENTIALS", "message": "รหัสผ่านเดิมไม่ถูกต้อง", "details": nil}})
		case errors.Is(err, service.ErrWeakPassword):
			c.JSON(http.StatusUnprocessableEntity, gin.H{"success": false, "error": gin.H{"code": "WEAK_PASSWORD", "message": "รหัสผ่านไม่ตรงเกณฑ์", "details": nil}})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"code": "INTERNAL_ERROR", "message": err.Error(), "details": nil}})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"message": "เปลี่ยนรหัสผ่านเรียบร้อย กรุณาเข้าสู่ระบบใหม่"}})
}

func (h *UserHandler) MyLoginLogs(c *gin.Context) {
	id, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": gin.H{"code": "UNAUTHENTICATED", "message": "unauthenticated", "details": nil}})
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	// Mirror LoginLogRepo.ListForUser's own clamping (CONTRACT.md §4.5) here so the
	// handler's meta.page/meta.limit reflect the values actually used for the query,
	// and so totalPages below never divides by a zero or otherwise invalid limit.
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	logs, total, err := h.svc.LoginLogsForUser(id, page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"code": "INTERNAL_ERROR", "message": err.Error(), "details": nil}})
		return
	}
	items := make([]gin.H, 0, len(logs))
	for _, l := range logs {
		items = append(items, gin.H{"id": l.ID, "success": l.Success, "ip_address": l.IPAddress, "user_agent": l.UserAgent, "created_at": l.CreatedAt})
	}
	totalPages := (total + int64(limit) - 1) / int64(limit)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items, "meta": gin.H{"page": page, "limit": limit, "total": total, "total_pages": totalPages}})
}

func (h *UserHandler) MySessions(c *gin.Context) {
	id, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": gin.H{"code": "UNAUTHENTICATED", "message": "unauthenticated", "details": nil}})
		return
	}
	sessions, err := h.svc.ActiveSessions(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"code": "INTERNAL_ERROR", "message": err.Error(), "details": nil}})
		return
	}
	items := make([]gin.H, 0, len(sessions))
	for _, s := range sessions {
		items = append(items, gin.H{"id": s.ID, "ip_address": s.IPAddress, "user_agent": s.UserAgent, "created_at": s.CreatedAt, "expires_at": s.ExpiresAt})
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items})
}

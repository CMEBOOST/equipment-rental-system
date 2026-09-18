package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/user-service/internal/dto"
	"github.com/equipment-rental-system/user-service/internal/repository"
	"github.com/equipment-rental-system/user-service/internal/service"
)

type AdminUserHandler struct {
	svc   *service.UserService
	roles *repository.RoleRepo
	cost  int
}

func NewAdminUserHandler(svc *service.UserService, roles *repository.RoleRepo, cost int) *AdminUserHandler {
	return &AdminUserHandler{svc: svc, roles: roles, cost: cost}
}

func (h *AdminUserHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	// Mirror UserRepo.List's own clamping (and UserHandler.MyLoginLogs's identical
	// guard, added in Task 11/e97be4a) here so the handler's meta.page/meta.limit
	// reflect the values actually used for the query, and so totalPages below
	// never divides by a zero or otherwise invalid limit.
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	var isActive *bool
	if v := c.Query("is_active"); v != "" {
		b := v == "true"
		isActive = &b
	}
	users, total, err := h.svc.ListUsers(repository.UserFilter{
		Query: c.Query("q"), Role: c.Query("role"), IsActive: isActive,
		Page: page, Limit: limit, Sort: c.Query("sort"), Order: c.Query("order"),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"code": "INTERNAL_ERROR", "message": err.Error(), "details": nil}})
		return
	}
	items := make([]gin.H, 0, len(users))
	for _, u := range users {
		items = append(items, gin.H{
			"id": u.ID, "email": u.Email, "username": u.Username, "full_name": u.FullName,
			"phone": u.Phone, "role": u.Role.Name, "is_active": u.IsActive, "created_at": u.CreatedAt,
		})
	}
	totalPages := (total + int64(limit) - 1) / int64(limit)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items, "meta": gin.H{"page": page, "limit": limit, "total": total, "total_pages": totalPages}})
}

func (h *AdminUserHandler) Create(c *gin.Context) {
	var req dto.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"code": "VALIDATION_ERROR", "message": err.Error(), "details": nil}})
		return
	}
	u, err := h.svc.CreateUser(req, h.roles, h.cost)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrEmailExists):
			c.JSON(http.StatusConflict, gin.H{"success": false, "error": gin.H{"code": "EMAIL_ALREADY_EXISTS", "message": "อีเมลนี้ถูกใช้งานแล้ว", "details": nil}})
		case errors.Is(err, service.ErrUsernameExists):
			c.JSON(http.StatusConflict, gin.H{"success": false, "error": gin.H{"code": "USERNAME_ALREADY_EXISTS", "message": "username นี้ถูกใช้แล้ว", "details": nil}})
		case errors.Is(err, service.ErrWeakPassword):
			c.JSON(http.StatusUnprocessableEntity, gin.H{"success": false, "error": gin.H{"code": "WEAK_PASSWORD", "message": "รหัสผ่านไม่ตรงเกณฑ์", "details": nil}})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"code": "INTERNAL_ERROR", "message": err.Error(), "details": nil}})
		}
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": gin.H{
		"id": u.ID, "email": u.Email, "username": u.Username, "full_name": u.FullName,
		"phone": u.Phone, "role": u.Role.Name, "is_active": u.IsActive, "created_at": u.CreatedAt,
	}})
}

// Get returns a single user by ID. Available to admin and staff.
func (h *AdminUserHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": gin.H{"code": "NOT_FOUND", "message": "ไม่พบผู้ใช้", "details": nil}})
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

// Update edits full_name/phone/email/username for a user by ID. Admin-only.
func (h *AdminUserHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": gin.H{"code": "NOT_FOUND", "message": "ไม่พบผู้ใช้", "details": nil}})
		return
	}
	var req dto.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"code": "VALIDATION_ERROR", "message": err.Error(), "details": nil}})
		return
	}
	u, err := h.svc.UpdateUser(id, req)
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": gin.H{"code": "NOT_FOUND", "message": "ไม่พบผู้ใช้", "details": nil}})
		case errors.Is(err, service.ErrEmailExists):
			c.JSON(http.StatusConflict, gin.H{"success": false, "error": gin.H{"code": "EMAIL_ALREADY_EXISTS", "message": "อีเมลนี้ถูกใช้งานแล้ว", "details": nil}})
		case errors.Is(err, service.ErrUsernameExists):
			c.JSON(http.StatusConflict, gin.H{"success": false, "error": gin.H{"code": "USERNAME_ALREADY_EXISTS", "message": "username นี้ถูกใช้แล้ว", "details": nil}})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"code": "INTERNAL_ERROR", "message": err.Error(), "details": nil}})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"id": u.ID, "email": u.Email, "username": u.Username, "full_name": u.FullName,
		"phone": u.Phone, "role": u.Role.Name, "is_active": u.IsActive, "updated_at": u.UpdatedAt,
	}})
}

// Delete soft-deletes a user by ID and revokes all of their refresh tokens.
// Admin-only. An admin may not delete their own account.
func (h *AdminUserHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": gin.H{"code": "NOT_FOUND", "message": "ไม่พบผู้ใช้", "details": nil}})
		return
	}
	// Self-deletion check happens before any DB mutation below.
	if requesterID, ok := currentUserID(c); ok && requesterID == id {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "error": gin.H{"code": "FORBIDDEN", "message": "ไม่สามารถลบบัญชีตัวเองได้", "details": nil}})
		return
	}
	if err := h.svc.DeleteUser(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": gin.H{"code": "NOT_FOUND", "message": "ไม่พบผู้ใช้", "details": nil}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"message": "ลบผู้ใช้เรียบร้อย"}})
}

// ChangeRole reassigns a user's role by name. Admin-only. The DTO's
// binding:"oneof=admin staff customer" tag rejects any unrecognized role
// name with 400 before this handler even runs, so ChangeRole's own
// gorm.ErrRecordNotFound path only covers an unknown user ID.
func (h *AdminUserHandler) ChangeRole(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": gin.H{"code": "NOT_FOUND", "message": "ไม่พบผู้ใช้", "details": nil}})
		return
	}
	var req dto.ChangeRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"code": "VALIDATION_ERROR", "message": err.Error(), "details": nil}})
		return
	}
	u, err := h.svc.ChangeRole(id, req.Role, h.roles)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": gin.H{"code": "NOT_FOUND", "message": "ไม่พบผู้ใช้", "details": nil}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"id": u.ID, "role": u.Role.Name, "updated_at": u.UpdatedAt}})
}

// ChangeStatus enables or disables a user's account. Admin-only. Disabling
// revokes all of the target's refresh tokens (see UserService.ChangeStatus).
// An admin may not disable/enable their own account -- the self-status-change
// check happens before any DB write, mirroring Delete's self-delete guard.
func (h *AdminUserHandler) ChangeStatus(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": gin.H{"code": "NOT_FOUND", "message": "ไม่พบผู้ใช้", "details": nil}})
		return
	}
	if requesterID, ok := currentUserID(c); ok && requesterID == id {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "error": gin.H{"code": "FORBIDDEN", "message": "ไม่สามารถเปลี่ยนสถานะบัญชีตัวเองได้", "details": nil}})
		return
	}
	var req dto.ChangeStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"code": "VALIDATION_ERROR", "message": err.Error(), "details": nil}})
		return
	}
	u, err := h.svc.ChangeStatus(id, req.IsActive)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": gin.H{"code": "NOT_FOUND", "message": "ไม่พบผู้ใช้", "details": nil}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"id": u.ID, "is_active": u.IsActive, "updated_at": u.UpdatedAt}})
}

// LoginLogsForUser returns the paginated login-log history for an arbitrary
// user by ID. Admin-only. A nonexistent user ID 404s (see the h.svc.GetProfile
// existence check below) rather than returning 200 with an empty list, so
// "no login history" and "no such user" stay distinguishable, consistent
// with every sibling /users/{id} endpoint.
//
// page/limit are clamped here, in the handler, BEFORE calling
// UserService.LoginLogsForUser and BEFORE computing totalPages -- mirroring
// LoginLogRepo.ListForUser's own clamping, UserHandler.MyLoginLogs's guard
// (Task 11/e97be4a), and AdminUserHandler.List's identical guard
// (Task 13/4baa211). Without this, ?limit=0 (or a non-numeric value, which
// strconv.Atoi silently turns into 0) reaches
// totalPages := (total + int64(limit) - 1) / int64(limit) and divides by
// zero -- the same bug class that shipped twice already in this codebase.
func (h *AdminUserHandler) LoginLogsForUser(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": gin.H{"code": "NOT_FOUND", "message": "ไม่พบผู้ใช้", "details": nil}})
		return
	}
	// A well-formed but nonexistent (or deleted) user ID must 404, not
	// silently succeed with an empty list: LoginLogRepo.ListForUser never
	// errors for an unknown user ID, it just returns zero rows, so without
	// this explicit existence check (mirroring Get's own h.svc.GetProfile
	// call) "no login history" and "no such user" would be indistinguishable.
	if _, err := h.svc.GetProfile(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": gin.H{"code": "NOT_FOUND", "message": "ไม่พบผู้ใช้", "details": nil}})
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
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
		items = append(items, gin.H{"id": l.ID, "email_attempted": l.EmailAttempted, "success": l.Success, "ip_address": l.IPAddress, "user_agent": l.UserAgent, "created_at": l.CreatedAt})
	}
	totalPages := (total + int64(limit) - 1) / int64(limit)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items, "meta": gin.H{"page": page, "limit": limit, "total": total, "total_pages": totalPages}})
}

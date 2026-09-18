package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/equipment-rental-system/user-service/internal/dto"
	"github.com/equipment-rental-system/user-service/internal/service"
)

type AuthHandler struct {
	svc *service.AuthService
}

func NewAuthHandler(svc *service.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req dto.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"code": "VALIDATION_ERROR", "message": err.Error(), "details": nil}})
		return
	}
	u, err := h.svc.Register(req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrEmailExists):
			c.JSON(http.StatusConflict, gin.H{"success": false, "error": gin.H{"code": "EMAIL_ALREADY_EXISTS", "message": "อีเมลนี้ถูกใช้งานแล้ว", "details": nil}})
		case errors.Is(err, service.ErrUsernameExists):
			c.JSON(http.StatusConflict, gin.H{"success": false, "error": gin.H{"code": "USERNAME_ALREADY_EXISTS", "message": "username นี้ถูกใช้แล้ว", "details": nil}})
		case errors.Is(err, service.ErrWeakPassword):
			c.JSON(http.StatusUnprocessableEntity, gin.H{"success": false, "error": gin.H{"code": "WEAK_PASSWORD", "message": "รหัสผ่านไม่ตรงเกณฑ์", "details": nil}})
		case errors.Is(err, service.ErrConflict):
			c.JSON(http.StatusConflict, gin.H{"success": false, "error": gin.H{"code": "CONFLICT", "message": "conflict", "details": nil}})
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

func (h *AuthHandler) Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"code": "VALIDATION_ERROR", "message": err.Error(), "details": nil}})
		return
	}
	access, refresh, expiresIn, user, err := h.svc.Login(req, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidCredentials):
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": gin.H{"code": "INVALID_CREDENTIALS", "message": "อีเมลหรือรหัสผ่านไม่ถูกต้อง", "details": nil}})
		case errors.Is(err, service.ErrAccountDisabled):
			c.JSON(http.StatusForbidden, gin.H{"success": false, "error": gin.H{"code": "ACCOUNT_DISABLED", "message": "บัญชีถูกปิดการใช้งาน", "details": nil}})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"code": "INTERNAL_ERROR", "message": err.Error(), "details": nil}})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"access_token": access, "refresh_token": refresh, "token_type": "Bearer", "expires_in": expiresIn,
		"user": gin.H{"id": user.ID, "email": user.Email, "username": user.Username, "full_name": user.FullName, "role": user.Role.Name},
	}})
}

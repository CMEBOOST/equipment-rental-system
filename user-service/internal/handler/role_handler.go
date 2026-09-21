package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/equipment-rental-system/user-service/internal/repository"
)

type RoleHandler struct {
	roles *repository.RoleRepo
}

func NewRoleHandler(roles *repository.RoleRepo) *RoleHandler {
	return &RoleHandler{roles: roles}
}

// List returns all roles, ordered by id. Available to admin and staff.
func (h *RoleHandler) List(c *gin.Context) {
	roles, err := h.roles.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"code": "INTERNAL_ERROR", "message": err.Error(), "details": nil}})
		return
	}
	items := make([]gin.H, 0, len(roles))
	for _, r := range roles {
		items = append(items, gin.H{"id": r.ID, "name": r.Name, "description": r.Description})
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items})
}

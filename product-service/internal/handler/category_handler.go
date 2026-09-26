package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/equipment-rental-system/product-service/internal/repository"
)

type CategoryHandler struct {
	categories *repository.CategoryRepo
}

func NewCategoryHandler(categories *repository.CategoryRepo) *CategoryHandler {
	return &CategoryHandler{categories: categories}
}

// List returns every category. Available to any authenticated role
// (CONTRACT.md §8.2) — there is no admin-only write path yet because the
// contract does not ask for one; categories are seeded by migration
// 000001_init_schema.
func (h *CategoryHandler) List(c *gin.Context) {
	categories, err := h.categories.List()
	if err != nil {
		respondInternalError(c, err)
		return
	}
	items := make([]gin.H, 0, len(categories))
	for _, cat := range categories {
		items = append(items, gin.H{
			"id": cat.ID, "name": cat.Name, "description": cat.Description, "created_at": cat.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items})
}

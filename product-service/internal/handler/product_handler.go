package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/equipment-rental-system/product-service/internal/dto"
	"github.com/equipment-rental-system/product-service/internal/model"
	"github.com/equipment-rental-system/product-service/internal/repository"
	"github.com/equipment-rental-system/product-service/internal/service"
)

type ProductHandler struct {
	svc *service.ProductService
}

func NewProductHandler(svc *service.ProductService) *ProductHandler {
	return &ProductHandler{svc: svc}
}

// productJSON is the one place a product's response shape is spelled out, so
// List/Get/Create/Update/ChangeStatus all return byte-identical field names
// (CONTRACT.md §8.2's minimum field list, plus category_id/category and
// image_url/updated_at which the endpoint table's "ต้องมีอย่างน้อย" note
// leaves room for).
func productJSON(p *model.Product) gin.H {
	return gin.H{
		"id": p.ID, "category_id": p.CategoryID, "category": p.Category.Name,
		"name": p.Name, "description": p.Description, "price_per_day": p.PricePerDay,
		"status": p.Status, "image_url": p.ImageURL,
		"created_at": p.CreatedAt, "updated_at": p.UpdatedAt,
	}
}

// List returns products with search/filter/pagination — available to every
// authenticated role (CONTRACT.md §8.2).
//
// Query params: q (name ILIKE), category_id, status, page, limit, sort, order
// — same names/defaults/clamping as user-service's /users list
// (CONTRACT.md §4.5).
func (h *ProductHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	filter := repository.ProductFilter{
		Query: c.Query("q"), Status: c.Query("status"),
		Page: page, Limit: limit, Sort: c.Query("sort"), Order: c.Query("order"),
	}
	if raw := c.Query("category_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			respondValidationError(c, "category_id must be a UUID")
			return
		}
		filter.CategoryID = &id
	}
	if filter.Status != "" && !model.IsValidStatus(filter.Status) {
		respondValidationError(c, "status must be one of: available, rented, maintenance")
		return
	}

	products, total, err := h.svc.List(filter)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	items := make([]gin.H, 0, len(products))
	for i := range products {
		items = append(items, productJSON(&products[i]))
	}
	totalPages := (total + int64(limit) - 1) / int64(limit)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items,
		"meta": gin.H{"page": page, "limit": limit, "total": total, "total_pages": totalPages}})
}

// Get returns a single product by id — available to every authenticated role.
func (h *ProductHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondNotFound(c, "ไม่พบสินค้า")
		return
	}
	p, err := h.svc.Get(id)
	if err != nil {
		mapProductError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": productJSON(p)})
}

// Create adds a product — admin, staff only (CONTRACT.md §8.2).
func (h *ProductHandler) Create(c *gin.Context) {
	var req dto.CreateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondValidationError(c, err.Error())
		return
	}
	categoryID, err := uuid.Parse(req.CategoryID)
	if err != nil {
		respondValidationError(c, "category_id must be a UUID")
		return
	}
	p, err := h.svc.Create(categoryID, req.Name, req.Description, req.PricePerDay, req.ImageURL)
	if err != nil {
		mapProductError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": productJSON(p)})
}

// Update edits a product — admin, staff only.
func (h *ProductHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondNotFound(c, "ไม่พบสินค้า")
		return
	}
	var req dto.UpdateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondValidationError(c, err.Error())
		return
	}
	var categoryID *uuid.UUID
	if req.CategoryID != nil {
		parsed, err := uuid.Parse(*req.CategoryID)
		if err != nil {
			respondValidationError(c, "category_id must be a UUID")
			return
		}
		categoryID = &parsed
	}
	p, err := h.svc.Update(id, categoryID, req.Name, req.Description, req.ImageURL, req.PricePerDay)
	if err != nil {
		mapProductError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": productJSON(p)})
}

// Delete removes a product — admin only (CONTRACT.md §8.2 gives DELETE a
// narrower role list than POST/PUT).
func (h *ProductHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondNotFound(c, "ไม่พบสินค้า")
		return
	}
	if err := h.svc.Delete(id); err != nil {
		mapProductError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"message": "ลบสินค้าเรียบร้อย"}})
}

// ChangeStatus flips a product between available/rented/maintenance.
// Reached either by an admin/staff Bearer token (through Kong) or by
// rental-service calling directly with X-Internal-Key
// (middleware.RequireInternalKeyOrRole — CONTRACT.md §7.3/§8.2).
func (h *ProductHandler) ChangeStatus(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondNotFound(c, "ไม่พบสินค้า")
		return
	}
	var req dto.ChangeStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondValidationError(c, err.Error())
		return
	}
	p, err := h.svc.ChangeStatus(id, req.Status)
	if err != nil {
		mapProductError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": productJSON(p)})
}

package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/product-service/internal/handler"
	"github.com/equipment-rental-system/product-service/internal/model"
	"github.com/equipment-rental-system/product-service/internal/repository"
)

func TestCategoryHandler_List_ReturnsSeededCategoriesSortedByName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Category{}))
	require.NoError(t, db.Create(&model.Category{ID: uuid.New(), Name: "เครื่องเสียง"}).Error)
	require.NoError(t, db.Create(&model.Category{ID: uuid.New(), Name: "กล้อง"}).Error)

	ch := handler.NewCategoryHandler(repository.NewCategoryRepo(db))
	r := gin.New()
	r.GET("/categories", ch.List)

	req := httptest.NewRequest(http.MethodGet, "/categories", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Success bool             `json:"success"`
		Data    []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.True(t, body.Success)
	require.Len(t, body.Data, 2)
	// CategoryRepo.List orders by name asc — "กล้อง" sorts before "เครื่องเสียง".
	require.Equal(t, "กล้อง", body.Data[0]["name"])
	require.Equal(t, "เครื่องเสียง", body.Data[1]["name"])
}

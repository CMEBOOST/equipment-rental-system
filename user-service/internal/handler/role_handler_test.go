package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/user-service/internal/handler"
	"github.com/equipment-rental-system/user-service/internal/model"
	"github.com/equipment-rental-system/user-service/internal/repository"
)

func TestRoleHandler_List_ReturnsSeededRolesInOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Role{}))
	require.NoError(t, db.Create(&model.Role{ID: 1, Name: "admin"}).Error)
	require.NoError(t, db.Create(&model.Role{ID: 2, Name: "staff"}).Error)
	require.NoError(t, db.Create(&model.Role{ID: 3, Name: "customer"}).Error)

	rh := handler.NewRoleHandler(repository.NewRoleRepo(db))
	r := gin.New()
	r.GET("/roles", rh.List)

	req := httptest.NewRequest(http.MethodGet, "/roles", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Data, 3)
	require.Equal(t, "admin", body.Data[0]["name"])
}

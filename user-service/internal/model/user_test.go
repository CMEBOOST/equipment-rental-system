package model_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/equipment-rental-system/user-service/internal/model"
)

func TestUser_TableName(t *testing.T) {
	assert.Equal(t, "users", model.User{}.TableName())
}

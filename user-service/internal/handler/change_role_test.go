package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/equipment-rental-system/user-service/internal/handler"
	"github.com/equipment-rental-system/user-service/internal/model"
	"github.com/equipment-rental-system/user-service/internal/repository"
)

// ChangePassword, ChangeStatus (on disable) and DeleteUser all revoke the
// target's refresh tokens; ChangeRole did not. A demoted admin's existing
// access token still carries role=admin until it expires (up to 15
// minutes), which is authoritative at any service that reads the claim
// instead of calling /auth/verify. And unlike Delete/ChangeStatus,
// ChangeRole had no self-action guard, so an admin could demote themselves
// out of the ability to undo it.

func patchRole(t *testing.T, h *handler.AdminUserHandler, targetID, callerID uuid.UUID, role string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"role": role})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("PATCH", "/api/v1/users/"+targetID.String()+"/role", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: targetID.String()}}
	c.Set("user_id", callerID.String())
	h.ChangeRole(c)
	return w
}

func TestAdminUserHandler_ChangeRole_RevokesTargetUsersSessions(t *testing.T) {
	h, db := setupAdminUserHandler(t)
	targetID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, email, username, password_hash, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?)`,
		targetID.String(), "demote@example.com", "demoteuser", "hash", 1, true,
	).Error)
	refreshRepo := repository.NewRefreshTokenRepo(db)
	require.NoError(t, refreshRepo.Create(&model.RefreshToken{
		ID: uuid.New(), UserID: targetID, TokenHash: "role-tok-1", ExpiresAt: time.Now().Add(time.Hour),
	}))
	require.NoError(t, refreshRepo.Create(&model.RefreshToken{
		ID: uuid.New(), UserID: targetID, TokenHash: "role-tok-2", ExpiresAt: time.Now().Add(time.Hour),
	}))
	active, err := refreshRepo.ListActiveForUser(targetID)
	require.NoError(t, err)
	require.Len(t, active, 2, "sanity check: sessions active before the role change")

	w := patchRole(t, h, targetID, uuid.New(), "customer")

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	updated, err := repository.NewUserRepo(db).FindByID(targetID)
	require.NoError(t, err)
	require.Equal(t, "customer", updated.Role.Name)

	active, err = refreshRepo.ListActiveForUser(targetID)
	require.NoError(t, err)
	require.Empty(t, active, "a role change must revoke the target's sessions, like its sibling operations")
}

// Another user's sessions must not be touched.
func TestAdminUserHandler_ChangeRole_DoesNotRevokeOtherUsersSessions(t *testing.T) {
	h, db := setupAdminUserHandler(t)
	targetID, bystanderID := uuid.New(), uuid.New()
	for _, u := range []struct {
		id             uuid.UUID
		email, name    string
		roleID         int
	}{
		{targetID, "t@example.com", "tuser", 2},
		{bystanderID, "b@example.com", "buser", 3},
	} {
		require.NoError(t, db.Exec(
			`INSERT INTO users (id, email, username, password_hash, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?)`,
			u.id.String(), u.email, u.name, "hash", u.roleID, true,
		).Error)
	}
	refreshRepo := repository.NewRefreshTokenRepo(db)
	require.NoError(t, refreshRepo.Create(&model.RefreshToken{
		ID: uuid.New(), UserID: bystanderID, TokenHash: "bystander-tok", ExpiresAt: time.Now().Add(time.Hour),
	}))

	w := patchRole(t, h, targetID, uuid.New(), "customer")
	require.Equal(t, http.StatusOK, w.Code)

	active, err := refreshRepo.ListActiveForUser(bystanderID)
	require.NoError(t, err)
	require.Len(t, active, 1, "only the target's sessions may be revoked")
}

func TestAdminUserHandler_ChangeRole_SelfChange_Returns403_AndDoesNotChange(t *testing.T) {
	h, db := setupAdminUserHandler(t)
	adminID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO users (id, email, username, password_hash, role_id, is_active) VALUES (?, ?, ?, ?, ?, ?)`,
		adminID.String(), "self-role@example.com", "selfroleuser", "hash", 1, true,
	).Error)

	w := patchRole(t, h, adminID, adminID, "customer")

	require.Equal(t, http.StatusForbidden, w.Code)
	requireErrorCode(t, w.Body.Bytes(), "FORBIDDEN")

	unchanged, err := repository.NewUserRepo(db).FindByID(adminID)
	require.NoError(t, err)
	require.Equal(t, "admin", unchanged.Role.Name, "the guard must run before any DB write")
}

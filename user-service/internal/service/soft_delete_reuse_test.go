package service_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/equipment-rental-system/user-service/internal/config"
	"github.com/equipment-rental-system/user-service/internal/dto"
	"github.com/equipment-rental-system/user-service/internal/repository"
	"github.com/equipment-rental-system/user-service/internal/service"
)

// Deleting a user is a soft delete, and every read path is scoped to
// `deleted_at IS NULL`. Before migration 000003 the unique constraints on
// users.email/users.username were *not* scoped that way, so a deleted
// account kept owning its email forever: the duplicate pre-check saw
// nothing, the INSERT hit a constraint the application could not see, and
// the caller got a bare 409 CONFLICT with no usable error code. These tests
// pin the behavior users actually expect -- deleting an account frees its
// email and username -- while keeping live accounts mutually exclusive.

func TestAuthService_Register_AfterSoftDelete_EmailAndUsernameAreReusable(t *testing.T) {
	authSvc, db := setupAuthServiceWithDB(t, ":memory:")
	userSvc := service.NewUserService(
		repository.NewUserRepo(db), repository.NewRefreshTokenRepo(db),
		&config.Config{BCryptCost: 4}, repository.NewLoginLogRepo(db),
	)
	req := dto.RegisterRequest{
		Email: "recycle@example.com", Username: "recycle",
		Password: "Passw0rd1", FullName: "First Owner",
	}

	first, err := authSvc.Register(req)
	require.NoError(t, err)

	require.NoError(t, userSvc.DeleteUser(first.ID))

	// Same email AND same username as the deleted account.
	second, err := authSvc.Register(req)

	require.NoError(t, err, "a soft-deleted account's email/username must become available again")
	require.NotEqual(t, first.ID, second.ID, "re-registration must create a new user, not resurrect the deleted one")

	// The live lookup resolves to the new account, and the old row is still
	// on disk (soft-deleted), so no history was destroyed to achieve this.
	found, err := repository.NewUserRepo(db).FindByEmail("recycle@example.com")
	require.NoError(t, err)
	require.Equal(t, second.ID, found.ID)

	var rowCount int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM users WHERE email = ?`, "recycle@example.com").Scan(&rowCount).Error)
	require.EqualValues(t, 2, rowCount, "the soft-deleted row must still exist alongside the new one")
}

// Guard rail for the migration: scoping uniqueness to live rows must not
// let two *live* users share an email.
func TestAuthService_Register_LiveDuplicateEmail_StillRejected(t *testing.T) {
	authSvc, _ := setupAuthServiceWithDB(t, ":memory:")
	req := dto.RegisterRequest{Email: "live@example.com", Username: "live1", Password: "Passw0rd1"}
	_, err := authSvc.Register(req)
	require.NoError(t, err)

	req.Username = "live2"
	_, err = authSvc.Register(req)

	require.ErrorIs(t, err, service.ErrEmailExists)
}

func TestUserService_CreateUser_AfterSoftDelete_EmailIsReusable(t *testing.T) {
	userSvc, roleRepo, _ := setupUserServiceForCreate(t, ":memory:")
	req := dto.CreateUserRequest{
		Email: "admin-recycle@example.com", Username: "adminrecycle",
		Password: "Passw0rd1", Role: "staff",
	}

	first, err := userSvc.CreateUser(req, roleRepo, 4)
	require.NoError(t, err)
	require.NoError(t, userSvc.DeleteUser(first.ID))

	second, err := userSvc.CreateUser(req, roleRepo, 4)

	require.NoError(t, err, "admin-created accounts must free their email on delete too")
	require.NotEqual(t, first.ID, second.ID)
}

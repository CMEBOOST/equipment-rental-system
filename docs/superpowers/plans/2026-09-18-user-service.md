# User Management Service (JWT & RBAC) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `user-service` (Go + Gin + GORM + PostgreSQL) implementing all 20 endpoints from the design doc: auth (register/login/refresh/logout/verify), self-service profile, and admin user management with RBAC.

**Architecture:** Layered Go service (`router → middleware → handler → service → repository → GORM/Postgres`), HS256 JWT access tokens (15m) + opaque SHA-256-hashed refresh tokens (7d) rotated on every refresh. Three roles (`admin`/`staff`/`customer`) enforced via `RequireRole` middleware reading the `role` claim.

**Tech Stack:** Go 1.23 · Gin · GORM (`gorm.io/driver/postgres`) · `golang-jwt/jwt/v5` · `golang.org/x/crypto/bcrypt` (cost 12) · `golang-migrate/migrate/v4` · `google/uuid` · `go-playground/validator/v10` · `stretchr/testify` (test assertions) · PostgreSQL 16

**Spec:** [docs/user-management-service-design.md](../../user-management-service-design.md) implements this plan's requirements; [docs/superpowers/specs/2026-09-18-kong-api-gateway-design.md](../specs/2026-09-18-kong-api-gateway-design.md) requires the `iss` claim added in Task 3.

## Global Constraints

- Base path: `/api/v1`, service port `8081`, DB `user_db` on `user-db:5432` (CONTRACT.md §1)
- Response envelope: `{"success":true,"data":...}` / `{"success":false,"error":{"code","message","details"}}` (CONTRACT.md §4)
- JSON keys `snake_case`, timestamps ISO 8601 UTC, all IDs UUID v4 string (CONTRACT.md §11)
- JWT: HS256, access TTL 15m, claims `sub,email,username,role,iss,iat,exp,jti` — `iss` = `"equipment-rental-system"` (Kong spec §3)
- Refresh token: opaque 64-hex string, store only SHA-256 hash, TTL 168h, rotate on every use
- Passwords: bcrypt cost 12, min 8 chars with ≥1 letter + ≥1 digit
- Every error path returns the exact `error.code` values listed in the design doc §6.4 — do not invent new codes

---

## Sprint 0 — Foundation

### Task 1: Project scaffold, config, DB connection, migrations, health check

**Files:**
- Create: `user-service/go.mod`, `user-service/cmd/api/main.go`
- Create: `user-service/internal/config/config.go`
- Create: `user-service/internal/db/db.go`
- Create: `user-service/internal/handler/health_handler.go`
- Create: `user-service/internal/router/router.go`
- Create: `user-service/migrations/000001_init_schema.up.sql`, `000001_init_schema.down.sql`
- Create: `user-service/migrations/000002_seed_roles.up.sql`, `000002_seed_roles.down.sql`
- Create: `user-service/.env.example`
- Test: `user-service/internal/handler/health_handler_test.go`

**Interfaces:**
- Produces: `config.Load() (*config.Config, error)` with fields `AppPort, DBHost, DBPort, DBUser, DBPassword, DBName, JWTSecret, JWTAccessTTL, JWTRefreshTTL, InternalAPIKey, BCryptCost string/time.Duration/int`
- Produces: `db.Connect(cfg *config.Config) (*gorm.DB, error)`
- Produces: `router.New(db *gorm.DB, cfg *config.Config) *gin.Engine`
- Produces: `handler.Health(db *gorm.DB) gin.HandlerFunc`

- [ ] **Step 1: Init go.mod and dependencies**

```bash
cd user-service
go mod init github.com/equipment-rental-system/user-service
go get github.com/gin-gonic/gin gorm.io/gorm gorm.io/driver/postgres \
  github.com/golang-jwt/jwt/v5 golang.org/x/crypto/bcrypt \
  github.com/golang-migrate/migrate/v4 github.com/google/uuid \
  github.com/go-playground/validator/v10 github.com/joho/godotenv \
  github.com/stretchr/testify
```

- [ ] **Step 2: Write failing test for health handler**

```go
// user-service/internal/handler/health_handler_test.go
package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/equipment-rental-system/user-service/internal/handler"
)

func TestHealth_DBUp_Returns200(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/health", handler.Health(nil)) // nil db + ping func below simulates "up"

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, true, body["success"])
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/handler/... -run TestHealth_DBUp_Returns200 -v`
Expected: FAIL — `handler.Health` undefined

- [ ] **Step 4: Implement config, db, health handler, router, main**

```go
// user-service/internal/config/config.go
package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	AppPort        string
	DBHost, DBPort, DBUser, DBPassword, DBName string
	JWTSecret      string
	JWTAccessTTL   time.Duration
	JWTRefreshTTL  time.Duration
	InternalAPIKey string
	BCryptCost     int
}

func Load() (*Config, error) {
	_ = godotenv.Load() // ignore error: absent in prod containers, present in dev
	accessTTL, err := time.ParseDuration(getEnv("JWT_ACCESS_TTL", "15m"))
	if err != nil {
		return nil, err
	}
	refreshTTL, err := time.ParseDuration(getEnv("JWT_REFRESH_TTL", "168h"))
	if err != nil {
		return nil, err
	}
	cost, err := strconv.Atoi(getEnv("BCRYPT_COST", "12"))
	if err != nil {
		return nil, err
	}
	return &Config{
		AppPort:        getEnv("APP_PORT", "8081"),
		DBHost:         getEnv("DB_HOST", "localhost"),
		DBPort:         getEnv("DB_PORT", "5432"),
		DBUser:         getEnv("DB_USER", "user_service"),
		DBPassword:     getEnv("DB_PASSWORD", "secret"),
		DBName:         getEnv("DB_NAME", "user_db"),
		JWTSecret:      getEnv("JWT_SECRET", ""),
		JWTAccessTTL:   accessTTL,
		JWTRefreshTTL:  refreshTTL,
		InternalAPIKey: getEnv("INTERNAL_API_KEY", ""),
		BCryptCost:     cost,
	}, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
```

```go
// user-service/internal/db/db.go
package db

import (
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/user-service/internal/config"
)

func Connect(cfg *config.Config) (*gorm.DB, error) {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName)
	return gorm.Open(postgres.Open(dsn), &gorm.Config{})
}
```

```go
// user-service/internal/handler/health_handler.go
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func Health(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		dbStatus := "up"
		if db != nil {
			sqlDB, err := db.DB()
			if err != nil || sqlDB.Ping() != nil {
				dbStatus = "down"
			}
		}
		if dbStatus == "down" {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"success": false,
				"error":   gin.H{"code": "INTERNAL_ERROR", "message": "database unavailable", "details": nil},
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    gin.H{"status": "ok", "service": "user-service", "db": dbStatus},
		})
	}
}
```

```go
// user-service/internal/router/router.go
package router

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/user-service/internal/config"
	"github.com/equipment-rental-system/user-service/internal/handler"
)

func New(database *gorm.DB, cfg *config.Config) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/health", handler.Health(database))
	// v1 group + auth/user/role routes are added by later tasks
	return r
}
```

```go
// user-service/cmd/api/main.go
package main

import (
	"log"

	"github.com/equipment-rental-system/user-service/internal/config"
	"github.com/equipment-rental-system/user-service/internal/db"
	"github.com/equipment-rental-system/user-service/internal/router"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}
	database, err := db.Connect(cfg)
	if err != nil {
		log.Fatalf("db connect error: %v", err)
	}
	r := router.New(database, cfg)
	if err := r.Run(":" + cfg.AppPort); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
```

```sql
-- user-service/migrations/000001_init_schema.up.sql
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE roles (
    id          SMALLINT     PRIMARY KEY,
    name        VARCHAR(20)  NOT NULL UNIQUE,
    description VARCHAR(255) NOT NULL DEFAULT ''
);

CREATE TABLE users (
    id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    email         VARCHAR(255) NOT NULL UNIQUE,
    username      VARCHAR(50)  NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    full_name     VARCHAR(120) NOT NULL DEFAULT '',
    phone         VARCHAR(20)  NOT NULL DEFAULT '',
    role_id       SMALLINT     NOT NULL REFERENCES roles(id),
    is_active     BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at    TIMESTAMPTZ  NULL
);
CREATE INDEX idx_users_role_id    ON users(role_id);
CREATE INDEX idx_users_deleted_at ON users(deleted_at);

CREATE TABLE refresh_tokens (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  VARCHAR(64)  NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ  NOT NULL,
    revoked_at  TIMESTAMPTZ  NULL,
    ip_address  VARCHAR(45)  NOT NULL DEFAULT '',
    user_agent  VARCHAR(255) NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens(user_id);

CREATE TABLE login_logs (
    id              BIGSERIAL    PRIMARY KEY,
    user_id         UUID         NULL REFERENCES users(id) ON DELETE SET NULL,
    email_attempted VARCHAR(255) NOT NULL,
    success         BOOLEAN      NOT NULL,
    ip_address      VARCHAR(45)  NOT NULL DEFAULT '',
    user_agent      VARCHAR(255) NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX idx_login_logs_user_id    ON login_logs(user_id);
CREATE INDEX idx_login_logs_created_at ON login_logs(created_at);
```

```sql
-- user-service/migrations/000001_init_schema.down.sql
DROP TABLE IF EXISTS login_logs;
DROP TABLE IF EXISTS refresh_tokens;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS roles;
```

```sql
-- user-service/migrations/000002_seed_roles.up.sql
INSERT INTO roles (id, name, description) VALUES
    (1, 'admin',    'ผู้ดูแลระบบ จัดการผู้ใช้ บทบาท สินค้า และการเช่าทั้งหมด'),
    (2, 'staff',    'เจ้าหน้าที่ จัดการสินค้า สร้างรายการเช่า และบันทึกการคืน'),
    (3, 'customer', 'ลูกค้า ดูสินค้า สร้างคำขอเช่า และดูประวัติการเช่าของตนเอง');
```

```sql
-- user-service/migrations/000002_seed_roles.down.sql
DELETE FROM roles WHERE id IN (1,2,3);
```

```bash
# user-service/.env.example
APP_PORT=8081
DB_HOST=user-db
DB_PORT=5432
DB_USER=user_service
DB_PASSWORD=secret
DB_NAME=user_db
JWT_SECRET=dev-secret-please-change-min-32-characters
JWT_ACCESS_TTL=15m
JWT_REFRESH_TTL=168h
INTERNAL_API_KEY=dev-internal-key
BCRYPT_COST=12
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/handler/... -run TestHealth_DBUp_Returns200 -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add user-service/
git commit -m "feat(user-service): scaffold project, config, db, health check, migrations"
```

---

## Sprint 1 — Auth core

### Task 2: Models

**Files:**
- Create: `user-service/internal/model/role.go`
- Create: `user-service/internal/model/user.go`
- Create: `user-service/internal/model/refresh_token.go`
- Create: `user-service/internal/model/login_log.go`
- Test: `user-service/internal/model/user_test.go`

**Interfaces:**
- Produces: `model.Role{ID int16, Name string, Description string}`
- Produces: `model.User{ID uuid.UUID, Email, Username, PasswordHash, FullName, Phone string, RoleID int16, Role Role, IsActive bool, CreatedAt, UpdatedAt time.Time, DeletedAt gorm.DeletedAt}`
- Produces: `model.RefreshToken{ID uuid.UUID, UserID uuid.UUID, TokenHash string, ExpiresAt time.Time, RevokedAt *time.Time, IPAddress, UserAgent string, CreatedAt time.Time}`
- Produces: `model.LoginLog{ID int64, UserID *uuid.UUID, EmailAttempted string, Success bool, IPAddress, UserAgent string, CreatedAt time.Time}`

- [ ] **Step 1: Write failing test asserting table names match migration**

```go
// user-service/internal/model/user_test.go
package model_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/equipment-rental-system/user-service/internal/model"
)

func TestUser_TableName(t *testing.T) {
	assert.Equal(t, "users", model.User{}.TableName())
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/model/... -v`
Expected: FAIL — `model.User` undefined

- [ ] **Step 3: Implement models**

```go
// user-service/internal/model/role.go
package model

type Role struct {
	ID          int16  `gorm:"primaryKey"`
	Name        string `gorm:"unique;size:20"`
	Description string `gorm:"size:255"`
}

func (Role) TableName() string { return "roles" }
```

```go
// user-service/internal/model/user.go
package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type User struct {
	ID           uuid.UUID `gorm:"primaryKey;default:gen_random_uuid()"`
	Email        string    `gorm:"unique;size:255"`
	Username     string    `gorm:"unique;size:50"`
	PasswordHash string    `gorm:"size:255"`
	FullName     string    `gorm:"size:120"`
	Phone        string    `gorm:"size:20"`
	RoleID       int16
	Role         Role `gorm:"foreignKey:RoleID"`
	IsActive     bool `gorm:"default:true"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    gorm.DeletedAt `gorm:"index"`
}

func (User) TableName() string { return "users" }
```

```go
// user-service/internal/model/refresh_token.go
package model

import (
	"time"

	"github.com/google/uuid"
)

type RefreshToken struct {
	ID         uuid.UUID `gorm:"primaryKey;default:gen_random_uuid()"`
	UserID     uuid.UUID
	TokenHash  string `gorm:"unique;size:64"`
	ExpiresAt  time.Time
	RevokedAt  *time.Time
	IPAddress  string `gorm:"size:45"`
	UserAgent  string `gorm:"size:255"`
	CreatedAt  time.Time
}

func (RefreshToken) TableName() string { return "refresh_tokens" }
```

```go
// user-service/internal/model/login_log.go
package model

import (
	"time"

	"github.com/google/uuid"
)

type LoginLog struct {
	ID              int64 `gorm:"primaryKey"`
	UserID          *uuid.UUID
	EmailAttempted  string `gorm:"size:255"`
	Success         bool
	IPAddress       string `gorm:"size:45"`
	UserAgent       string `gorm:"size:255"`
	CreatedAt       time.Time
}

func (LoginLog) TableName() string { return "login_logs" }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/model/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add user-service/internal/model/
git commit -m "feat(user-service): add GORM models for roles, users, refresh_tokens, login_logs"
```

---

### Task 3: Token service (JWT issue/parse + refresh token hashing)

**Files:**
- Create: `user-service/internal/service/token_service.go`
- Test: `user-service/internal/service/token_service_test.go`

**Interfaces:**
- Consumes: `model.User` (Task 2)
- Produces: `service.NewTokenService(secret string, accessTTL time.Duration) *TokenService`
- Produces: `(*TokenService) GenerateAccessToken(u model.User) (token string, expiresIn int, err error)`
- Produces: `(*TokenService) ParseAccessToken(token string) (claims *Claims, err error)` where `Claims{Sub, Email, Username, Role, Iss string; Exp, Iat int64; Jti string}`
- Produces: `service.NewOpaqueRefreshToken() (raw string, err error)` — 32 random bytes as 64 hex chars
- Produces: `service.HashRefreshToken(raw string) string` — SHA-256 hex

- [ ] **Step 1: Write failing test — token round-trip carries `iss` and `role`**

```go
// user-service/internal/service/token_service_test.go
package service_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/equipment-rental-system/user-service/internal/model"
	"github.com/equipment-rental-system/user-service/internal/service"
)

func TestTokenService_GenerateAndParse_RoundTrip(t *testing.T) {
	ts := service.NewTokenService("test-secret-min-32-characters-ok", 15*time.Minute)
	u := model.User{
		ID:       uuid.New(),
		Email:    "somchai@example.com",
		Username: "somchai",
		Role:     model.Role{Name: "customer"},
	}

	token, expiresIn, err := ts.GenerateAccessToken(u)
	require.NoError(t, err)
	require.Equal(t, 900, expiresIn)

	claims, err := ts.ParseAccessToken(token)
	require.NoError(t, err)
	require.Equal(t, u.ID.String(), claims.Sub)
	require.Equal(t, "customer", claims.Role)
	require.Equal(t, "equipment-rental-system", claims.Iss)
}

func TestHashRefreshToken_Deterministic(t *testing.T) {
	raw, err := service.NewOpaqueRefreshToken()
	require.NoError(t, err)
	require.Len(t, raw, 64)
	require.Equal(t, service.HashRefreshToken(raw), service.HashRefreshToken(raw))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service/... -run TestTokenService -v`
Expected: FAIL — `service.NewTokenService` undefined

- [ ] **Step 3: Implement token service**

```go
// user-service/internal/service/token_service.go
package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/equipment-rental-system/user-service/internal/model"
)

const tokenIssuer = "equipment-rental-system"

type Claims struct {
	Sub      string `json:"sub"`
	Email    string `json:"email"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Iss      string `json:"iss"`
	jwt.RegisteredClaims
}

type TokenService struct {
	secret    []byte
	accessTTL time.Duration
}

func NewTokenService(secret string, accessTTL time.Duration) *TokenService {
	return &TokenService{secret: []byte(secret), accessTTL: accessTTL}
}

func (s *TokenService) GenerateAccessToken(u model.User) (string, int, error) {
	now := time.Now()
	claims := Claims{
		Sub:      u.ID.String(),
		Email:    u.Email,
		Username: u.Username,
		Role:     u.Role.Name,
		Iss:      tokenIssuer,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.accessTTL)),
			ID:        uuid.New().String(),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", 0, err
	}
	return signed, int(s.accessTTL.Seconds()), nil
}

func (s *TokenService) ParseAccessToken(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		return s.secret, nil
	})
	if err != nil {
		return nil, err
	}
	return claims, nil
}

func NewOpaqueRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func HashRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/service/... -run TestTokenService -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add user-service/internal/service/token_service.go user-service/internal/service/token_service_test.go
git commit -m "feat(user-service): add JWT token service with iss claim and opaque refresh tokens"
```

---

### Task 4: Repositories (user, refresh_token, login_log, role)

**Files:**
- Create: `user-service/internal/repository/user_repo.go`
- Create: `user-service/internal/repository/refresh_token_repo.go`
- Create: `user-service/internal/repository/login_log_repo.go`
- Create: `user-service/internal/repository/role_repo.go`
- Test: `user-service/internal/repository/user_repo_test.go` (uses `sqlite` in-memory via GORM for repo tests)

**Interfaces:**
- Consumes: `model.User/RefreshToken/LoginLog/Role` (Task 2)
- Produces: `repository.NewUserRepo(db *gorm.DB) *UserRepo` with methods `Create, FindByEmail, FindByUsername, FindByID, List(filter UserFilter) ([]model.User, int64, error), Update, SoftDelete`
- Produces: `repository.NewRefreshTokenRepo(db *gorm.DB) *RefreshTokenRepo` with methods `Create, FindByHash, RevokeByHash, RevokeAllForUser, ListActiveForUser`
- Produces: `repository.NewLoginLogRepo(db *gorm.DB) *LoginLogRepo` with methods `Create, ListForUser(userID uuid.UUID) ([]model.LoginLog, int64, error)`
- Produces: `repository.NewRoleRepo(db *gorm.DB) *RoleRepo` with methods `FindByName, List`

- [ ] **Step 1: Add test dependency and write failing test**

```bash
go get github.com/glebarez/sqlite
```

```go
// user-service/internal/repository/user_repo_test.go
package repository_test

import (
	"testing"

	"github.com/glebarez/sqlite" // pure-Go sqlite driver, no CGO needed
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/user-service/internal/model"
	"github.com/equipment-rental-system/user-service/internal/repository"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Role{}, &model.User{}, &model.RefreshToken{}, &model.LoginLog{}))
	require.NoError(t, db.Create(&model.Role{ID: 3, Name: "customer"}).Error)
	return db
}

func TestUserRepo_CreateAndFindByEmail(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewUserRepo(db)

	u := &model.User{Email: "a@example.com", Username: "a", PasswordHash: "hash", RoleID: 3}
	require.NoError(t, repo.Create(u))

	found, err := repo.FindByEmail("a@example.com")
	require.NoError(t, err)
	require.Equal(t, u.ID, found.ID)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/repository/... -v`
Expected: FAIL — `repository.NewUserRepo` undefined

- [ ] **Step 3: Implement repositories**

```go
// user-service/internal/repository/user_repo.go
package repository

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/user-service/internal/model"
)

type UserRepo struct{ db *gorm.DB }

func NewUserRepo(db *gorm.DB) *UserRepo { return &UserRepo{db: db} }

type UserFilter struct {
	Query    string
	Role     string
	IsActive *bool
	Page, Limit int
	Sort, Order string
}

func (r *UserRepo) Create(u *model.User) error {
	return r.db.Create(u).Error
}

func (r *UserRepo) FindByEmail(email string) (*model.User, error) {
	var u model.User
	if err := r.db.Preload("Role").Where("email = ?", email).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepo) FindByUsername(username string) (*model.User, error) {
	var u model.User
	if err := r.db.Preload("Role").Where("username = ?", username).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepo) FindByID(id uuid.UUID) (*model.User, error) {
	var u model.User
	if err := r.db.Preload("Role").First(&u, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepo) List(f UserFilter) ([]model.User, int64, error) {
	q := r.db.Model(&model.User{}).Preload("Role")
	if f.Query != "" {
		q = q.Where("email ILIKE ? OR username ILIKE ? OR full_name ILIKE ?", "%"+f.Query+"%", "%"+f.Query+"%", "%"+f.Query+"%")
	}
	if f.Role != "" {
		q = q.Joins("JOIN roles ON roles.id = users.role_id").Where("roles.name = ?", f.Role)
	}
	if f.IsActive != nil {
		q = q.Where("is_active = ?", *f.IsActive)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	sort, order := f.Sort, f.Order
	if sort == "" {
		sort = "created_at"
	}
	if order == "" {
		order = "desc"
	}
	page, limit := f.Page, f.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	var users []model.User
	err := q.Order(sort + " " + order).Offset((page - 1) * limit).Limit(limit).Find(&users).Error
	return users, total, err
}

func (r *UserRepo) Update(u *model.User) error {
	return r.db.Save(u).Error
}

func (r *UserRepo) SoftDelete(id uuid.UUID) error {
	return r.db.Delete(&model.User{}, "id = ?", id).Error
}
```

```go
// user-service/internal/repository/refresh_token_repo.go
package repository

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/user-service/internal/model"
)

type RefreshTokenRepo struct{ db *gorm.DB }

func NewRefreshTokenRepo(db *gorm.DB) *RefreshTokenRepo { return &RefreshTokenRepo{db: db} }

func (r *RefreshTokenRepo) Create(t *model.RefreshToken) error {
	return r.db.Create(t).Error
}

func (r *RefreshTokenRepo) FindByHash(hash string) (*model.RefreshToken, error) {
	var t model.RefreshToken
	if err := r.db.Where("token_hash = ?", hash).First(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *RefreshTokenRepo) RevokeByHash(hash string) error {
	return r.db.Model(&model.RefreshToken{}).Where("token_hash = ?", hash).
		Update("revoked_at", time.Now()).Error
}

func (r *RefreshTokenRepo) RevokeAllForUser(userID uuid.UUID) error {
	return r.db.Model(&model.RefreshToken{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Update("revoked_at", time.Now()).Error
}

func (r *RefreshTokenRepo) ListActiveForUser(userID uuid.UUID) ([]model.RefreshToken, error) {
	var tokens []model.RefreshToken
	err := r.db.Where("user_id = ? AND revoked_at IS NULL AND expires_at > ?", userID, time.Now()).
		Find(&tokens).Error
	return tokens, err
}
```

```go
// user-service/internal/repository/login_log_repo.go
package repository

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/user-service/internal/model"
)

type LoginLogRepo struct{ db *gorm.DB }

func NewLoginLogRepo(db *gorm.DB) *LoginLogRepo { return &LoginLogRepo{db: db} }

func (r *LoginLogRepo) Create(l *model.LoginLog) error {
	return r.db.Create(l).Error
}

func (r *LoginLogRepo) ListForUser(userID uuid.UUID, page, limit int) ([]model.LoginLog, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	q := r.db.Model(&model.LoginLog{}).Where("user_id = ?", userID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var logs []model.LoginLog
	err := q.Order("created_at desc").Offset((page - 1) * limit).Limit(limit).Find(&logs).Error
	return logs, total, err
}
```

```go
// user-service/internal/repository/role_repo.go
package repository

import (
	"gorm.io/gorm"

	"github.com/equipment-rental-system/user-service/internal/model"
)

type RoleRepo struct{ db *gorm.DB }

func NewRoleRepo(db *gorm.DB) *RoleRepo { return &RoleRepo{db: db} }

func (r *RoleRepo) FindByName(name string) (*model.Role, error) {
	var role model.Role
	if err := r.db.Where("name = ?", name).First(&role).Error; err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *RoleRepo) List() ([]model.Role, error) {
	var roles []model.Role
	err := r.db.Order("id").Find(&roles).Error
	return roles, err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/repository/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add user-service/internal/repository/ user-service/go.mod user-service/go.sum
git commit -m "feat(user-service): add repositories for user, refresh_token, login_log, role"
```

---

### Task 5: Auth service + handler — register

**Files:**
- Create: `user-service/internal/dto/auth_dto.go`
- Create: `user-service/internal/service/auth_service.go`
- Create: `user-service/internal/handler/auth_handler.go`
- Modify: `user-service/internal/router/router.go` — add `POST /api/v1/auth/register`
- Test: `user-service/internal/service/auth_service_test.go`

**Interfaces:**
- Consumes: `repository.UserRepo/RoleRepo` (Task 4), `bcrypt` for hashing
- Produces: `dto.RegisterRequest{Email, Username, Password, FullName, Phone string}` (validator tags: `email`, `required,min=3,max=50`, `required,min=8`)
- Produces: `service.NewAuthService(users *repository.UserRepo, roles *repository.RoleRepo, refreshTokens *repository.RefreshTokenRepo, logs *repository.LoginLogRepo, tokens *service.TokenService, cfg *config.Config) *AuthService`
- Produces: `(*AuthService) Register(req dto.RegisterRequest) (*model.User, error)` — returns `ErrEmailExists`, `ErrUsernameExists`, `ErrWeakPassword` sentinel errors
- Produces: `handler.NewAuthHandler(svc *service.AuthService) *AuthHandler` with `(*AuthHandler) Register(c *gin.Context)`

- [ ] **Step 1: Write failing test — duplicate email rejected**

```go
// user-service/internal/service/auth_service_test.go
package service_test

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/user-service/internal/config"
	"github.com/equipment-rental-system/user-service/internal/dto"
	"github.com/equipment-rental-system/user-service/internal/model"
	"github.com/equipment-rental-system/user-service/internal/repository"
	"github.com/equipment-rental-system/user-service/internal/service"
)

func setupAuthService(t *testing.T) *service.AuthService {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Role{}, &model.User{}, &model.RefreshToken{}, &model.LoginLog{}))
	require.NoError(t, db.Create(&model.Role{ID: 3, Name: "customer"}).Error)

	cfg := &config.Config{BCryptCost: 4, JWTSecret: "test-secret-min-32-characters-ok", JWTAccessTTL: 15 * time.Minute, JWTRefreshTTL: 168 * time.Hour}
	return service.NewAuthService(
		repository.NewUserRepo(db), repository.NewRoleRepo(db),
		repository.NewRefreshTokenRepo(db), repository.NewLoginLogRepo(db),
		service.NewTokenService(cfg.JWTSecret, cfg.JWTAccessTTL), cfg,
	)
}

func TestAuthService_Register_DuplicateEmail_ReturnsErrEmailExists(t *testing.T) {
	svc := setupAuthService(t)
	req := dto.RegisterRequest{Email: "a@example.com", Username: "user1", Password: "Passw0rd1", FullName: "A"}

	_, err := svc.Register(req)
	require.NoError(t, err)

	req.Username = "user2"
	_, err = svc.Register(req)
	require.ErrorIs(t, err, service.ErrEmailExists)
}

func TestAuthService_Register_AssignsCustomerRole(t *testing.T) {
	svc := setupAuthService(t)
	u, err := svc.Register(dto.RegisterRequest{Email: "b@example.com", Username: "userb", Password: "Passw0rd1"})
	require.NoError(t, err)
	require.Equal(t, int16(3), u.RoleID)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service/... -run TestAuthService_Register -v`
Expected: FAIL — `service.NewAuthService` undefined

- [ ] **Step 3: Implement DTO + auth service (register) + handler + route**

```go
// user-service/internal/dto/auth_dto.go
package dto

type RegisterRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Username string `json:"username" binding:"required,min=3,max=50"`
	Password string `json:"password" binding:"required,min=8"`
	FullName string `json:"full_name"`
	Phone    string `json:"phone"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type LogoutRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}
```

```go
// user-service/internal/service/auth_service.go
package service

import (
	"errors"
	"regexp"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/user-service/internal/config"
	"github.com/equipment-rental-system/user-service/internal/dto"
	"github.com/equipment-rental-system/user-service/internal/model"
	"github.com/equipment-rental-system/user-service/internal/repository"
)

var (
	ErrEmailExists      = errors.New("email already exists")
	ErrUsernameExists   = errors.New("username already exists")
	ErrWeakPassword     = errors.New("password does not meet strength requirements")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrAccountDisabled  = errors.New("account disabled")
)

var passwordHasDigitAndLetter = regexp.MustCompile(`^.*[A-Za-z].*$`)
var passwordHasDigit = regexp.MustCompile(`^.*[0-9].*$`)

type AuthService struct {
	users         *repository.UserRepo
	roles         *repository.RoleRepo
	refreshTokens *repository.RefreshTokenRepo
	logs          *repository.LoginLogRepo
	tokens        *TokenService
	cfg           *config.Config
}

func NewAuthService(users *repository.UserRepo, roles *repository.RoleRepo,
	refreshTokens *repository.RefreshTokenRepo, logs *repository.LoginLogRepo,
	tokens *TokenService, cfg *config.Config) *AuthService {
	return &AuthService{users: users, roles: roles, refreshTokens: refreshTokens, logs: logs, tokens: tokens, cfg: cfg}
}

func isStrongPassword(pw string) bool {
	return len(pw) >= 8 && passwordHasDigitAndLetter.MatchString(pw) && passwordHasDigit.MatchString(pw)
}

func (s *AuthService) Register(req dto.RegisterRequest) (*model.User, error) {
	if !isStrongPassword(req.Password) {
		return nil, ErrWeakPassword
	}
	if _, err := s.users.FindByEmail(req.Email); err == nil {
		return nil, ErrEmailExists
	}
	if _, err := s.users.FindByUsername(req.Username); err == nil {
		return nil, ErrUsernameExists
	}
	role, err := s.roles.FindByName("customer")
	if err != nil {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), s.cfg.BCryptCost)
	if err != nil {
		return nil, err
	}
	u := &model.User{
		Email: req.Email, Username: req.Username, PasswordHash: string(hash),
		FullName: req.FullName, Phone: req.Phone, RoleID: role.ID, IsActive: true,
	}
	if err := s.users.Create(u); err != nil {
		return nil, err
	}
	u.Role = *role
	return u, nil
}

var errNotFound = gorm.ErrRecordNotFound
```

```go
// user-service/internal/handler/auth_handler.go
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
```

```go
// user-service/internal/router/router.go (modify: add v1 group + register route)
package router

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/user-service/internal/config"
	"github.com/equipment-rental-system/user-service/internal/handler"
	"github.com/equipment-rental-system/user-service/internal/repository"
	"github.com/equipment-rental-system/user-service/internal/service"
)

func New(database *gorm.DB, cfg *config.Config) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/health", handler.Health(database))

	userRepo := repository.NewUserRepo(database)
	roleRepo := repository.NewRoleRepo(database)
	refreshRepo := repository.NewRefreshTokenRepo(database)
	logRepo := repository.NewLoginLogRepo(database)
	tokenSvc := service.NewTokenService(cfg.JWTSecret, cfg.JWTAccessTTL)
	authSvc := service.NewAuthService(userRepo, roleRepo, refreshRepo, logRepo, tokenSvc, cfg)
	authHandler := handler.NewAuthHandler(authSvc)

	v1 := r.Group("/api/v1")
	v1.POST("/auth/register", authHandler.Register)
	return r
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/service/... -run TestAuthService_Register -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add user-service/internal/dto/ user-service/internal/service/auth_service.go \
  user-service/internal/handler/auth_handler.go user-service/internal/router/router.go \
  user-service/internal/service/auth_service_test.go
git commit -m "feat(user-service): implement POST /auth/register"
```

---

### Task 6: Auth service + handler — login (with login_logs)

**Files:**
- Modify: `user-service/internal/service/auth_service.go` — add `Login`
- Modify: `user-service/internal/handler/auth_handler.go` — add `Login`
- Modify: `user-service/internal/router/router.go` — add `POST /api/v1/auth/login`
- Test: `user-service/internal/service/auth_service_test.go` — add login tests

**Interfaces:**
- Consumes: `AuthService` fields from Task 5, `TokenService.GenerateAccessToken` (Task 3), `NewOpaqueRefreshToken/HashRefreshToken` (Task 3)
- Produces: `(*AuthService) Login(req dto.LoginRequest, ip, userAgent string) (accessToken, refreshToken string, expiresIn int, user *model.User, err error)` — returns `ErrInvalidCredentials`, `ErrAccountDisabled`

- [ ] **Step 1: Write failing tests — wrong password rejected + log written, disabled account rejected**

```go
// append to user-service/internal/service/auth_service_test.go
func TestAuthService_Login_WrongPassword_ReturnsErrInvalidCredentials(t *testing.T) {
	svc := setupAuthService(t)
	_, err := svc.Register(dto.RegisterRequest{Email: "c@example.com", Username: "userc", Password: "Passw0rd1"})
	require.NoError(t, err)

	_, _, _, _, err = svc.Login(dto.LoginRequest{Email: "c@example.com", Password: "WrongPass1"}, "1.2.3.4", "test-agent")
	require.ErrorIs(t, err, service.ErrInvalidCredentials)
}

func TestAuthService_Login_Success_ReturnsTokens(t *testing.T) {
	svc := setupAuthService(t)
	_, err := svc.Register(dto.RegisterRequest{Email: "d@example.com", Username: "userd", Password: "Passw0rd1"})
	require.NoError(t, err)

	access, refresh, expiresIn, user, err := svc.Login(dto.LoginRequest{Email: "d@example.com", Password: "Passw0rd1"}, "1.2.3.4", "test-agent")
	require.NoError(t, err)
	require.NotEmpty(t, access)
	require.Len(t, refresh, 64)
	require.Equal(t, 900, expiresIn)
	require.Equal(t, "userd", user.Username)
}

func TestAuthService_Login_DisabledAccount_ReturnsErrAccountDisabled(t *testing.T) {
	svc := setupAuthService(t)
	u, err := svc.Register(dto.RegisterRequest{Email: "e@example.com", Username: "usere", Password: "Passw0rd1"})
	require.NoError(t, err)
	u.IsActive = false
	require.NoError(t, svc.UsersRepoForTest().Update(u))

	_, _, _, _, err = svc.Login(dto.LoginRequest{Email: "e@example.com", Password: "Passw0rd1"}, "1.2.3.4", "test-agent")
	require.ErrorIs(t, err, service.ErrAccountDisabled)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service/... -run TestAuthService_Login -v`
Expected: FAIL — `svc.Login` and `UsersRepoForTest` undefined

- [ ] **Step 3: Implement `Login` (and a test-only repo accessor)**

```go
// append to user-service/internal/service/auth_service.go
func (s *AuthService) UsersRepoForTest() *repository.UserRepo { return s.users }

func (s *AuthService) Login(req dto.LoginRequest, ip, userAgent string) (string, string, int, *model.User, error) {
	u, err := s.users.FindByEmail(req.Email)
	logSuccess := false
	defer func() {
		var userID *interface{}
		_ = userID
		logEntry := &model.LoginLog{EmailAttempted: req.Email, Success: logSuccess, IPAddress: ip, UserAgent: userAgent}
		if u != nil {
			id := u.ID
			logEntry.UserID = &id
		}
		_ = s.logs.Create(logEntry)
	}()

	if err != nil {
		return "", "", 0, nil, ErrInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)) != nil {
		return "", "", 0, nil, ErrInvalidCredentials
	}
	if !u.IsActive {
		return "", "", 0, nil, ErrAccountDisabled
	}

	access, expiresIn, err := s.tokens.GenerateAccessToken(*u)
	if err != nil {
		return "", "", 0, nil, err
	}
	rawRefresh, err := NewOpaqueRefreshToken()
	if err != nil {
		return "", "", 0, nil, err
	}
	if err := s.refreshTokens.Create(&model.RefreshToken{
		UserID: u.ID, TokenHash: HashRefreshToken(rawRefresh),
		ExpiresAt: time.Now().Add(s.cfg.JWTRefreshTTL), IPAddress: ip, UserAgent: userAgent,
	}); err != nil {
		return "", "", 0, nil, err
	}
	logSuccess = true
	return access, rawRefresh, expiresIn, u, nil
}
```

Add `Login` handler:

```go
// append to user-service/internal/handler/auth_handler.go
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
```

```go
// user-service/internal/router/router.go — add inside v1 group
v1.POST("/auth/login", authHandler.Login)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/service/... -run TestAuthService_Login -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add user-service/internal/
git commit -m "feat(user-service): implement POST /auth/login with login_logs audit trail"
```

---

### Task 7: Auth service + handler — refresh (rotation) and logout

**Files:**
- Modify: `user-service/internal/service/auth_service.go` — add `Refresh`, `Logout`
- Modify: `user-service/internal/handler/auth_handler.go` — add `Refresh`, `Logout`
- Modify: `user-service/internal/router/router.go` — add routes (Logout needs auth middleware — added in Task 8, wire route there)
- Test: append to `user-service/internal/service/auth_service_test.go`

**Interfaces:**
- Consumes: `RefreshTokenRepo` (Task 4), `TokenService` (Task 3)
- Produces: `(*AuthService) Refresh(rawToken string) (accessToken, newRefreshToken string, expiresIn int, err error)` — returns `ErrInvalidRefreshToken`
- Produces: `(*AuthService) Logout(rawToken string) error`

- [ ] **Step 1: Write failing tests — rotation invalidates old token, revoked token rejected**

```go
// append to user-service/internal/service/auth_service_test.go
func TestAuthService_Refresh_RotatesToken(t *testing.T) {
	svc := setupAuthService(t)
	_, err := svc.Register(dto.RegisterRequest{Email: "f@example.com", Username: "userf", Password: "Passw0rd1"})
	require.NoError(t, err)
	_, refresh, _, _, err := svc.Login(dto.LoginRequest{Email: "f@example.com", Password: "Passw0rd1"}, "ip", "ua")
	require.NoError(t, err)

	_, newRefresh, expiresIn, err := svc.Refresh(refresh)
	require.NoError(t, err)
	require.NotEqual(t, refresh, newRefresh)
	require.Equal(t, 900, expiresIn)

	// old token must now be rejected
	_, _, _, err = svc.Refresh(refresh)
	require.ErrorIs(t, err, service.ErrInvalidRefreshToken)
}

func TestAuthService_Logout_RevokesToken(t *testing.T) {
	svc := setupAuthService(t)
	_, err := svc.Register(dto.RegisterRequest{Email: "g@example.com", Username: "userg", Password: "Passw0rd1"})
	require.NoError(t, err)
	_, refresh, _, _, err := svc.Login(dto.LoginRequest{Email: "g@example.com", Password: "Passw0rd1"}, "ip", "ua")
	require.NoError(t, err)

	require.NoError(t, svc.Logout(refresh))
	_, _, _, err = svc.Refresh(refresh)
	require.ErrorIs(t, err, service.ErrInvalidRefreshToken)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service/... -run "TestAuthService_Refresh|TestAuthService_Logout" -v`
Expected: FAIL — `svc.Refresh`/`svc.Logout` undefined

- [ ] **Step 3: Implement `Refresh` and `Logout`**

```go
// append to user-service/internal/service/auth_service.go
var ErrInvalidRefreshToken = errors.New("invalid or expired refresh token")

func (s *AuthService) Refresh(rawToken string) (string, string, int, error) {
	hash := HashRefreshToken(rawToken)
	rt, err := s.refreshTokens.FindByHash(hash)
	if err != nil || rt.RevokedAt != nil || rt.ExpiresAt.Before(time.Now()) {
		return "", "", 0, ErrInvalidRefreshToken
	}
	u, err := s.users.FindByID(rt.UserID)
	if err != nil {
		return "", "", 0, ErrInvalidRefreshToken
	}
	if !u.IsActive {
		return "", "", 0, ErrAccountDisabled
	}
	if err := s.refreshTokens.RevokeByHash(hash); err != nil {
		return "", "", 0, err
	}
	access, expiresIn, err := s.tokens.GenerateAccessToken(*u)
	if err != nil {
		return "", "", 0, err
	}
	newRaw, err := NewOpaqueRefreshToken()
	if err != nil {
		return "", "", 0, err
	}
	if err := s.refreshTokens.Create(&model.RefreshToken{
		UserID: u.ID, TokenHash: HashRefreshToken(newRaw), ExpiresAt: time.Now().Add(s.cfg.JWTRefreshTTL),
	}); err != nil {
		return "", "", 0, err
	}
	return access, newRaw, expiresIn, nil
}

func (s *AuthService) Logout(rawToken string) error {
	return s.refreshTokens.RevokeByHash(HashRefreshToken(rawToken))
}
```

Add handlers:

```go
// append to user-service/internal/handler/auth_handler.go
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req dto.RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"code": "VALIDATION_ERROR", "message": err.Error(), "details": nil}})
		return
	}
	access, refresh, expiresIn, err := h.svc.Refresh(req.RefreshToken)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidRefreshToken):
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": gin.H{"code": "INVALID_REFRESH_TOKEN", "message": "refresh token ไม่ถูกต้องหรือหมดอายุ", "details": nil}})
		case errors.Is(err, service.ErrAccountDisabled):
			c.JSON(http.StatusForbidden, gin.H{"success": false, "error": gin.H{"code": "ACCOUNT_DISABLED", "message": "บัญชีถูกปิดการใช้งาน", "details": nil}})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"code": "INTERNAL_ERROR", "message": err.Error(), "details": nil}})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"access_token": access, "refresh_token": refresh, "token_type": "Bearer", "expires_in": expiresIn,
	}})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	var req dto.LogoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"code": "VALIDATION_ERROR", "message": err.Error(), "details": nil}})
		return
	}
	if err := h.svc.Logout(req.RefreshToken); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"code": "INTERNAL_ERROR", "message": err.Error(), "details": nil}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"message": "ออกจากระบบเรียบร้อย"}})
}
```

```go
// user-service/internal/router/router.go — add inside v1 group
v1.POST("/auth/refresh", authHandler.Refresh)
// Logout requires auth middleware — route added in Task 8 once middleware exists
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/service/... -v`
Expected: PASS (all auth_service tests)

- [ ] **Step 5: Commit**

```bash
git add user-service/internal/
git commit -m "feat(user-service): implement POST /auth/refresh (rotation) and logout"
```

---

### Task 8: Auth middleware (RequireAuth) + wire logout route

**Files:**
- Create: `user-service/internal/middleware/auth.go`
- Modify: `user-service/internal/router/router.go` — add `RequireAuth`, wire `POST /api/v1/auth/logout`
- Test: `user-service/internal/middleware/auth_test.go`

**Interfaces:**
- Consumes: `service.TokenService.ParseAccessToken` (Task 3)
- Produces: `middleware.RequireAuth(ts *service.TokenService) gin.HandlerFunc` — sets `c.Set("user_id", claims.Sub)` and `c.Set("role", claims.Role)`; missing/invalid/expired token → `401 UNAUTHENTICATED`

- [ ] **Step 1: Write failing test — missing token rejected, valid token passes claims through**

```go
// user-service/internal/middleware/auth_test.go
package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/equipment-rental-system/user-service/internal/middleware"
	"github.com/equipment-rental-system/user-service/internal/model"
	"github.com/equipment-rental-system/user-service/internal/service"
)

func TestRequireAuth_NoToken_Returns401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ts := service.NewTokenService("test-secret-min-32-characters-ok", 15*time.Minute)
	r := gin.New()
	r.GET("/protected", middleware.RequireAuth(ts), func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireAuth_ValidToken_SetsUserIDAndRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ts := service.NewTokenService("test-secret-min-32-characters-ok", 15*time.Minute)
	u := model.User{ID: uuid.New(), Role: model.Role{Name: "staff"}}
	token, _, err := ts.GenerateAccessToken(u)
	require.NoError(t, err)

	var gotRole any
	r := gin.New()
	r.GET("/protected", middleware.RequireAuth(ts), func(c *gin.Context) {
		gotRole, _ = c.Get("role")
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "staff", gotRole)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/middleware/... -v`
Expected: FAIL — `middleware.RequireAuth` undefined

- [ ] **Step 3: Implement middleware and wire logout route**

```go
// user-service/internal/middleware/auth.go
package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/equipment-rental-system/user-service/internal/service"
)

func RequireAuth(ts *service.TokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			unauthenticated(c)
			return
		}
		token := strings.TrimPrefix(header, "Bearer ")
		claims, err := ts.ParseAccessToken(token)
		if err != nil {
			unauthenticated(c)
			return
		}
		c.Set("user_id", claims.Sub)
		c.Set("role", claims.Role)
		c.Next()
	}
}

func unauthenticated(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"success": false,
		"error":   gin.H{"code": "UNAUTHENTICATED", "message": "ไม่มี token หรือ token ไม่ถูกต้อง", "details": nil},
	})
}
```

```go
// user-service/internal/router/router.go — add near top of New(), after tokenSvc is created
authMW := middleware.RequireAuth(tokenSvc)
// add inside v1 group
v1.POST("/auth/logout", authMW, authHandler.Logout)
```

(add `"github.com/equipment-rental-system/user-service/internal/middleware"` to the import block)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/middleware/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add user-service/internal/middleware/ user-service/internal/router/router.go
git commit -m "feat(user-service): add RequireAuth middleware, wire POST /auth/logout"
```

---

## Sprint 2 — Self-service profile

### Task 9: GET/PUT /me

**Files:**
- Create: `user-service/internal/dto/user_dto.go`
- Create: `user-service/internal/service/user_service.go`
- Create: `user-service/internal/handler/user_handler.go`
- Modify: `user-service/internal/router/router.go` — add `GET /api/v1/me`, `PUT /api/v1/me`
- Test: `user-service/internal/service/user_service_test.go`

**Interfaces:**
- Consumes: `repository.UserRepo` (Task 4)
- Produces: `dto.UpdateProfileRequest{FullName, Phone *string}`
- Produces: `service.NewUserService(users *repository.UserRepo) *UserService`
- Produces: `(*UserService) GetProfile(userID uuid.UUID) (*model.User, error)`
- Produces: `(*UserService) UpdateProfile(userID uuid.UUID, req dto.UpdateProfileRequest) (*model.User, error)`
- Produces: `handler.NewUserHandler(svc *service.UserService) *UserHandler` with `Me`, `UpdateMe`

- [ ] **Step 1: Write failing test — update only touches full_name/phone**

```go
// user-service/internal/service/user_service_test.go
package service_test

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/user-service/internal/dto"
	"github.com/equipment-rental-system/user-service/internal/model"
	"github.com/equipment-rental-system/user-service/internal/repository"
	"github.com/equipment-rental-system/user-service/internal/service"
)

func setupUserService(t *testing.T) (*service.UserService, *model.User) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Role{}, &model.User{}))
	require.NoError(t, db.Create(&model.Role{ID: 3, Name: "customer"}).Error)
	u := &model.User{Email: "p@example.com", Username: "p", PasswordHash: "x", RoleID: 3, FullName: "Old Name"}
	require.NoError(t, db.Create(u).Error)
	return service.NewUserService(repository.NewUserRepo(db)), u
}

func TestUserService_UpdateProfile_ChangesFullNameAndPhone(t *testing.T) {
	svc, u := setupUserService(t)
	newName := "New Name"
	updated, err := svc.UpdateProfile(u.ID, dto.UpdateProfileRequest{FullName: &newName})
	require.NoError(t, err)
	require.Equal(t, "New Name", updated.FullName)
	require.Equal(t, u.Email, updated.Email) // email untouched
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service/... -run TestUserService_UpdateProfile -v`
Expected: FAIL — `service.NewUserService` undefined

- [ ] **Step 3: Implement**

```go
// append to user-service/internal/dto/user_dto.go (create file)
package dto

type UpdateProfileRequest struct {
	FullName *string `json:"full_name"`
	Phone    *string `json:"phone"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required,min=8"`
}

type CreateUserRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Username string `json:"username" binding:"required,min=3,max=50"`
	Password string `json:"password" binding:"required,min=8"`
	FullName string `json:"full_name"`
	Phone    string `json:"phone"`
	Role     string `json:"role" binding:"required,oneof=admin staff customer"`
	IsActive *bool  `json:"is_active"`
}

type UpdateUserRequest struct {
	Email    *string `json:"email"`
	Username *string `json:"username"`
	FullName *string `json:"full_name"`
	Phone    *string `json:"phone"`
}

type ChangeRoleRequest struct {
	Role string `json:"role" binding:"required,oneof=admin staff customer"`
}

type ChangeStatusRequest struct {
	IsActive bool `json:"is_active"`
}
```

```go
// user-service/internal/service/user_service.go
package service

import (
	"github.com/google/uuid"

	"github.com/equipment-rental-system/user-service/internal/dto"
	"github.com/equipment-rental-system/user-service/internal/model"
	"github.com/equipment-rental-system/user-service/internal/repository"
)

type UserService struct {
	users *repository.UserRepo
}

func NewUserService(users *repository.UserRepo) *UserService {
	return &UserService{users: users}
}

func (s *UserService) GetProfile(userID uuid.UUID) (*model.User, error) {
	return s.users.FindByID(userID)
}

func (s *UserService) UpdateProfile(userID uuid.UUID, req dto.UpdateProfileRequest) (*model.User, error) {
	u, err := s.users.FindByID(userID)
	if err != nil {
		return nil, err
	}
	if req.FullName != nil {
		u.FullName = *req.FullName
	}
	if req.Phone != nil {
		u.Phone = *req.Phone
	}
	if err := s.users.Update(u); err != nil {
		return nil, err
	}
	return u, nil
}
```

```go
// user-service/internal/handler/user_handler.go
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/equipment-rental-system/user-service/internal/dto"
	"github.com/equipment-rental-system/user-service/internal/service"
)

type UserHandler struct {
	svc *service.UserService
}

func NewUserHandler(svc *service.UserService) *UserHandler {
	return &UserHandler{svc: svc}
}

func currentUserID(c *gin.Context) (uuid.UUID, bool) {
	raw, ok := c.Get("user_id")
	if !ok {
		return uuid.UUID{}, false
	}
	id, err := uuid.Parse(raw.(string))
	return id, err == nil
}

func (h *UserHandler) Me(c *gin.Context) {
	id, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": gin.H{"code": "UNAUTHENTICATED", "message": "unauthenticated", "details": nil}})
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

func (h *UserHandler) UpdateMe(c *gin.Context) {
	id, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": gin.H{"code": "UNAUTHENTICATED", "message": "unauthenticated", "details": nil}})
		return
	}
	var req dto.UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"code": "VALIDATION_ERROR", "message": err.Error(), "details": nil}})
		return
	}
	u, err := h.svc.UpdateProfile(id, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"code": "INTERNAL_ERROR", "message": err.Error(), "details": nil}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"id": u.ID, "full_name": u.FullName, "phone": u.Phone, "updated_at": u.UpdatedAt,
	}})
}
```

```go
// user-service/internal/router/router.go — add
userSvc := service.NewUserService(userRepo)
userHandler := handler.NewUserHandler(userSvc)
v1.GET("/me", authMW, userHandler.Me)
v1.PUT("/me", authMW, userHandler.UpdateMe)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/service/... -run TestUserService_UpdateProfile -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add user-service/internal/
git commit -m "feat(user-service): implement GET/PUT /me"
```

---

### Task 10: PUT /me/password

**Files:**
- Modify: `user-service/internal/service/user_service.go` — add `ChangePassword`
- Modify: `user-service/internal/handler/user_handler.go` — add `ChangePassword`
- Modify: `user-service/internal/router/router.go` — add route
- Test: append to `user-service/internal/service/user_service_test.go`

**Interfaces:**
- Consumes: `RefreshTokenRepo.RevokeAllForUser` (Task 4), `bcrypt`
- Produces: `(*UserService) ChangePassword(userID uuid.UUID, req dto.ChangePasswordRequest, cost int) error` — returns `ErrInvalidCredentials` on wrong current password

- [ ] **Step 1: Write failing test — wrong current password rejected, correct one revokes all sessions**

```go
// append to user-service/internal/service/user_service_test.go
func TestUserService_ChangePassword_WrongCurrent_ReturnsError(t *testing.T) {
	svc, u := setupUserServiceWithPassword(t, "Passw0rd1")
	err := svc.ChangePassword(u.ID, dto.ChangePasswordRequest{CurrentPassword: "WrongOne1", NewPassword: "N3wPassw0rd"}, 4)
	require.ErrorIs(t, err, service.ErrInvalidCredentials)
}

func TestUserService_ChangePassword_Correct_RevokesAllSessions(t *testing.T) {
	svc, u := setupUserServiceWithPassword(t, "Passw0rd1")
	err := svc.ChangePassword(u.ID, dto.ChangePasswordRequest{CurrentPassword: "Passw0rd1", NewPassword: "N3wPassw0rd"}, 4)
	require.NoError(t, err)
}
```

Add a variant setup helper that also wires `RefreshTokenRepo` and hashes a known password:

```go
// append to user-service/internal/service/user_service_test.go
func setupUserServiceWithPassword(t *testing.T, plain string) (*service.UserService, *model.User) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Role{}, &model.User{}, &model.RefreshToken{}))
	require.NoError(t, db.Create(&model.Role{ID: 3, Name: "customer"}).Error)
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), 4)
	require.NoError(t, err)
	u := &model.User{Email: "q@example.com", Username: "q", PasswordHash: string(hash), RoleID: 3}
	require.NoError(t, db.Create(u).Error)
	return service.NewUserService(repository.NewUserRepo(db), repository.NewRefreshTokenRepo(db)), u
}
```

(imports to add in that test file: `"golang.org/x/crypto/bcrypt"`)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service/... -run TestUserService_ChangePassword -v`
Expected: FAIL — `ChangePassword` undefined, and `NewUserService` has the wrong arity (still 1-arg from Task 9)

- [ ] **Step 3: Implement — extend `UserService` to also hold `RefreshTokenRepo`**

Change the constructor signature from Task 9's single-arg form to accept both repos:

```go
// user-service/internal/service/user_service.go — replace struct + constructor
type UserService struct {
	users         *repository.UserRepo
	refreshTokens *repository.RefreshTokenRepo
}

func NewUserService(users *repository.UserRepo, refreshTokens *repository.RefreshTokenRepo) *UserService {
	return &UserService{users: users, refreshTokens: refreshTokens}
}
```

Update every existing call site to pass the second argument: Task 9's `setupUserService` test helper (build a `RefreshTokenRepo` on the same in-memory db) and `router.go`'s `service.NewUserService(userRepo, refreshRepo)`.

Add `ChangePassword`:

```go
// append to user-service/internal/service/user_service.go
import "golang.org/x/crypto/bcrypt" // add to import block

func (s *UserService) ChangePassword(userID uuid.UUID, req dto.ChangePasswordRequest, cost int) error {
	u, err := s.users.FindByID(userID)
	if err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.CurrentPassword)) != nil {
		return ErrInvalidCredentials
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), cost)
	if err != nil {
		return err
	}
	u.PasswordHash = string(hash)
	if err := s.users.Update(u); err != nil {
		return err
	}
	return s.refreshTokens.RevokeAllForUser(userID)
}
```

Add handler + route:

```go
// append to user-service/internal/handler/user_handler.go
func (h *UserHandler) ChangePassword(c *gin.Context) {
	id, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": gin.H{"code": "UNAUTHENTICATED", "message": "unauthenticated", "details": nil}})
		return
	}
	var req dto.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"code": "VALIDATION_ERROR", "message": err.Error(), "details": nil}})
		return
	}
	if err := h.svc.ChangePassword(id, req, 12); err != nil {
		if err == service.ErrInvalidCredentials {
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": gin.H{"code": "INVALID_CREDENTIALS", "message": "รหัสผ่านเดิมไม่ถูกต้อง", "details": nil}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"code": "INTERNAL_ERROR", "message": err.Error(), "details": nil}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"message": "เปลี่ยนรหัสผ่านเรียบร้อย กรุณาเข้าสู่ระบบใหม่"}})
}
```

```go
// user-service/internal/router/router.go — add
v1.PUT("/me/password", authMW, userHandler.ChangePassword)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/service/... -run TestUserService_ChangePassword -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add user-service/internal/
git commit -m "feat(user-service): implement PUT /me/password, revoke sessions on change"
```

---

### Task 11: GET /me/login-logs and GET /me/sessions

**Files:**
- Modify: `user-service/internal/handler/user_handler.go` — add `MyLoginLogs`, `MySessions`
- Modify: `user-service/internal/router/router.go` — add routes
- Test: `user-service/internal/handler/user_handler_test.go`

**Interfaces:**
- Consumes: `LoginLogRepo.ListForUser`, `RefreshTokenRepo.ListActiveForUser` (Task 4)
- Produces: `handler.UserHandler.MyLoginLogs(c *gin.Context)`, `.MySessions(c *gin.Context)` — both read `page`/`limit` query params (login logs only)

- [ ] **Step 1: Write failing test — sessions endpoint returns only non-revoked, non-expired tokens**

```go
// user-service/internal/handler/user_handler_test.go
package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/equipment-rental-system/user-service/internal/handler"
	"github.com/equipment-rental-system/user-service/internal/model"
	"github.com/equipment-rental-system/user-service/internal/repository"
	"github.com/equipment-rental-system/user-service/internal/service"
)

func setupHandlerWithUser(t *testing.T) (*gin.Engine, *model.User, *gorm.DB) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Role{}, &model.User{}, &model.RefreshToken{}, &model.LoginLog{}))
	require.NoError(t, db.Create(&model.Role{ID: 3, Name: "customer"}).Error)
	u := &model.User{Email: "s@example.com", Username: "s", PasswordHash: "x", RoleID: 3}
	require.NoError(t, db.Create(u).Error)

	uh := handler.NewUserHandler(service.NewUserService(repository.NewUserRepo(db), repository.NewRefreshTokenRepo(db), repository.NewLoginLogRepo(db)))
	r := gin.New()
	r.GET("/sessions", func(c *gin.Context) { c.Set("user_id", u.ID.String()); c.Next() }, uh.MySessions)
	return r, u, db
}

func TestMySessions_OnlyActiveReturned(t *testing.T) {
	r, u, db := setupHandlerWithUser(t)
	require.NoError(t, db.Create(&model.RefreshToken{UserID: u.ID, TokenHash: "active", ExpiresAt: time.Now().Add(time.Hour)}).Error)
	revokedAt := time.Now()
	require.NoError(t, db.Create(&model.RefreshToken{UserID: u.ID, TokenHash: "revoked", ExpiresAt: time.Now().Add(time.Hour), RevokedAt: &revokedAt}).Error)

	req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/handler/... -run TestMySessions -v`
Expected: FAIL — `uh.MySessions` undefined

- [ ] **Step 3: Implement handlers + routes**

```go
// append to user-service/internal/handler/user_handler.go
import "strconv" // add to import block

func (h *UserHandler) MyLoginLogs(c *gin.Context) {
	id, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": gin.H{"code": "UNAUTHENTICATED", "message": "unauthenticated", "details": nil}})
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	logs, total, err := h.svc.LoginLogsForUser(id, page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"code": "INTERNAL_ERROR", "message": err.Error(), "details": nil}})
		return
	}
	items := make([]gin.H, 0, len(logs))
	for _, l := range logs {
		items = append(items, gin.H{"id": l.ID, "success": l.Success, "ip_address": l.IPAddress, "user_agent": l.UserAgent, "created_at": l.CreatedAt})
	}
	totalPages := (total + int64(limit) - 1) / int64(limit)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items, "meta": gin.H{"page": page, "limit": limit, "total": total, "total_pages": totalPages}})
}

func (h *UserHandler) MySessions(c *gin.Context) {
	id, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": gin.H{"code": "UNAUTHENTICATED", "message": "unauthenticated", "details": nil}})
		return
	}
	sessions, err := h.svc.ActiveSessions(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"code": "INTERNAL_ERROR", "message": err.Error(), "details": nil}})
		return
	}
	items := make([]gin.H, 0, len(sessions))
	for _, s := range sessions {
		items = append(items, gin.H{"id": s.ID, "ip_address": s.IPAddress, "user_agent": s.UserAgent, "created_at": s.CreatedAt, "expires_at": s.ExpiresAt})
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items})
}
```

```go
// append to user-service/internal/service/user_service.go
import "github.com/equipment-rental-system/user-service/internal/model" // already imported; ensure present

func (s *UserService) LoginLogsForUser(userID uuid.UUID, page, limit int) ([]model.LoginLog, int64, error) {
	return s.loginLogs.ListForUser(userID, page, limit)
}

func (s *UserService) ActiveSessions(userID uuid.UUID) ([]model.RefreshToken, error) {
	return s.refreshTokens.ListActiveForUser(userID)
}
```

Add `loginLogs *repository.LoginLogRepo` field to `UserService` struct and constructor (third parameter). This changes the constructor arity again (Task 9 introduced 1 arg, Task 10 changed it to 2), so update **every** existing call site to pass a third `*repository.LoginLogRepo` argument:
- `router.go`: `service.NewUserService(userRepo, refreshRepo, logRepo)`
- Task 9's `setupUserService` test helper in `user_service_test.go` (build a `LoginLogRepo` on the same in-memory db — it already has `model.LoginLog` migrated)
- Task 10's `setupUserServiceWithPassword` test helper in `user_service_test.go` (same — it already migrates `model.LoginLog`)

Run `go build ./...` after this task's changes and confirm no call site still uses the old 1-arg or 2-arg form.

```go
// user-service/internal/router/router.go — add
v1.GET("/me/login-logs", authMW, userHandler.MyLoginLogs)
v1.GET("/me/sessions", authMW, userHandler.MySessions)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/handler/... -run TestMySessions -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add user-service/internal/
git commit -m "feat(user-service): implement GET /me/login-logs and GET /me/sessions"
```

---

## Sprint 3 — RBAC + Admin user management

### Task 12: RBAC middleware (RequireRole)

**Files:**
- Create: `user-service/internal/middleware/rbac.go`
- Test: `user-service/internal/middleware/rbac_test.go`

**Interfaces:**
- Consumes: `c.Get("role")` set by `RequireAuth` (Task 8)
- Produces: `middleware.RequireRole(allowed ...string) gin.HandlerFunc` — role not in `allowed` → `403 FORBIDDEN`

- [ ] **Step 1: Write failing test**

```go
// user-service/internal/middleware/rbac_test.go
package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/equipment-rental-system/user-service/internal/middleware"
)

func TestRequireRole_DisallowedRole_Returns403(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/admin-only", func(c *gin.Context) { c.Set("role", "customer"); c.Next() },
		middleware.RequireRole("admin"), func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/admin-only", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestRequireRole_AllowedRole_Passes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/admin-only", func(c *gin.Context) { c.Set("role", "admin"); c.Next() },
		middleware.RequireRole("admin", "staff"), func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/admin-only", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/middleware/... -run TestRequireRole -v`
Expected: FAIL — `middleware.RequireRole` undefined

- [ ] **Step 3: Implement**

```go
// user-service/internal/middleware/rbac.go
package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func RequireRole(allowed ...string) gin.HandlerFunc {
	set := make(map[string]struct{}, len(allowed))
	for _, r := range allowed {
		set[r] = struct{}{}
	}
	return func(c *gin.Context) {
		role, _ := c.Get("role")
		roleStr, _ := role.(string)
		if _, ok := set[roleStr]; !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   gin.H{"code": "FORBIDDEN", "message": "บทบาทไม่มีสิทธิ์เข้าถึง resource นี้", "details": nil},
			})
			return
		}
		c.Next()
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/middleware/... -run TestRequireRole -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add user-service/internal/middleware/rbac.go user-service/internal/middleware/rbac_test.go
git commit -m "feat(user-service): add RequireRole RBAC middleware"
```

---

### Task 13: Admin — GET /users (list/search) + POST /users (create)

**Files:**
- Modify: `user-service/internal/service/user_service.go` — add `ListUsers`, `CreateUser`
- Create: `user-service/internal/handler/admin_user_handler.go`
- Modify: `user-service/internal/router/router.go` — add routes
- Test: append to `user-service/internal/service/user_service_test.go`

**Interfaces:**
- Consumes: `UserRepo.List/Create` (Task 4), `RoleRepo.FindByName` (Task 4)
- Produces: `(*UserService) ListUsers(f repository.UserFilter) ([]model.User, int64, error)`
- Produces: `(*UserService) CreateUser(req dto.CreateUserRequest, roles *repository.RoleRepo, cost int) (*model.User, error)`

- [ ] **Step 1: Write failing test — admin-created user gets requested role**

```go
// append to user-service/internal/service/user_service_test.go
func TestUserService_CreateUser_AssignsRequestedRole(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Role{}, &model.User{}, &model.RefreshToken{}, &model.LoginLog{}))
	require.NoError(t, db.Create(&model.Role{ID: 2, Name: "staff"}).Error)

	roleRepo := repository.NewRoleRepo(db)
	svc := service.NewUserService(repository.NewUserRepo(db), repository.NewRefreshTokenRepo(db), repository.NewLoginLogRepo(db))

	u, err := svc.CreateUser(dto.CreateUserRequest{
		Email: "staff1@example.com", Username: "staff1", Password: "St@ffPass1", Role: "staff",
	}, roleRepo, 4)
	require.NoError(t, err)
	require.Equal(t, int16(2), u.RoleID)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service/... -run TestUserService_CreateUser -v`
Expected: FAIL — `svc.CreateUser` undefined

- [ ] **Step 3: Implement**

```go
// append to user-service/internal/service/user_service.go
import (
	"golang.org/x/crypto/bcrypt"

	"github.com/equipment-rental-system/user-service/internal/dto"
)

func (s *UserService) ListUsers(f repository.UserFilter) ([]model.User, int64, error) {
	return s.users.List(f)
}

func (s *UserService) CreateUser(req dto.CreateUserRequest, roles *repository.RoleRepo, cost int) (*model.User, error) {
	if _, err := s.users.FindByEmail(req.Email); err == nil {
		return nil, ErrEmailExists
	}
	if _, err := s.users.FindByUsername(req.Username); err == nil {
		return nil, ErrUsernameExists
	}
	role, err := roles.FindByName(req.Role)
	if err != nil {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), cost)
	if err != nil {
		return nil, err
	}
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	u := &model.User{
		Email: req.Email, Username: req.Username, PasswordHash: string(hash),
		FullName: req.FullName, Phone: req.Phone, RoleID: role.ID, IsActive: isActive,
	}
	if err := s.users.Create(u); err != nil {
		return nil, err
	}
	u.Role = *role
	return u, nil
}
```

```go
// user-service/internal/handler/admin_user_handler.go
package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

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

func userToJSON(u interface{ GetForJSON() gin.H }) gin.H { return u.GetForJSON() }

func (h *AdminUserHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
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
```

(Remove the unused `userToJSON` helper — it was scaffolding only; do not include it in the committed file.)

```go
// user-service/internal/router/router.go — add
adminUserHandler := handler.NewAdminUserHandler(userSvc, roleRepo, cfg.BCryptCost)
v1.GET("/users", authMW, middleware.RequireRole("admin"), adminUserHandler.List)
v1.POST("/users", authMW, middleware.RequireRole("admin"), adminUserHandler.Create)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/service/... -run TestUserService_CreateUser -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add user-service/internal/
git commit -m "feat(user-service): implement GET/POST /users (admin list + create)"
```

---

### Task 14: Admin — GET/PUT/DELETE /users/{id}

**Files:**
- Modify: `user-service/internal/handler/admin_user_handler.go` — add `Get`, `Update`, `Delete`
- Modify: `user-service/internal/router/router.go` — add routes
- Test: `user-service/internal/handler/admin_user_handler_test.go`

**Interfaces:**
- Consumes: `UserRepo.FindByID/Update/SoftDelete` (Task 4)
- Produces: `AdminUserHandler.Get/Update/Delete(c *gin.Context)` — `Get` allows `admin,staff`; `Update`/`Delete` admin-only

- [ ] **Step 1: Write failing test — delete removes user from subsequent list (soft delete)**

```go
// user-service/internal/handler/admin_user_handler_test.go
package handler_test

import (
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
	"github.com/equipment-rental-system/user-service/internal/service"
)

func setupAdminHandler(t *testing.T) (*gin.Engine, *model.User, *gorm.DB) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Role{}, &model.User{}, &model.RefreshToken{}, &model.LoginLog{}))
	require.NoError(t, db.Create(&model.Role{ID: 3, Name: "customer"}).Error)
	u := &model.User{Email: "z@example.com", Username: "z", PasswordHash: "x", RoleID: 3}
	require.NoError(t, db.Create(u).Error)

	svc := service.NewUserService(repository.NewUserRepo(db), repository.NewRefreshTokenRepo(db), repository.NewLoginLogRepo(db))
	ah := handler.NewAdminUserHandler(svc, repository.NewRoleRepo(db), 4)
	r := gin.New()
	r.DELETE("/users/:id", ah.Delete)
	r.GET("/users/:id", ah.Get)
	return r, u, db
}

func TestAdminUserHandler_Delete_ThenGet_Returns404(t *testing.T) {
	r, u, _ := setupAdminHandler(t)

	req := httptest.NewRequest(http.MethodDelete, "/users/"+u.ID.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	req2 := httptest.NewRequest(http.MethodGet, "/users/"+u.ID.String(), nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusNotFound, w2.Code)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/handler/... -run TestAdminUserHandler_Delete -v`
Expected: FAIL — `ah.Delete`/`ah.Get` undefined

- [ ] **Step 3: Implement**

```go
// append to user-service/internal/handler/admin_user_handler.go
import "github.com/google/uuid" // add to import block

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

func (h *AdminUserHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": gin.H{"code": "NOT_FOUND", "message": "ไม่พบผู้ใช้", "details": nil}})
		return
	}
	var req dto.UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"code": "VALIDATION_ERROR", "message": err.Error(), "details": nil}})
		return
	}
	u, err := h.svc.UpdateProfile(id, req)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": gin.H{"code": "NOT_FOUND", "message": "ไม่พบผู้ใช้", "details": nil}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"id": u.ID, "email": u.Email, "username": u.Username, "full_name": u.FullName,
		"phone": u.Phone, "role": u.Role.Name, "is_active": u.IsActive, "updated_at": u.UpdatedAt,
	}})
}

func (h *AdminUserHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": gin.H{"code": "NOT_FOUND", "message": "ไม่พบผู้ใช้", "details": nil}})
		return
	}
	requesterID, _ := currentUserID(c)
	if requesterID == id {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "error": gin.H{"code": "FORBIDDEN", "message": "ไม่สามารถลบบัญชีตัวเองได้", "details": nil}})
		return
	}
	if err := h.svc.DeleteUser(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": gin.H{"code": "NOT_FOUND", "message": "ไม่พบผู้ใช้", "details": nil}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"message": "ลบผู้ใช้เรียบร้อย"}})
}
```

```go
// append to user-service/internal/service/user_service.go
func (s *UserService) DeleteUser(id uuid.UUID) error {
	if _, err := s.users.FindByID(id); err != nil {
		return err
	}
	if err := s.refreshTokens.RevokeAllForUser(id); err != nil {
		return err
	}
	return s.users.SoftDelete(id)
}
```

```go
// user-service/internal/router/router.go — add
v1.GET("/users/:id", authMW, middleware.RequireRole("admin", "staff"), adminUserHandler.Get)
v1.PUT("/users/:id", authMW, middleware.RequireRole("admin"), adminUserHandler.Update)
v1.DELETE("/users/:id", authMW, middleware.RequireRole("admin"), adminUserHandler.Delete)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/handler/... -run TestAdminUserHandler_Delete -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add user-service/internal/
git commit -m "feat(user-service): implement GET/PUT/DELETE /users/{id}"
```

---

### Task 15: Admin — PATCH /users/{id}/role and /users/{id}/status

**Files:**
- Modify: `user-service/internal/service/user_service.go` — add `ChangeRole`, `ChangeStatus`
- Modify: `user-service/internal/handler/admin_user_handler.go` — add `ChangeRole`, `ChangeStatus`
- Modify: `user-service/internal/router/router.go` — add routes
- Test: append to `user-service/internal/service/user_service_test.go`

**Interfaces:**
- Consumes: `RoleRepo.FindByName`, `RefreshTokenRepo.RevokeAllForUser` (Task 4)
- Produces: `(*UserService) ChangeRole(id uuid.UUID, roleName string, roles *repository.RoleRepo) (*model.User, error)`
- Produces: `(*UserService) ChangeStatus(id uuid.UUID, isActive bool) (*model.User, error)` — disabling revokes all refresh tokens

- [ ] **Step 1: Write failing test — disabling revokes sessions**

```go
// append to user-service/internal/service/user_service_test.go
func TestUserService_ChangeStatus_DisablingRevokesSessions(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Role{}, &model.User{}, &model.RefreshToken{}, &model.LoginLog{}))
	require.NoError(t, db.Create(&model.Role{ID: 3, Name: "customer"}).Error)
	u := &model.User{Email: "y@example.com", Username: "y", PasswordHash: "x", RoleID: 3, IsActive: true}
	require.NoError(t, db.Create(u).Error)
	refreshRepo := repository.NewRefreshTokenRepo(db)
	require.NoError(t, refreshRepo.Create(&model.RefreshToken{UserID: u.ID, TokenHash: "abc", ExpiresAt: time.Now().Add(time.Hour)}))

	svc := service.NewUserService(repository.NewUserRepo(db), refreshRepo, repository.NewLoginLogRepo(db))
	updated, err := svc.ChangeStatus(u.ID, false)
	require.NoError(t, err)
	require.False(t, updated.IsActive)

	active, err := refreshRepo.ListActiveForUser(u.ID)
	require.NoError(t, err)
	require.Empty(t, active)
}
```

(add `"time"` import to the test file if not already present)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service/... -run TestUserService_ChangeStatus -v`
Expected: FAIL — `svc.ChangeStatus` undefined

- [ ] **Step 3: Implement**

```go
// append to user-service/internal/service/user_service.go
func (s *UserService) ChangeRole(id uuid.UUID, roleName string, roles *repository.RoleRepo) (*model.User, error) {
	u, err := s.users.FindByID(id)
	if err != nil {
		return nil, err
	}
	role, err := roles.FindByName(roleName)
	if err != nil {
		return nil, err
	}
	u.RoleID = role.ID
	if err := s.users.Update(u); err != nil {
		return nil, err
	}
	u.Role = *role
	return u, nil
}

func (s *UserService) ChangeStatus(id uuid.UUID, isActive bool) (*model.User, error) {
	u, err := s.users.FindByID(id)
	if err != nil {
		return nil, err
	}
	u.IsActive = isActive
	if err := s.users.Update(u); err != nil {
		return nil, err
	}
	if !isActive {
		if err := s.refreshTokens.RevokeAllForUser(id); err != nil {
			return nil, err
		}
	}
	return u, nil
}
```

```go
// append to user-service/internal/handler/admin_user_handler.go
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

func (h *AdminUserHandler) ChangeStatus(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": gin.H{"code": "NOT_FOUND", "message": "ไม่พบผู้ใช้", "details": nil}})
		return
	}
	requesterID, _ := currentUserID(c)
	if requesterID == id {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "error": gin.H{"code": "FORBIDDEN", "message": "ไม่สามารถปิดบัญชีตัวเองได้", "details": nil}})
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
```

```go
// user-service/internal/router/router.go — add
v1.PATCH("/users/:id/role", authMW, middleware.RequireRole("admin"), adminUserHandler.ChangeRole)
v1.PATCH("/users/:id/status", authMW, middleware.RequireRole("admin"), adminUserHandler.ChangeStatus)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/service/... -run TestUserService_ChangeStatus -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add user-service/internal/
git commit -m "feat(user-service): implement PATCH /users/{id}/role and /users/{id}/status"
```

---

### Task 16: Admin — GET /users/{id}/login-logs and GET /roles

**Files:**
- Modify: `user-service/internal/handler/admin_user_handler.go` — add `LoginLogsForUser`
- Create: `user-service/internal/handler/role_handler.go`
- Modify: `user-service/internal/router/router.go` — add routes
- Test: `user-service/internal/handler/role_handler_test.go`

**Interfaces:**
- Consumes: `UserService.LoginLogsForUser` (Task 11), `RoleRepo.List` (Task 4)
- Produces: `handler.NewRoleHandler(roles *repository.RoleRepo) *RoleHandler` with `List(c *gin.Context)`

- [ ] **Step 1: Write failing test — roles list returns seeded 3 roles in id order**

```go
// user-service/internal/handler/role_handler_test.go
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
	var body struct{ Data []map[string]any `json:"data"` }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Data, 3)
	require.Equal(t, "admin", body.Data[0]["name"])
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/handler/... -run TestRoleHandler_List -v`
Expected: FAIL — `handler.NewRoleHandler` undefined

- [ ] **Step 3: Implement**

```go
// user-service/internal/handler/role_handler.go
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
```

```go
// append to user-service/internal/handler/admin_user_handler.go
import "strconv" // already imported above in Task 13 — do not duplicate

func (h *AdminUserHandler) LoginLogsForUser(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": gin.H{"code": "NOT_FOUND", "message": "ไม่พบผู้ใช้", "details": nil}})
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	logs, total, err := h.svc.LoginLogsForUser(id, page, limit)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": gin.H{"code": "NOT_FOUND", "message": "ไม่พบผู้ใช้", "details": nil}})
		return
	}
	items := make([]gin.H, 0, len(logs))
	for _, l := range logs {
		items = append(items, gin.H{"id": l.ID, "email_attempted": l.EmailAttempted, "success": l.Success, "ip_address": l.IPAddress, "user_agent": l.UserAgent, "created_at": l.CreatedAt})
	}
	totalPages := (total + int64(limit) - 1) / int64(limit)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items, "meta": gin.H{"page": page, "limit": limit, "total": total, "total_pages": totalPages}})
}
```

```go
// user-service/internal/router/router.go — add
roleHandler := handler.NewRoleHandler(roleRepo)
v1.GET("/users/:id/login-logs", authMW, middleware.RequireRole("admin"), adminUserHandler.LoginLogsForUser)
v1.GET("/roles", authMW, middleware.RequireRole("admin", "staff"), roleHandler.List)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/handler/... -run TestRoleHandler_List -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add user-service/internal/
git commit -m "feat(user-service): implement GET /users/{id}/login-logs and GET /roles"
```

---

## Sprint 4 — Internal endpoint

### Task 17: Internal-key middleware + POST /auth/verify

**Files:**
- Create: `user-service/internal/middleware/internal.go`
- Modify: `user-service/internal/service/auth_service.go` — add `VerifyToken`
- Modify: `user-service/internal/handler/auth_handler.go` — add `Verify`
- Modify: `user-service/internal/router/router.go` — add route
- Test: `user-service/internal/middleware/internal_test.go`, append to `auth_service_test.go`

**Interfaces:**
- Consumes: `TokenService.ParseAccessToken` (Task 3), `UserRepo.FindByID` (Task 4)
- Produces: `middleware.RequireInternalKey(expected string) gin.HandlerFunc` — wrong/missing `X-Internal-Key` → `403 FORBIDDEN`
- Produces: `(*AuthService) VerifyToken(rawToken string) (*model.User, expiresAt time.Time, err error)` — `ErrInvalidCredentials` if parse fails, `ErrAccountDisabled` if inactive

- [ ] **Step 1: Write failing tests**

```go
// user-service/internal/middleware/internal_test.go
package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/equipment-rental-system/user-service/internal/middleware"
)

func TestRequireInternalKey_WrongKey_Returns403(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/internal", middleware.RequireInternalKey("correct-key"), func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/internal", nil)
	req.Header.Set("X-Internal-Key", "wrong-key")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
}
```

```go
// append to user-service/internal/service/auth_service_test.go
func TestAuthService_VerifyToken_DisabledAccount_ReturnsErrAccountDisabled(t *testing.T) {
	svc := setupAuthService(t)
	_, err := svc.Register(dto.RegisterRequest{Email: "v@example.com", Username: "userv", Password: "Passw0rd1"})
	require.NoError(t, err)
	access, _, _, user, err := svc.Login(dto.LoginRequest{Email: "v@example.com", Password: "Passw0rd1"}, "ip", "ua")
	require.NoError(t, err)

	user.IsActive = false
	require.NoError(t, svc.UsersRepoForTest().Update(user))

	_, _, err = svc.VerifyToken(access)
	require.ErrorIs(t, err, service.ErrAccountDisabled)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/middleware/... -run TestRequireInternalKey -v` and `go test ./internal/service/... -run TestAuthService_VerifyToken -v`
Expected: both FAIL — symbols undefined

- [ ] **Step 3: Implement**

```go
// user-service/internal/middleware/internal.go
package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func RequireInternalKey(expected string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("X-Internal-Key") != expected {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   gin.H{"code": "FORBIDDEN", "message": "invalid internal key", "details": nil},
			})
			return
		}
		c.Next()
	}
}
```

```go
// append to user-service/internal/service/auth_service.go
// "time" is already imported (added in Task 5) — no import change needed here

func (s *AuthService) VerifyToken(rawToken string) (*model.User, time.Time, error) {
	claims, err := s.tokens.ParseAccessToken(rawToken)
	if err != nil {
		return nil, time.Time{}, ErrInvalidCredentials
	}
	userID, err := uuidParse(claims.Sub)
	if err != nil {
		return nil, time.Time{}, ErrInvalidCredentials
	}
	u, err := s.users.FindByID(userID)
	if err != nil {
		return nil, time.Time{}, ErrInvalidCredentials
	}
	if !u.IsActive {
		return nil, time.Time{}, ErrAccountDisabled
	}
	return u, claims.ExpiresAt.Time, nil
}
```

```go
// user-service/internal/service/token_service.go — add helper used above
// "github.com/google/uuid" is already imported (added in Task 3) — no import change needed here

func uuidParse(s string) (uuid.UUID, error) { return uuid.Parse(s) }
```

```go
// append to user-service/internal/handler/auth_handler.go
type VerifyRequest struct {
	Token string `json:"token" binding:"required"`
}

func (h *AuthHandler) Verify(c *gin.Context) {
	var req VerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"code": "VALIDATION_ERROR", "message": err.Error(), "details": nil}})
		return
	}
	u, expiresAt, err := h.svc.VerifyToken(req.Token)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAccountDisabled):
			c.JSON(http.StatusForbidden, gin.H{"success": false, "error": gin.H{"code": "ACCOUNT_DISABLED", "message": "บัญชีถูกปิดการใช้งาน", "details": nil}})
		default:
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": gin.H{"code": "UNAUTHENTICATED", "message": "token ไม่ถูกต้องหรือหมดอายุ", "details": nil}})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"active": true, "user_id": u.ID, "email": u.Email, "username": u.Username, "role": u.Role.Name, "expires_at": expiresAt,
	}})
}
```

```go
// user-service/internal/router/router.go — add
v1.POST("/auth/verify", middleware.RequireInternalKey(cfg.InternalAPIKey), authHandler.Verify)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/middleware/... -run TestRequireInternalKey -v` and `go test ./internal/service/... -run TestAuthService_VerifyToken -v`
Expected: both PASS

- [ ] **Step 5: Commit**

```bash
git add user-service/internal/
git commit -m "feat(user-service): implement POST /auth/verify with internal-key middleware"
```

---

## Sprint 5 — Containerization

### Task 18: Dockerfile + docker-compose wiring + end-to-end smoke test

**Files:**
- Create: `user-service/Dockerfile`
- Create: `docker-compose.yml` (repo root — first service entry; product/rental services append to this same file in their own plans)
- Modify: `user-service/.env.example` (already created in Task 1 — verify values match compose)
- Test: manual smoke test (documented below; no Go test file — this task's "test" is the running container)

**Interfaces:**
- Consumes: everything from Tasks 1–17
- Produces: a running `user-service` container reachable at `http://localhost:8081`

- [ ] **Step 1: Write the Dockerfile**

```dockerfile
# user-service/Dockerfile
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/user-service ./cmd/api

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /bin/user-service /bin/user-service
COPY --from=builder /app/migrations /migrations
EXPOSE 8081
ENTRYPOINT ["/bin/user-service"]
```

- [ ] **Step 2: Create root docker-compose.yml with user-service + user-db**

```yaml
# docker-compose.yml (repo root)
version: "3.9"

services:
  user-service:
    build: ./user-service
    container_name: user-service
    ports: ["8081:8081"]
    env_file: [./.env]
    depends_on:
      user-db:
        condition: service_healthy
    networks: [rental-net]

  user-db:
    image: postgres:16-alpine
    container_name: user-db
    environment:
      POSTGRES_USER: user_service
      POSTGRES_PASSWORD: secret
      POSTGRES_DB: user_db
    volumes: ["user-db-data:/var/lib/postgresql/data"]
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U user_service -d user_db"]
      interval: 5s
      timeout: 5s
      retries: 5
    networks: [rental-net]

volumes:
  user-db-data:

networks:
  rental-net:
    name: rental-net
    driver: bridge
```

- [ ] **Step 3: Run migrations, build, and smoke-test manually**

```bash
cp .env.example .env
docker compose up -d --build
docker compose exec user-service /bin/sh -c \
  "apk add --no-cache curl >/dev/null 2>&1 || true"
curl http://localhost:8081/health
```

Expected: `{"success":true,"data":{"status":"ok","service":"user-service","db":"up"}}`

Then run migrations (if not auto-run in `main.go` — this plan did not add auto-migration; run manually):

```bash
docker run --rm --network rental-net \
  -v "$(pwd)/user-service/migrations:/migrations" \
  migrate/migrate -path=/migrations \
  -database "postgres://user_service:secret@user-db:5432/user_db?sslmode=disable" up
```

```bash
curl -X POST http://localhost:8081/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"smoke@example.com","username":"smoke","password":"Passw0rd1","full_name":"Smoke Test"}'

curl -X POST http://localhost:8081/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"smoke@example.com","password":"Passw0rd1"}'
```

Expected: register returns `201` with the created user; login returns `200` with `access_token`/`refresh_token`.

- [ ] **Step 4: Confirm all automated tests still pass before committing**

Run: `cd user-service && go test ./... -v`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add user-service/Dockerfile docker-compose.yml .env.example
git commit -m "feat(user-service): add Dockerfile, root docker-compose, verify end-to-end smoke test"
```

---

## What this plan does NOT cover

- `product-service` and `rental-service` — separate plans (next in sequence), following the same Go/Gin/GORM/migrate conventions established here
- Kong Gateway wiring — separate plan (`docs/superpowers/specs/2026-09-18-kong-api-gateway-design.md`), implemented only after all 3 services exist and run
- Rate limiting on `/auth/login`/`/auth/register` (mentioned in the design doc §8.4 as a nice-to-have `RateLimit` middleware) — out of scope; not required by CONTRACT.md

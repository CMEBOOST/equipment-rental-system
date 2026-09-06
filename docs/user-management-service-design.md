# เอกสารออกแบบระบบ: User Management Service (JWT & RBAC)

> โครงงาน: **ระบบเช่าอุปกรณ์ (Equipment Rental System)**
> โมดูล: **ระบบจัดการผู้ใช้งาน** — ผู้รับผิดชอบ: **สุรเชษฐ์ สีสา (67114540583)**
> สถาปัตยกรรม: Microservices · ภาษา: Go · ฐานข้อมูล: PostgreSQL (Docker)
> ขอบเขตเอกสาร: เฉพาะโมดูล User Management เท่านั้น

---

## สารบัญ

1. [ภาพรวมระบบ](#1-ภาพรวมระบบ)
2. [สถาปัตยกรรมและเทคโนโลยี](#2-สถาปัตยกรรมและเทคโนโลยี)
3. [Database Schema](#3-database-schema)
4. [โครงสร้างโปรเจกต์ (Go)](#4-โครงสร้างโปรเจกต์-go)
5. [Docker & docker-compose](#5-docker--docker-compose)
6. [มาตรฐาน API (Response / Error / Auth)](#6-มาตรฐาน-api)
7. [รายการ API Path ทั้งหมด](#7-รายการ-api-path-ทั้งหมด)
8. [JWT และความปลอดภัย](#8-jwt-และความปลอดภัย)
9. [ขั้นตอนการรันโปรเจกต์](#9-ขั้นตอนการรันโปรเจกต์)

---

## 1. ภาพรวมระบบ

User Management Service เป็นบริการกลางที่ทำหน้าที่:

- **Authentication** — ลงทะเบียน, เข้าสู่ระบบ, ออกจากระบบ, ออก/ต่ออายุ JWT
- **Authorization (RBAC)** — ควบคุมสิทธิ์การเข้าถึงตามบทบาท (Admin / Staff / Customer)
- **User Management** — เพิ่ม / แก้ไข / ลบ / ค้นหาผู้ใช้, กำหนดบทบาท, เปิด-ปิดบัญชี
- **Audit** — บันทึกประวัติการเข้าสู่ระบบ (Login Log)
- เป็น **แหล่งความจริงเดียว (single source of truth)** ของข้อมูลผู้ใช้และบทบาทในระบบ

### ความเชื่อมโยงกับบริการอื่น

```
                 ┌────────────────────────┐
   Client ─────► │  User Management :8081  │  (โมดูลนี้)
   (Web/Mobile)  │  - ออก JWT (HS256)     │
                 │  - /auth/verify         │
                 └───────────┬────────────┘
                             │ ใช้ JWT_SECRET ร่วมกัน / เรียก /auth/verify
              ┌──────────────┼───────────────┐
              ▼                              ▼
   ┌────────────────────┐        ┌────────────────────┐
   │ Product Service     │        │ Rental Service      │
   │ :8082 (เอกพล)       │        │ :8083 (วิษณุพงศ์)   │
   │ - ตรวจ JWT เอง      │        │ - ตรวจ JWT เอง      │
   │ - อ่าน role จาก claim│       │ - อ่าน user id/role │
   └────────────────────┘        └────────────────────┘
```

- ทุกบริการใช้ **`JWT_SECRET` เดียวกัน** (แจกผ่าน environment variable) จึงสามารถถอดและตรวจสอบลายเซ็น JWT ได้เองโดยไม่ต้องเรียกข้ามบริการทุกครั้ง
- บริการอื่นอ่าน `sub` (user id) และ `role` จาก claim ของ token ไปใช้บังคับสิทธิ์ในฝั่งตนเอง
- กรณีต้องการตรวจสอบแบบเข้มงวด (เช็คว่าบัญชียังไม่ถูกปิด / token ยังไม่ถูก revoke) ให้เรียก `POST /api/v1/auth/verify` ของโมดูลนี้

### ตารางสิทธิ์การเข้าถึงตามบทบาท (RBAC)

| บทบาท | สิทธิ์ในโมดูล User Management | สิทธิ์ในภาพรวมระบบ (อ้างอิง) |
|---|---|---|
| **Admin** | จัดการผู้ใช้ทั้งหมด, กำหนด/เปลี่ยนบทบาท, เปิด-ปิดบัญชี, ดู login log ของทุกคน | จัดการสินค้าและการเช่าได้ทั้งหมด |
| **Staff** | ดูรายชื่อผู้ใช้และรายละเอียด (อ่านอย่างเดียว), ดูรายการบทบาท, จัดการโปรไฟล์ตนเอง | จัดการสินค้า, สร้างรายการเช่า, บันทึกการคืน |
| **Customer** | จัดการเฉพาะโปรไฟล์และรหัสผ่านของตนเอง, ดู login log ของตนเอง | ดูสินค้า, สร้างคำขอเช่า, ดูประวัติการเช่าของตนเอง |

---

## 2. สถาปัตยกรรมและเทคโนโลยี

| หัวข้อ | รายละเอียด |
|---|---|
| ภาษา | Go 1.23 |
| HTTP Router | Gin (`github.com/gin-gonic/gin`) |
| ORM / DB access | GORM (`gorm.io/gorm`) + driver `gorm.io/driver/postgres` |
| ฐานข้อมูล | PostgreSQL 16 (คอนเทนเนอร์แยกของโมดูลนี้) |
| JWT | `github.com/golang-jwt/jwt/v5` — อัลกอริทึม **HS256** |
| Password Hashing | `golang.org/x/crypto/bcrypt` (cost = 12) |
| Migration | `github.com/golang-migrate/migrate/v4` (ไฟล์ SQL ใน `migrations/`) |
| UUID | `github.com/google/uuid` (UUID v4 เป็น primary key) |
| Config | อ่านจาก environment variable (`.env` ผ่าน `github.com/joho/godotenv` ตอน dev) |
| Validation | `github.com/go-playground/validator/v10` (มากับ Gin binding) |

### พอร์ตของแต่ละบริการ (ข้อตกลงในทีม)

| บริการ | พอร์ต | ฐานข้อมูล |
|---|---|---|
| User Management (โมดูลนี้) | `8081` | `user-db` : 5432 |
| Product Service | `8082` | `product-db` |
| Rental Service | `8083` | `rental-db` |

### Environment Variables

| ตัวแปร | ตัวอย่าง | คำอธิบาย |
|---|---|---|
| `APP_PORT` | `8081` | พอร์ต HTTP ของบริการ |
| `DB_HOST` | `user-db` | โฮสต์ PostgreSQL |
| `DB_PORT` | `5432` | พอร์ต PostgreSQL |
| `DB_USER` | `user_service` | ผู้ใช้ฐานข้อมูล |
| `DB_PASSWORD` | `secret` | รหัสผ่านฐานข้อมูล |
| `DB_NAME` | `user_db` | ชื่อฐานข้อมูล |
| `JWT_SECRET` | `change-me-32-chars-min` | คีย์ลับสำหรับเซ็น/ตรวจ JWT (ใช้ร่วมกันทุกบริการ) |
| `JWT_ACCESS_TTL` | `15m` | อายุ access token |
| `JWT_REFRESH_TTL` | `168h` | อายุ refresh token (7 วัน) |
| `INTERNAL_API_KEY` | `internal-shared-key` | คีย์สำหรับเรียก `/auth/verify` จากบริการภายใน |
| `BCRYPT_COST` | `12` | ความแรงของ bcrypt |

### สถาปัตยกรรมภายใน (Layered)

```
HTTP Request
   │
   ▼
[ Router (Gin) ]
   │
   ▼
[ Middleware ]  ── RequestID · Logger · Recovery · CORS
   │              RequireAuth (ตรวจ JWT)
   │              RequireRole (ตรวจบทบาท)
   ▼
[ Handler ]      ── รับ/validate request, แปลงเป็น DTO, จัด HTTP response
   │
   ▼
[ Service ]      ── business logic (hash password, ออก token, หมุน refresh token)
   │
   ▼
[ Repository ]   ── query ฐานข้อมูลผ่าน GORM
   │
   ▼
[ PostgreSQL ]
```

---

## 3. Database Schema

ฐานข้อมูล `user_db` มี 4 ตาราง

### 3.1 ERD (แบบข้อความ)

```
roles (1) ───< (N) users (1) ───< (N) refresh_tokens
                       │
                       └───< (N) login_logs
```

### 3.2 DDL

```sql
-- extension สำหรับสุ่ม UUID
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- 1) ตารางบทบาท
CREATE TABLE roles (
    id          SMALLINT     PRIMARY KEY,
    name        VARCHAR(20)  NOT NULL UNIQUE,      -- 'admin' | 'staff' | 'customer'
    description VARCHAR(255) NOT NULL DEFAULT ''
);

-- 2) ตารางผู้ใช้
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
    deleted_at    TIMESTAMPTZ  NULL                  -- soft delete
);
CREATE INDEX idx_users_role_id    ON users(role_id);
CREATE INDEX idx_users_deleted_at ON users(deleted_at);

-- 3) ตาราง refresh token (เก็บเฉพาะ hash ของ token)
CREATE TABLE refresh_tokens (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  VARCHAR(64)  NOT NULL UNIQUE,       -- SHA-256 hex ของ refresh token
    expires_at  TIMESTAMPTZ  NOT NULL,
    revoked_at  TIMESTAMPTZ  NULL,                  -- logout / rotate = ตั้งค่าเวลานี้
    ip_address  VARCHAR(45)  NOT NULL DEFAULT '',
    user_agent  VARCHAR(255) NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens(user_id);

-- 4) ตารางประวัติการเข้าสู่ระบบ
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

### 3.3 Seed data

```sql
INSERT INTO roles (id, name, description) VALUES
    (1, 'admin',    'ผู้ดูแลระบบ จัดการผู้ใช้ บทบาท สินค้า และการเช่าทั้งหมด'),
    (2, 'staff',    'เจ้าหน้าที่ จัดการสินค้า สร้างรายการเช่า และบันทึกการคืน'),
    (3, 'customer', 'ลูกค้า ดูสินค้า สร้างคำขอเช่า และดูประวัติการเช่าของตนเอง');

-- ผู้ดูแลระบบเริ่มต้น (รหัสผ่าน: Admin@123 — ต้องเปลี่ยนหลังติดตั้ง)
INSERT INTO users (email, username, password_hash, full_name, role_id)
VALUES ('admin@example.com', 'admin',
        '$2a$12$REPLACE_WITH_REAL_BCRYPT_HASH', 'System Admin', 1);
```

---

## 4. โครงสร้างโปรเจกต์ (Go)

```
user-service/
├── cmd/
│   └── api/
│       └── main.go                  # จุดเริ่มโปรแกรม: โหลด config, เชื่อม DB, start Gin
├── internal/
│   ├── config/
│   │   └── config.go                # โหลด env → struct Config
│   ├── db/
│   │   └── db.go                    # เชื่อมต่อ GORM + connection pool
│   ├── model/
│   │   ├── user.go                  # struct User, Role
│   │   ├── refresh_token.go
│   │   └── login_log.go
│   ├── dto/
│   │   ├── auth_dto.go              # RegisterRequest, LoginRequest, TokenResponse ...
│   │   └── user_dto.go              # CreateUserRequest, UpdateUserRequest ...
│   ├── repository/
│   │   ├── user_repo.go
│   │   ├── refresh_token_repo.go
│   │   └── login_log_repo.go
│   ├── service/
│   │   ├── auth_service.go          # register, login, refresh, logout, verify
│   │   ├── user_service.go          # CRUD ผู้ใช้, เปลี่ยน role, เปลี่ยน status
│   │   └── token_service.go         # สร้าง/ตรวจ JWT, hash refresh token
│   ├── handler/
│   │   ├── auth_handler.go
│   │   ├── user_handler.go
│   │   ├── role_handler.go
│   │   └── health_handler.go
│   ├── middleware/
│   │   ├── auth.go                  # RequireAuth (ตรวจ JWT → ใส่ claim ใน context)
│   │   ├── rbac.go                  # RequireRole("admin", ...)
│   │   ├── internal.go              # RequireInternalKey (สำหรับ /auth/verify)
│   │   └── common.go                # RequestID, Logger, Recovery, CORS
│   └── router/
│       └── router.go                # ประกอบ route ทั้งหมด
├── migrations/
│   ├── 000001_init_schema.up.sql
│   ├── 000001_init_schema.down.sql
│   ├── 000002_seed_roles.up.sql
│   └── 000002_seed_roles.down.sql
├── .env.example
├── Dockerfile
├── docker-compose.yml
├── go.mod
└── go.sum
```

---

## 5. Docker & docker-compose

### 5.1 Dockerfile (multi-stage)

```dockerfile
# ---- build stage ----
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/user-service ./cmd/api

# ---- runtime stage ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /bin/user-service /bin/user-service
COPY --from=builder /app/migrations /migrations
EXPOSE 8081
ENTRYPOINT ["/bin/user-service"]
```

### 5.2 docker-compose.yml (เฉพาะส่วนของโมดูลนี้)

```yaml
version: "3.9"

services:
  user-service:
    build: ./user-service
    container_name: user-service
    ports:
      - "8081:8081"
    environment:
      APP_PORT: "8081"
      DB_HOST: user-db
      DB_PORT: "5432"
      DB_USER: user_service
      DB_PASSWORD: secret
      DB_NAME: user_db
      JWT_SECRET: change-me-in-production-min-32-characters
      JWT_ACCESS_TTL: "15m"
      JWT_REFRESH_TTL: "168h"
      INTERNAL_API_KEY: internal-shared-key
      BCRYPT_COST: "12"
    depends_on:
      user-db:
        condition: service_healthy
    networks:
      - rental-net

  user-db:
    image: postgres:16-alpine
    container_name: user-db
    environment:
      POSTGRES_USER: user_service
      POSTGRES_PASSWORD: secret
      POSTGRES_DB: user_db
    volumes:
      - user-db-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U user_service -d user_db"]
      interval: 5s
      timeout: 5s
      retries: 5
    networks:
      - rental-net

volumes:
  user-db-data:

networks:
  rental-net:
    name: rental-net      # network กลางที่ทั้ง 3 บริการใช้ร่วมกัน
    driver: bridge
```

> หมายเหตุ: การรัน migration ทำตอน service เริ่มทำงาน (เรียก `migrate` ใน `main.go`) หรือรันมือด้วย `make migrate-up`

---

## 6. มาตรฐาน API

### 6.1 Base URL

```
http://localhost:8081/api/v1
```

### 6.2 Header

| Header | ใช้เมื่อ | ตัวอย่าง |
|---|---|---|
| `Authorization` | ทุก endpoint ที่ต้องล็อกอิน | `Bearer <access_token>` |
| `Content-Type` | request ที่มี body | `application/json` |
| `X-Internal-Key` | เฉพาะ `POST /auth/verify` | `internal-shared-key` |

### 6.3 รูปแบบ Response (envelope)

**สำเร็จ**

```json
{
  "success": true,
  "data": { }
}
```

**สำเร็จแบบมีรายการ (pagination)**

```json
{
  "success": true,
  "data": [ ],
  "meta": { "page": 1, "limit": 20, "total": 57, "total_pages": 3 }
}
```

**ผิดพลาด**

```json
{
  "success": false,
  "error": {
    "code": "INVALID_CREDENTIALS",
    "message": "อีเมลหรือรหัสผ่านไม่ถูกต้อง",
    "details": null
  }
}
```

### 6.4 ตารางรหัส Error

| HTTP | `error.code` | ความหมาย |
|---|---|---|
| 400 | `VALIDATION_ERROR` | ข้อมูลใน request ไม่ผ่านการตรวจสอบ (`details` เป็น map ของ field → ข้อความ) |
| 400 | `BAD_REQUEST` | รูปแบบ request ไม่ถูกต้อง เช่น JSON เสีย |
| 401 | `UNAUTHENTICATED` | ไม่มี token / token หมดอายุ / ลายเซ็นไม่ถูกต้อง |
| 401 | `INVALID_CREDENTIALS` | อีเมลหรือรหัสผ่านไม่ถูกต้อง (เฉพาะตอน login) |
| 401 | `INVALID_REFRESH_TOKEN` | refresh token ไม่ถูกต้อง / หมดอายุ / ถูก revoke |
| 403 | `FORBIDDEN` | บทบาทไม่มีสิทธิ์เข้าถึง resource นี้ |
| 403 | `ACCOUNT_DISABLED` | บัญชีถูกปิดการใช้งาน |
| 404 | `NOT_FOUND` | ไม่พบข้อมูลที่ร้องขอ |
| 409 | `EMAIL_ALREADY_EXISTS` | อีเมลนี้ถูกใช้แล้ว |
| 409 | `USERNAME_ALREADY_EXISTS` | username นี้ถูกใช้แล้ว |
| 422 | `WEAK_PASSWORD` | รหัสผ่านไม่ตรงเกณฑ์ (ยาว ≥ 8, มีตัวอักษรและตัวเลข) |
| 429 | `TOO_MANY_REQUESTS` | ขอ login ถี่เกินไป (rate limit) |
| 500 | `INTERNAL_ERROR` | ข้อผิดพลาดภายในเซิร์ฟเวอร์ |

### 6.5 กติกา Pagination / Filter

| Query param | ค่าเริ่มต้น | คำอธิบาย |
|---|---|---|
| `page` | `1` | หน้าที่ต้องการ (เริ่มที่ 1) |
| `limit` | `20` | จำนวนต่อหน้า (สูงสุด 100) |
| `sort` | `created_at` | ฟิลด์ที่ใช้เรียง |
| `order` | `desc` | `asc` หรือ `desc` |

---

## 7. รายการ API Path ทั้งหมด

สรุปทั้งหมด **20 endpoints**

| # | Method | Path | สิทธิ์ | หน้าที่ |
|---|---|---|---|---|
| 1 | POST | `/api/v1/auth/register` | Public | สมัครสมาชิก (เป็น Customer) |
| 2 | POST | `/api/v1/auth/login` | Public | เข้าสู่ระบบ |
| 3 | POST | `/api/v1/auth/refresh` | Public + refresh token | ต่ออายุ access token |
| 4 | POST | `/api/v1/auth/logout` | Authenticated | ออกจากระบบ |
| 5 | POST | `/api/v1/auth/verify` | Internal key | ให้บริการอื่นตรวจสอบ token |
| 6 | GET | `/api/v1/me` | Authenticated | ดูโปรไฟล์ตนเอง |
| 7 | PUT | `/api/v1/me` | Authenticated | แก้ไขโปรไฟล์ตนเอง |
| 8 | PUT | `/api/v1/me/password` | Authenticated | เปลี่ยนรหัสผ่านตนเอง |
| 9 | GET | `/api/v1/me/login-logs` | Authenticated | ดูประวัติการเข้าใช้ของตนเอง |
| 10 | GET | `/api/v1/users` | Admin | รายการผู้ใช้ (ค้นหา/แบ่งหน้า) |
| 11 | POST | `/api/v1/users` | Admin | สร้างผู้ใช้ + กำหนดบทบาท |
| 12 | GET | `/api/v1/users/{id}` | Admin, Staff | ดูรายละเอียดผู้ใช้ |
| 13 | PUT | `/api/v1/users/{id}` | Admin | แก้ไขข้อมูลผู้ใช้ |
| 14 | DELETE | `/api/v1/users/{id}` | Admin | ลบผู้ใช้ (soft delete) |
| 15 | PATCH | `/api/v1/users/{id}/role` | Admin | เปลี่ยนบทบาทผู้ใช้ |
| 16 | PATCH | `/api/v1/users/{id}/status` | Admin | เปิด/ปิดบัญชีผู้ใช้ |
| 17 | GET | `/api/v1/users/{id}/login-logs` | Admin | ดูประวัติการเข้าใช้ของผู้ใช้รายนั้น |
| 18 | GET | `/api/v1/roles` | Admin, Staff | รายการบทบาททั้งหมด |
| 19 | GET | `/health` | Public | ตรวจสอบสถานะบริการ |
| 20 | GET | `/api/v1/me/sessions` | Authenticated | ดูรายการ session (refresh token) ที่ยัง active |

---

### 7.1 `POST /api/v1/auth/register`

**หน้าที่:** สมัครสมาชิกใหม่ ระบบกำหนดบทบาทเป็น `customer` โดยอัตโนมัติ

| ส่วน | รายละเอียด |
|---|---|
| สิทธิ์ | Public |
| Body (JSON) | `email` (string, required, email), `username` (string, required, 3–50), `password` (string, required, ≥ 8, มีตัวอักษร + ตัวเลข), `full_name` (string, optional), `phone` (string, optional) |

**Request**

```json
{
  "email": "somchai@example.com",
  "username": "somchai",
  "password": "Passw0rd123",
  "full_name": "สมชาย ใจดี",
  "phone": "0812345678"
}
```

**Response codes:** `201` สร้างสำเร็จ · `400` `VALIDATION_ERROR` · `409` `EMAIL_ALREADY_EXISTS` / `USERNAME_ALREADY_EXISTS` · `422` `WEAK_PASSWORD`

**Response `201`**

```json
{
  "success": true,
  "data": {
    "id": "9b1c7c2e-3d4a-4a1b-8f0e-2a1b3c4d5e6f",
    "email": "somchai@example.com",
    "username": "somchai",
    "full_name": "สมชาย ใจดี",
    "phone": "0812345678",
    "role": "customer",
    "is_active": true,
    "created_at": "2026-09-06T09:00:00Z"
  }
}
```

**Response `409`**

```json
{ "success": false, "error": { "code": "EMAIL_ALREADY_EXISTS", "message": "อีเมลนี้ถูกใช้งานแล้ว", "details": null } }
```

---

### 7.2 `POST /api/v1/auth/login`

**หน้าที่:** ตรวจสอบอีเมล + รหัสผ่าน คืน access token และ refresh token พร้อมบันทึก login log

| ส่วน | รายละเอียด |
|---|---|
| สิทธิ์ | Public |
| Body (JSON) | `email` (string, required), `password` (string, required) |

**Request**

```json
{ "email": "somchai@example.com", "password": "Passw0rd123" }
```

**Response codes:** `200` สำเร็จ · `400` `VALIDATION_ERROR` · `401` `INVALID_CREDENTIALS` · `403` `ACCOUNT_DISABLED` · `429` `TOO_MANY_REQUESTS`

**Response `200`**

```json
{
  "success": true,
  "data": {
    "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "refresh_token": "d7f3a1b9c2e4...(opaque 64 hex)",
    "token_type": "Bearer",
    "expires_in": 900,
    "user": {
      "id": "9b1c7c2e-3d4a-4a1b-8f0e-2a1b3c4d5e6f",
      "email": "somchai@example.com",
      "username": "somchai",
      "full_name": "สมชาย ใจดี",
      "role": "customer"
    }
  }
}
```

**Response `401`**

```json
{ "success": false, "error": { "code": "INVALID_CREDENTIALS", "message": "อีเมลหรือรหัสผ่านไม่ถูกต้อง", "details": null } }
```

---

### 7.3 `POST /api/v1/auth/refresh`

**หน้าที่:** ใช้ refresh token แลก access token ใหม่ พร้อม **หมุน (rotate)** refresh token — token เดิมถูก revoke และออก token ใหม่

| ส่วน | รายละเอียด |
|---|---|
| สิทธิ์ | Public (ต้องส่ง refresh token ที่ยังใช้ได้) |
| Body (JSON) | `refresh_token` (string, required) |

**Request**

```json
{ "refresh_token": "d7f3a1b9c2e4..." }
```

**Response codes:** `200` สำเร็จ · `400` `VALIDATION_ERROR` · `401` `INVALID_REFRESH_TOKEN` · `403` `ACCOUNT_DISABLED`

**Response `200`**

```json
{
  "success": true,
  "data": {
    "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...(ใหม่)",
    "refresh_token": "a1b2c3d4...(ใหม่)",
    "token_type": "Bearer",
    "expires_in": 900
  }
}
```

**Response `401`**

```json
{ "success": false, "error": { "code": "INVALID_REFRESH_TOKEN", "message": "refresh token ไม่ถูกต้องหรือหมดอายุ", "details": null } }
```

---

### 7.4 `POST /api/v1/auth/logout`

**หน้าที่:** ออกจากระบบ — revoke refresh token ที่ระบุ (ตั้ง `revoked_at`) ทำให้ต่ออายุไม่ได้อีก

| ส่วน | รายละเอียด |
|---|---|
| สิทธิ์ | Authenticated (`Authorization: Bearer <access_token>`) |
| Body (JSON) | `refresh_token` (string, required) |

**Request**

```json
{ "refresh_token": "d7f3a1b9c2e4..." }
```

**Response codes:** `200` สำเร็จ · `401` `UNAUTHENTICATED`

**Response `200`**

```json
{ "success": true, "data": { "message": "ออกจากระบบเรียบร้อย" } }
```

---

### 7.5 `POST /api/v1/auth/verify`

**หน้าที่:** ให้บริการอื่น (Product / Rental) ส่ง access token มาตรวจสอบแบบเข้มงวด — ตรวจลายเซ็น, อายุ, สถานะบัญชี — และคืนข้อมูล user + role กลับไป

| ส่วน | รายละเอียด |
|---|---|
| สิทธิ์ | Internal — ต้องมี header `X-Internal-Key: <INTERNAL_API_KEY>` |
| Body (JSON) | `token` (string, required) — access token ที่จะตรวจ |

**Request**

```
POST /api/v1/auth/verify
X-Internal-Key: internal-shared-key
```

```json
{ "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." }
```

**Response codes:** `200` token ใช้ได้ · `401` `UNAUTHENTICATED` (token ไม่ถูกต้อง/หมดอายุ) · `403` `FORBIDDEN` (ไม่มี/ผิด internal key) หรือ `ACCOUNT_DISABLED`

**Response `200`**

```json
{
  "success": true,
  "data": {
    "active": true,
    "user_id": "9b1c7c2e-3d4a-4a1b-8f0e-2a1b3c4d5e6f",
    "email": "somchai@example.com",
    "username": "somchai",
    "role": "customer",
    "expires_at": "2026-09-06T09:15:00Z"
  }
}
```

**Response `401`**

```json
{ "success": false, "error": { "code": "UNAUTHENTICATED", "message": "token ไม่ถูกต้องหรือหมดอายุ", "details": null } }
```

---

### 7.6 `GET /api/v1/me`

**หน้าที่:** ดูข้อมูลโปรไฟล์ของผู้ใช้ที่ล็อกอินอยู่

| ส่วน | รายละเอียด |
|---|---|
| สิทธิ์ | Authenticated |
| Parameter | — (อ่าน user id จาก claim ของ token) |

**Response codes:** `200` สำเร็จ · `401` `UNAUTHENTICATED`

**Response `200`**

```json
{
  "success": true,
  "data": {
    "id": "9b1c7c2e-3d4a-4a1b-8f0e-2a1b3c4d5e6f",
    "email": "somchai@example.com",
    "username": "somchai",
    "full_name": "สมชาย ใจดี",
    "phone": "0812345678",
    "role": "customer",
    "is_active": true,
    "created_at": "2026-09-06T09:00:00Z",
    "updated_at": "2026-09-06T09:00:00Z"
  }
}
```

---

### 7.7 `PUT /api/v1/me`

**หน้าที่:** แก้ไขข้อมูลโปรไฟล์ของตนเอง (แก้ได้เฉพาะ `full_name` และ `phone` — ไม่ให้เปลี่ยน email/username/role ผ่าน endpoint นี้)

| ส่วน | รายละเอียด |
|---|---|
| สิทธิ์ | Authenticated |
| Body (JSON) | `full_name` (string, optional), `phone` (string, optional) |

**Request**

```json
{ "full_name": "สมชาย ใจดีมาก", "phone": "0899999999" }
```

**Response codes:** `200` สำเร็จ · `400` `VALIDATION_ERROR` · `401` `UNAUTHENTICATED`

**Response `200`**

```json
{
  "success": true,
  "data": {
    "id": "9b1c7c2e-3d4a-4a1b-8f0e-2a1b3c4d5e6f",
    "full_name": "สมชาย ใจดีมาก",
    "phone": "0899999999",
    "updated_at": "2026-09-06T10:30:00Z"
  }
}
```

---

### 7.8 `PUT /api/v1/me/password`

**หน้าที่:** เปลี่ยนรหัสผ่านตนเอง — ต้องยืนยันรหัสผ่านเดิม เมื่อสำเร็จจะ revoke refresh token ทั้งหมดของผู้ใช้ (บังคับล็อกอินใหม่ทุกอุปกรณ์)

| ส่วน | รายละเอียด |
|---|---|
| สิทธิ์ | Authenticated |
| Body (JSON) | `current_password` (string, required), `new_password` (string, required, ≥ 8, ตัวอักษร + ตัวเลข) |

**Request**

```json
{ "current_password": "Passw0rd123", "new_password": "N3wPassw0rd!" }
```

**Response codes:** `200` สำเร็จ · `400` `VALIDATION_ERROR` · `401` `INVALID_CREDENTIALS` (รหัสเดิมผิด) · `422` `WEAK_PASSWORD`

**Response `200`**

```json
{ "success": true, "data": { "message": "เปลี่ยนรหัสผ่านเรียบร้อย กรุณาเข้าสู่ระบบใหม่" } }
```

---

### 7.9 `GET /api/v1/me/login-logs`

**หน้าที่:** ดูประวัติการเข้าสู่ระบบของตนเอง (ทั้งสำเร็จและล้มเหลว)

| ส่วน | รายละเอียด |
|---|---|
| สิทธิ์ | Authenticated |
| Query | `page`, `limit`, `success` (optional: `true`/`false`) |

**Response codes:** `200` สำเร็จ · `401` `UNAUTHENTICATED`

**Response `200`**

```json
{
  "success": true,
  "data": [
    {
      "id": 1024,
      "success": true,
      "ip_address": "203.0.113.10",
      "user_agent": "Mozilla/5.0 ...",
      "created_at": "2026-09-06T09:00:00Z"
    },
    {
      "id": 1000,
      "success": false,
      "ip_address": "203.0.113.10",
      "user_agent": "Mozilla/5.0 ...",
      "created_at": "2026-09-05T22:14:00Z"
    }
  ],
  "meta": { "page": 1, "limit": 20, "total": 2, "total_pages": 1 }
}
```

---

### 7.10 `GET /api/v1/users`

**หน้าที่:** รายการผู้ใช้ทั้งหมด รองรับค้นหา กรอง และแบ่งหน้า (สำหรับหน้าจอจัดการผู้ใช้ของ Admin)

| ส่วน | รายละเอียด |
|---|---|
| สิทธิ์ | Admin |
| Query | `page`, `limit`, `sort`, `order`, `q` (ค้นหาจาก email/username/full_name), `role` (`admin`/`staff`/`customer`), `is_active` (`true`/`false`) |

**Request**

```
GET /api/v1/users?q=somchai&role=customer&is_active=true&page=1&limit=20
```

**Response codes:** `200` สำเร็จ · `401` `UNAUTHENTICATED` · `403` `FORBIDDEN`

**Response `200`**

```json
{
  "success": true,
  "data": [
    {
      "id": "9b1c7c2e-3d4a-4a1b-8f0e-2a1b3c4d5e6f",
      "email": "somchai@example.com",
      "username": "somchai",
      "full_name": "สมชาย ใจดี",
      "phone": "0812345678",
      "role": "customer",
      "is_active": true,
      "created_at": "2026-09-06T09:00:00Z"
    }
  ],
  "meta": { "page": 1, "limit": 20, "total": 1, "total_pages": 1 }
}
```

---

### 7.11 `POST /api/v1/users`

**หน้าที่:** Admin สร้างผู้ใช้ใหม่และกำหนดบทบาทได้ทันที (ใช้สร้างบัญชี Staff / Admin)

| ส่วน | รายละเอียด |
|---|---|
| สิทธิ์ | Admin |
| Body (JSON) | `email` (required, email), `username` (required, 3–50), `password` (required, ≥ 8), `full_name` (optional), `phone` (optional), `role` (required: `admin`/`staff`/`customer`), `is_active` (optional, default `true`) |

**Request**

```json
{
  "email": "staff01@example.com",
  "username": "staff01",
  "password": "St@ffPass1",
  "full_name": "เจ้าหน้าที่ หนึ่ง",
  "phone": "0876543210",
  "role": "staff",
  "is_active": true
}
```

**Response codes:** `201` สร้างสำเร็จ · `400` `VALIDATION_ERROR` · `401` `UNAUTHENTICATED` · `403` `FORBIDDEN` · `409` `EMAIL_ALREADY_EXISTS` / `USERNAME_ALREADY_EXISTS`

**Response `201`**

```json
{
  "success": true,
  "data": {
    "id": "1f2e3d4c-5b6a-7c8d-9e0f-1a2b3c4d5e6f",
    "email": "staff01@example.com",
    "username": "staff01",
    "full_name": "เจ้าหน้าที่ หนึ่ง",
    "phone": "0876543210",
    "role": "staff",
    "is_active": true,
    "created_at": "2026-09-06T11:00:00Z"
  }
}
```

---

### 7.12 `GET /api/v1/users/{id}`

**หน้าที่:** ดูรายละเอียดผู้ใช้รายคน

| ส่วน | รายละเอียด |
|---|---|
| สิทธิ์ | Admin, Staff |
| Path param | `id` (UUID, required) |

**Response codes:** `200` สำเร็จ · `401` `UNAUTHENTICATED` · `403` `FORBIDDEN` · `404` `NOT_FOUND`

**Response `200`**

```json
{
  "success": true,
  "data": {
    "id": "9b1c7c2e-3d4a-4a1b-8f0e-2a1b3c4d5e6f",
    "email": "somchai@example.com",
    "username": "somchai",
    "full_name": "สมชาย ใจดี",
    "phone": "0812345678",
    "role": "customer",
    "is_active": true,
    "created_at": "2026-09-06T09:00:00Z",
    "updated_at": "2026-09-06T09:00:00Z"
  }
}
```

**Response `404`**

```json
{ "success": false, "error": { "code": "NOT_FOUND", "message": "ไม่พบผู้ใช้", "details": null } }
```

---

### 7.13 `PUT /api/v1/users/{id}`

**หน้าที่:** Admin แก้ไขข้อมูลผู้ใช้ (`full_name`, `phone`, `email`, `username`)

| ส่วน | รายละเอียด |
|---|---|
| สิทธิ์ | Admin |
| Path param | `id` (UUID, required) |
| Body (JSON) | `email` (optional, email), `username` (optional, 3–50), `full_name` (optional), `phone` (optional) |

**Request**

```json
{ "full_name": "สมชาย ใจดีที่สุด", "phone": "0811111111" }
```

**Response codes:** `200` สำเร็จ · `400` `VALIDATION_ERROR` · `401` `UNAUTHENTICATED` · `403` `FORBIDDEN` · `404` `NOT_FOUND` · `409` `EMAIL_ALREADY_EXISTS` / `USERNAME_ALREADY_EXISTS`

**Response `200`**

```json
{
  "success": true,
  "data": {
    "id": "9b1c7c2e-3d4a-4a1b-8f0e-2a1b3c4d5e6f",
    "email": "somchai@example.com",
    "username": "somchai",
    "full_name": "สมชาย ใจดีที่สุด",
    "phone": "0811111111",
    "role": "customer",
    "is_active": true,
    "updated_at": "2026-09-06T12:00:00Z"
  }
}
```

---

### 7.14 `DELETE /api/v1/users/{id}`

**หน้าที่:** ลบผู้ใช้แบบ soft delete (ตั้ง `deleted_at`) และ revoke refresh token ทั้งหมดของผู้ใช้นั้น

| ส่วน | รายละเอียด |
|---|---|
| สิทธิ์ | Admin |
| Path param | `id` (UUID, required) |

**Response codes:** `200` สำเร็จ (หรือ `204`) · `401` `UNAUTHENTICATED` · `403` `FORBIDDEN` (รวมกรณีพยายามลบตัวเอง) · `404` `NOT_FOUND`

**Response `200`**

```json
{ "success": true, "data": { "message": "ลบผู้ใช้เรียบร้อย" } }
```

---

### 7.15 `PATCH /api/v1/users/{id}/role`

**หน้าที่:** เปลี่ยนบทบาทของผู้ใช้ (เช่น เลื่อน Customer → Staff)

| ส่วน | รายละเอียด |
|---|---|
| สิทธิ์ | Admin |
| Path param | `id` (UUID, required) |
| Body (JSON) | `role` (string, required: `admin` / `staff` / `customer`) |

**Request**

```json
{ "role": "staff" }
```

**Response codes:** `200` สำเร็จ · `400` `VALIDATION_ERROR` (role ไม่ถูกต้อง) · `401` `UNAUTHENTICATED` · `403` `FORBIDDEN` · `404` `NOT_FOUND`

**Response `200`**

```json
{
  "success": true,
  "data": {
    "id": "9b1c7c2e-3d4a-4a1b-8f0e-2a1b3c4d5e6f",
    "role": "staff",
    "updated_at": "2026-09-06T12:30:00Z"
  }
}
```

---

### 7.16 `PATCH /api/v1/users/{id}/status`

**หน้าที่:** เปิด / ปิดการใช้งานบัญชีผู้ใช้ — ปิดแล้วจะล็อกอินไม่ได้และ token เดิมใช้ผ่าน `/auth/verify` ไม่ได้ พร้อม revoke refresh token ทั้งหมด

| ส่วน | รายละเอียด |
|---|---|
| สิทธิ์ | Admin |
| Path param | `id` (UUID, required) |
| Body (JSON) | `is_active` (boolean, required) |

**Request**

```json
{ "is_active": false }
```

**Response codes:** `200` สำเร็จ · `400` `VALIDATION_ERROR` · `401` `UNAUTHENTICATED` · `403` `FORBIDDEN` (รวมกรณีปิดบัญชีตัวเอง) · `404` `NOT_FOUND`

**Response `200`**

```json
{
  "success": true,
  "data": {
    "id": "9b1c7c2e-3d4a-4a1b-8f0e-2a1b3c4d5e6f",
    "is_active": false,
    "updated_at": "2026-09-06T12:45:00Z"
  }
}
```

---

### 7.17 `GET /api/v1/users/{id}/login-logs`

**หน้าที่:** Admin ดูประวัติการเข้าสู่ระบบของผู้ใช้รายที่ระบุ

| ส่วน | รายละเอียด |
|---|---|
| สิทธิ์ | Admin |
| Path param | `id` (UUID, required) |
| Query | `page`, `limit`, `success` (optional) |

**Response codes:** `200` สำเร็จ · `401` `UNAUTHENTICATED` · `403` `FORBIDDEN` · `404` `NOT_FOUND`

**Response `200`**

```json
{
  "success": true,
  "data": [
    {
      "id": 2048,
      "email_attempted": "somchai@example.com",
      "success": true,
      "ip_address": "203.0.113.10",
      "user_agent": "PostmanRuntime/7.37",
      "created_at": "2026-09-06T09:00:00Z"
    }
  ],
  "meta": { "page": 1, "limit": 20, "total": 1, "total_pages": 1 }
}
```

---

### 7.18 `GET /api/v1/roles`

**หน้าที่:** รายการบทบาททั้งหมดพร้อมคำอธิบาย (ใช้เติม dropdown ตอนสร้าง/แก้ผู้ใช้)

| ส่วน | รายละเอียด |
|---|---|
| สิทธิ์ | Admin, Staff |
| Parameter | — |

**Response codes:** `200` สำเร็จ · `401` `UNAUTHENTICATED` · `403` `FORBIDDEN`

**Response `200`**

```json
{
  "success": true,
  "data": [
    { "id": 1, "name": "admin",    "description": "ผู้ดูแลระบบ จัดการผู้ใช้ บทบาท สินค้า และการเช่าทั้งหมด" },
    { "id": 2, "name": "staff",    "description": "เจ้าหน้าที่ จัดการสินค้า สร้างรายการเช่า และบันทึกการคืน" },
    { "id": 3, "name": "customer", "description": "ลูกค้า ดูสินค้า สร้างคำขอเช่า และดูประวัติการเช่าของตนเอง" }
  ]
}
```

---

### 7.19 `GET /health`

**หน้าที่:** ตรวจสอบว่าบริการและการเชื่อมต่อฐานข้อมูลพร้อมทำงาน (ใช้กับ Docker healthcheck / API gateway)

| ส่วน | รายละเอียด |
|---|---|
| สิทธิ์ | Public |
| Parameter | — |

**Response codes:** `200` พร้อมใช้งาน · `503` ฐานข้อมูลเชื่อมต่อไม่ได้

**Response `200`**

```json
{ "success": true, "data": { "status": "ok", "service": "user-service", "db": "up", "time": "2026-09-06T13:00:00Z" } }
```

**Response `503`**

```json
{ "success": false, "error": { "code": "INTERNAL_ERROR", "message": "database unavailable", "details": null } }
```

---

### 7.20 `GET /api/v1/me/sessions`

**หน้าที่:** ดูรายการ session (refresh token) ที่ยัง active ของตนเอง — ช่วยให้ผู้ใช้เห็นว่ามีการล็อกอินค้างจากอุปกรณ์ใดบ้าง

| ส่วน | รายละเอียด |
|---|---|
| สิทธิ์ | Authenticated |
| Parameter | — |

**Response codes:** `200` สำเร็จ · `401` `UNAUTHENTICATED`

**Response `200`**

```json
{
  "success": true,
  "data": [
    {
      "id": "5c6d7e8f-9a0b-1c2d-3e4f-5a6b7c8d9e0f",
      "ip_address": "203.0.113.10",
      "user_agent": "Mozilla/5.0 ...",
      "created_at": "2026-09-06T09:00:00Z",
      "expires_at": "2026-09-13T09:00:00Z"
    }
  ]
}
```

---

## 8. JWT และความปลอดภัย

### 8.1 โครงสร้าง Access Token (JWT, HS256)

**Header**

```json
{ "alg": "HS256", "typ": "JWT" }
```

**Payload (claims)**

```json
{
  "sub": "9b1c7c2e-3d4a-4a1b-8f0e-2a1b3c4d5e6f",
  "email": "somchai@example.com",
  "username": "somchai",
  "role": "customer",
  "iat": 1757148000,
  "exp": 1757148900,
  "jti": "3b9d1f0e-..."
}
```

- เซ็นด้วย `JWT_SECRET` (อย่างน้อย 32 ตัวอักษร) — บริการอื่นใช้ secret เดียวกันตรวจลายเซ็นเองได้
- อายุ 15 นาที เพื่อจำกัดความเสียหายหากรั่ว

### 8.2 Refresh Token

- เป็น **opaque random string** (32 ไบต์ → 64 hex) ไม่ใช่ JWT
- เก็บใน DB เฉพาะ **SHA-256 hash** (`token_hash`) — ต่อให้ DB รั่วก็นำ token ไปใช้ไม่ได้
- อายุ 7 วัน
- **Rotation:** ทุกครั้งที่เรียก `/auth/refresh` จะ revoke token เดิมและออกใหม่
- **Reuse detection (ทางเลือกเสริม):** ถ้าพบการใช้ token ที่ถูก revoke ไปแล้ว ให้ revoke token ทั้งสายของผู้ใช้

### 8.3 การเก็บรหัสผ่าน

- hash ด้วย `bcrypt` cost 12 ก่อนบันทึก ไม่เก็บ plaintext
- เกณฑ์รหัสผ่าน: ยาว ≥ 8, มีตัวอักษรและตัวเลขอย่างน้อยอย่างละ 1

### 8.4 Middleware

| Middleware | หน้าที่ |
|---|---|
| `RequireAuth` | อ่าน `Authorization: Bearer`, ตรวจลายเซ็น + `exp`, ใส่ `user_id`, `role` ลง context; ผิด → `401 UNAUTHENTICATED` |
| `RequireRole(roles...)` | เทียบ `role` ใน context กับรายการที่อนุญาต; ไม่ตรง → `403 FORBIDDEN` |
| `RequireInternalKey` | ตรวจ header `X-Internal-Key` เทียบ `INTERNAL_API_KEY`; ผิด → `403 FORBIDDEN` |
| `RateLimit` (เฉพาะ `/auth/login`, `/auth/register`) | จำกัดจำนวนคำขอต่อ IP (เช่น 10 ครั้ง/นาที) → `429 TOO_MANY_REQUESTS` |

### 8.5 Login Log

บันทึกทุกครั้งที่เรียก `/auth/login` ทั้งกรณีสำเร็จและล้มเหลว: `email_attempted`, `success`, `ip_address`, `user_agent`, `user_id` (ถ้าจับคู่อีเมลได้)

### 8.6 หมายเหตุความปลอดภัยอื่น

- เปิด CORS เฉพาะ origin ของ frontend ที่กำหนด
- ทุก endpoint รับ-ส่งผ่าน HTTPS ใน production (จัดการที่ reverse proxy / gateway)
- ไม่คืน `password_hash` ใน response ใด ๆ
- ข้อความ error ตอน login ไม่ระบุว่าอีเมลไม่มีอยู่หรือรหัสผ่านผิด (กัน user enumeration)

---

## 9. ขั้นตอนการรันโปรเจกต์

### 9.1 เตรียม environment

```bash
cp .env.example .env
# แก้ค่า JWT_SECRET, INTERNAL_API_KEY ให้เป็นค่าลับของตัวเอง
```

### 9.2 รันด้วย Docker Compose

```bash
docker compose up -d --build
```

### 9.3 รัน migration (กรณีไม่ได้รันอัตโนมัติใน main.go)

```bash
docker compose exec user-service migrate \
  -path=/migrations \
  -database "postgres://user_service:secret@user-db:5432/user_db?sslmode=disable" up
```

### 9.4 ทดสอบ

```bash
# health
curl http://localhost:8081/health

# สมัครสมาชิก
curl -X POST http://localhost:8081/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"somchai@example.com","username":"somchai","password":"Passw0rd123","full_name":"สมชาย ใจดี"}'

# เข้าสู่ระบบ
curl -X POST http://localhost:8081/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"somchai@example.com","password":"Passw0rd123"}'

# ดูโปรไฟล์ตนเอง (แทน <token> ด้วย access_token ที่ได้)
curl http://localhost:8081/api/v1/me \
  -H "Authorization: Bearer <token>"

# ตรวจ token จากบริการภายใน
curl -X POST http://localhost:8081/api/v1/auth/verify \
  -H "Content-Type: application/json" \
  -H "X-Internal-Key: internal-shared-key" \
  -d '{"token":"<token>"}'
```

### 9.5 หยุดและล้างข้อมูล

```bash
docker compose down          # หยุด
docker compose down -v        # หยุด + ลบ volume ฐานข้อมูล
```

---

## ภาคผนวก: สิ่งที่บริการอื่นต้องรู้ (Interface Contract)

ให้เพื่อนร่วมทีม (Product / Rental service) ทำตามนี้:

1. **ตรวจ JWT เอง** — ใช้ `JWT_SECRET` เดียวกัน, อัลกอริทึม `HS256`, เช็ค `exp`
2. อ่าน `sub` = user id, `role` = บทบาท จาก claim
3. บังคับสิทธิ์ในฝั่งตนเอง เช่น การสร้างสินค้าต้อง `role in (admin, staff)`
4. ถ้าต้องการความมั่นใจว่าบัญชียัง active — เรียก `POST /api/v1/auth/verify` พร้อม header `X-Internal-Key`
5. ห้ามแก้ข้อมูลในตาราง `users` โดยตรง — ให้เรียกผ่าน API ของโมดูลนี้เท่านั้น

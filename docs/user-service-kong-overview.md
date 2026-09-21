# สรุปงานที่รับผิดชอบ: user-service + Kong API Gateway

> ผู้จัดทำ: สุรเชษฐ์ สีสา (67114540583) · โครงงาน: Equipment Rental System
> เอกสารนี้อธิบายว่า "มีอะไรบ้าง" และ "ทำงานอย่างไร" สำหรับสองส่วนที่รับผิดชอบ —
> ระบบจัดการผู้ใช้งาน (JWT & RBAC) และ Kong API Gateway

---

## 1. ภาพรวม

รับผิดชอบ 2 ส่วนในระบบ:

1. **user-service** — service เดียวในระบบที่จัดการเรื่องผู้ใช้ทั้งหมด: สมัครสมาชิก,
   เข้าสู่ระบบ, ออก JWT token, จัดการสิทธิ์ (RBAC), จัดการบัญชีผู้ใช้โดย admin
2. **Kong API Gateway** — ตัวกลางที่ยืนหน้าทั้ง 3 service ของทีม (user/product/rental)
   ทำหน้าที่ตรวจสอบ JWT token ให้ **ก่อน** ที่ request จะไปถึง service ปลายทาง

ทั้งสองส่วนนี้เชื่อมกัน: user-service เป็นคนเดียวที่ **ออก** token, Kong เป็นคนเดียวที่
**ตรวจ** token — ทั้งระบบ (รวม product/rental ที่ยังไม่มีโค้ด) เชื่อ token ที่ user-service
เซ็นและ Kong รับรองแล้วเท่านั้น

---

## 2. สถาปัตยกรรมรวม

```
Client
  │
  ▼
Kong (:8000)  ◄── DB-less, อ่าน config จาก deploy/kong/kong.yml ไฟล์เดียว
  │
  ├─► user-service (:8081) ──► user-db (PostgreSQL)
  ├─► product-service (:8082)   [ยังไม่มีโค้ด — ทีมอื่นทำ]
  └─► rental-service (:8083)    [ยังไม่มีโค้ด — ทีมอื่นทำ]
```

**ทำไมต้องมี Kong:** ถ้าไม่มี Kong แต่ละ service ต้อง verify JWT signature เอง —
หมายความว่าต้องถือ `JWT_SECRET` เหมือนกันทั้ง 3 service (secret กระจายไปหลายที่ เสี่ยงหลุด
ง่ายขึ้น) และต้องเขียน logic verify ซ้ำ 3 รอบ พอมี Kong คั่นกลาง มีแค่ user-service กับ Kong
เท่านั้นที่ถือ secret, product/rental แค่ "อ่าน" claims จาก token ที่ Kong ยืนยันแล้วว่าแท้จริง

---

## 3. Database Schema (user-db)

4 ตาราง จัดการโดย GORM (migration แบบ SQL ตรงๆ ใน `user-service/migrations/`):

| ตาราง | ใช้เก็บอะไร | field สำคัญ |
|---|---|---|
| `roles` | รายชื่อ role (seed ไว้ตั้งแต่ migration: admin/staff/customer) | `id` (int16), `name` |
| `users` | บัญชีผู้ใช้ | `id` (UUID), `email`/`username` (unique), `password_hash` (bcrypt), `role_id` (FK), `is_active`, `deleted_at` (soft delete) |
| `refresh_tokens` | session สำหรับต่ออายุ login | `token_hash` (SHA-256 ของ token จริง — **ไม่เก็บ token ดิบ**), `expires_at`, `revoked_at` |
| `login_logs` | ประวัติการพยายาม login ทุกครั้ง (สำเร็จ/ไม่สำเร็จ) | `user_id` (nullable — ถ้า login ผิด email อาจไม่รู้ user), `success`, `ip_address` |

**จุดที่ตั้งใจออกแบบเป็นพิเศษ:**
- `password_hash` ไม่เคยถูกส่งออกไปใน response ใดๆ (DTO ทุกตัวไม่มี field นี้)
- `refresh_tokens.token_hash` unique — เก็บแค่ hash กัน token หลุดจาก DB dump แล้วใช้ต่อได้
- `users.deleted_at` เป็น partial unique index คู่กับ `email`/`username` (ดูหัวข้อ 8) —
  ลบ user แล้วสมัครซ้ำด้วย email เดิมได้ทันที ไม่ต้องรอใคร hard-delete

---

## 4. โครงสร้างโค้ด (layered architecture)

```
user-service/internal/
├── model/       ข้อมูล + GORM tag ตรงกับตาราง (user.go, role.go, refresh_token.go, login_log.go)
├── repository/  คุยกับ DB อย่างเดียว ไม่มี business logic (user_repo.go, ...)
├── service/     business logic ทั้งหมดอยู่ที่นี่ (auth_service.go, user_service.go, token_service.go)
├── handler/     รับ HTTP request, เรียก service, แปลงผลลัพธ์เป็น JSON response
├── middleware/  ทำงานก่อนถึง handler (auth.go=ตรวจ JWT, rbac.go=ตรวจ role, cors.go, internal.go=ตรวจ X-Internal-Key)
├── dto/         โครงสร้าง request/response + validation tag (auth_dto.go, user_dto.go)
├── router/      ผูก path เข้ากับ handler + middleware (router.go — ดูภาพรวม endpoint ทั้งหมดได้ที่นี่)
├── config/      อ่าน environment variable
└── db/          เปิด connection + รัน migration อัตโนมัติตอน start
```

แนวคิด: แต่ละ layer รู้แค่ layer ถัดไป — handler ไม่คุยกับ DB ตรง, repository ไม่รู้เรื่อง
HTTP — ทำให้ test แต่ละส่วนแยกกันได้ (เช่น test service logic โดยไม่ต้องเปิด DB จริง)

---

## 5. JWT Authentication ทำงานอย่างไร

### 5.1 Register → Login

```
POST /auth/register  →  บันทึก user ใหม่ (bcrypt hash password, role default = customer)
POST /auth/login      →  ตรวจ email+password ถูกต้อง
                       →  ออก access_token (JWT, อายุ 15 นาที)
                       →  ออก refresh_token (string สุ่ม 64 ตัวอักษร, อายุ 7 วัน, เก็บแค่ hash ใน DB)
                       →  บันทึกลง login_logs ทุกครั้ง (สำเร็จหรือไม่ก็บันทึก)
```

### 5.2 รูปแบบ Access Token (JWT)

```json
{
  "sub": "<user UUID>",
  "email": "...",
  "username": "...",
  "role": "customer",
  "iss": "equipment-rental-system",
  "iat": 1757148000,
  "exp": 1757148900,
  "jti": "<uuid สุ่มทุกครั้ง>"
}
```

เซ็นด้วย HS256 + `JWT_SECRET` — claim `iss` (issuer) เป็นค่าคงที่เดียวกันทุก token ใช้บอก
Kong ว่าจะไปหา secret ไหนมาตรวจ (ไม่เกี่ยวกับตัว user คนไหนเลย)

### 5.3 Refresh Token

เป็น **opaque string** (สุ่มมาเฉยๆ ไม่ใช่ JWT) เหตุผล: ไม่ต้อง decode ได้ ต้องมี DB มา
เทียบเท่านั้น ทำให้ **revoke ได้จริง** (JWT ธรรมดาถ้าออกไปแล้ว revoke ไม่ได้จนกว่าจะหมดอายุ
เอง) — ตอน login/register จะได้ token ดิบกลับไป แต่ DB เก็บแค่ SHA-256 hash ของมัน

`POST /auth/refresh` เอา refresh token มาแลก access token ใหม่ + **หมุน refresh token ใหม่ทุกครั้ง** (token เก่าใช้ไม่ได้อีก) — ป้องกัน token หลุดแล้วถูกใช้ซ้ำเงียบๆ

### 5.4 Logout / เปลี่ยนรหัสผ่าน / เปลี่ยน role

ทุกกรณีนี้จะ **revoke session** (ตั้ง `revoked_at` ใน `refresh_tokens`) — logout revoke
แค่ session ที่ logout, เปลี่ยนรหัสผ่าน/ถูกเปลี่ยน role โดย admin จะ revoke session **ทั้งหมด**
ของ user คนนั้น (บังคับให้ login ใหม่ ป้องกันกรณีรหัสผ่านหลุดแล้วยังมี session เก่าค้างอยู่)

---

## 6. RBAC (Role-Based Access Control) ทำงานอย่างไร

3 role: `admin`, `staff`, `customer` — เก็บใน token claim `role` ตรงๆ ไม่ต้อง query DB ซ้ำ
ทุก request

Middleware 2 ชั้น ทำงานเรียงกัน (ดู `router.go`):

```go
v1.GET("/users", authMW, middleware.RequireRole("admin"), adminUserHandler.List)
//                ↑ ชั้น 1: token ถูกต้องไหม        ↑ ชั้น 2: role ตรงไหม
```

1. **`authMW`** (`middleware/auth.go`) — decode + verify JWT signature/expiry, ถ้าผ่าน
   ใส่ user id/role ลง context ให้ handler ใช้ต่อ, ถ้าไม่ผ่านตอบ `401` ทันที
2. **`RequireRole(...roles)`** (`middleware/rbac.go`) — เช็คว่า role ใน token ตรงกับที่
   endpoint นี้อนุญาตไหม ถ้าไม่ตรงตอบ `403`

Endpoint ไหนไม่ใส่ `RequireRole` = แค่ต้อง login (role อะไรก็ได้) เช่น `/me`

---

## 7. Kong API Gateway ทำงานอย่างไร

### 7.1 DB-less mode คืออะไร

ปกติ Kong ต้องมี Postgres ของตัวเองเก็บ config (services/routes/plugins) แบบ dynamic
แต่โครงงานนี้เล็ก ไม่คุ้มที่จะมี DB เพิ่มอีกตัว เลยใช้โหมด **declarative** แทน — เขียน config
ทั้งหมดลงไฟล์เดียว [`deploy/kong/kong.yml`](../deploy/kong/kong.yml) แล้วให้ Kong โหลดไฟล์
นี้ตอน start (`KONG_DATABASE=off` + `KONG_DECLARATIVE_CONFIG=/kong/kong.yml`)

### 7.2 Kong ตรวจ JWT ได้อย่างไรโดยไม่ต้องเรียก user-service

Kong มี plugin ชื่อ `jwt` ในตัว — ทำงานแบบนี้:

1. ที่ `kong.yml` ประกาศ **consumer** ชื่อ `rental-system-issuer` พร้อม **credential**:
   key = `equipment-rental-system` (ต้องตรงกับ claim `iss` ใน token), secret = ค่าเดียวกับ
   `JWT_SECRET` ที่ user-service ใช้เซ็น
2. Request เข้ามาพร้อม `Authorization: Bearer <token>` — plugin `jwt` จะ:
   - decode header ของ token หา claim `iss` → เอาไปจับคู่กับ consumer/credential ข้อ 1
   - **verify signature** ด้วย secret ที่จับคู่ได้ (ไม่ใช่แค่ decode เฉยๆ — เช็คว่าไม่มีใครปลอม)
   - **verify `exp`** (ต้องตั้ง `claims_to_verify: ["exp"]` ไว้ในไฟล์ ไม่งั้น Kong จะไม่เช็ค
     ว่า token หมดอายุหรือยัง — จุดนี้เคยเป็นบั๊กจริงตอน implement รอบแรก แก้แล้ว)
   - ผ่านหมด → forward request ต่อไปที่ service จริง (header `Authorization` ยังอยู่)
   - ไม่ผ่าน → ตอบ `401` **ที่ Kong เลย** ไม่ถึง service ปลายทางด้วยซ้ำ

Config พิเศษอีกตัวที่ตั้งไว้: `run_on_preflight: false` — ป้องกันไม่ให้ Kong ไปบล็อก
`OPTIONS` request (browser preflight สำหรับ CORS) เพราะ preflight ไม่มี `Authorization`
header อยู่แล้วตามมาตรฐาน ถ้าไม่ตั้งอันนี้ Kong จะเข้าใจผิดว่าเป็น request ที่ไม่มี token
แล้วปฏิเสธ ทำให้หน้าเว็บเรียก API ผ่าน Kong ไม่ได้เลย

### 7.3 Route ไหนต้องมี token, ไหนไม่ต้อง

`kong.yml` แบ่ง route ของ user-service เป็น 3 กลุ่ม:

| กลุ่ม route | มี `jwt` plugin ไหม | ตัวอย่าง path |
|---|---|---|
| public | ไม่มี | `/auth/register`, `/auth/login`, `/health` |
| internal | ไม่มี (แต่ตัว service เองเช็ค `X-Internal-Key` แทน) | `/auth/verify` |
| protected | มี | `/me`, `/users`, `/roles` |

### 7.4 product-service / rental-service จะเห็นอะไร

เมื่อ 2 service นี้มีโค้ดแล้วและผ่าน Kong: ตัว service **ไม่ต้องถือ `JWT_SECRET` เลย**
เพราะ Kong verify signature ให้เสร็จแล้วก่อนถึงตัวเอง แค่ decode payload (base64) อ่าน
`role`/`sub` เอาไปทำ RBAC ต่อ — ไม่ต้อง verify signature ซ้ำ (Kong การันตีมาแล้วว่า token
แท้จริง)

---

## 8. บั๊กที่เจอจริงระหว่างพัฒนา และวิธีแก้ (สรุปสั้น)

รายการนี้ไม่ใช่ทฤษฎี — เป็นสิ่งที่เจอจริงระหว่าง implement แล้วแก้ไปแล้วทั้งหมด เก็บไว้เป็น
บันทึกว่าทำไมโค้ดถึงเป็นแบบนี้:

| บั๊ก | จุดที่เจอ | วิธีแก้ |
|---|---|---|
| SQL injection ผ่าน query param `sort`/`order` ของ `GET /users` | ใช้ค่าจาก URL ต่อ SQL ตรงๆ | จำกัดให้เลือกได้แค่ column ที่ allowlist ไว้ + asc/desc เท่านั้น |
| Race condition ตอน register พร้อมกัน 2 คน อีเมลเดียวกัน | เช็คซ้ำก่อน insert แล้วยัง insert ซ้ำได้ | ใช้ DB unique constraint เป็นตัวตัดสินจริง จับ error `ErrDuplicatedKey` แล้ว re-query |
| หน้า list พัง (หารด้วย 0) ตอนส่ง `?limit=0` | คำนวณ `total_pages` โดยไม่ clamp ค่า `limit` ก่อน | validate/clamp `page`/`limit` ในทุก handler ก่อนใช้ |
| ลืมตั้ง `JWT_SECRET` ก็ยัง start service ได้ (fail-open) | `config.Load()` ไม่เช็คว่า secret ว่างไหม | บังคับ error ตอน start ถ้า `JWT_SECRET`/`INTERNAL_API_KEY` ว่างหรือสั้นเกินไป |
| ลบ user แล้วเอา email เดิมไปสมัครใหม่ไม่ได้ | unique constraint ชนกับ record ที่ soft-delete ไปแล้ว | เปลี่ยนเป็น partial unique index (`WHERE deleted_at IS NULL`) |
| Kong route `/health` ชนกันทั้ง 3 service (route ผิดไปที่ service ที่ไม่มีจริง) | ประกาศ path เดียวกันซ้ำ 3 ที่ใน kong.yml | ลบ route health ของ product/rental ออกก่อน (ยังไม่มี service จริง) — ต้องตั้งใหม่เป็น path ไม่ซ้ำตอนมีโค้ดจริง |
| **Kong ไม่เช็คว่า token หมดอายุหรือยัง** (ปล่อยผ่านหมด) | `jwt` plugin ไม่ได้ตั้ง `claims_to_verify` | เพิ่ม `claims_to_verify: ["exp"]` — ถ้าไม่แก้ พอ product/rental ทำแบบ decode-only ตาม contract จะกลายเป็นช่องโหว่จริง |
| CORS preflight (`OPTIONS`) โดน Kong ปฏิเสธ | `jwt` plugin เช็ค preflight request ด้วย (ค่า default) | ตั้ง `run_on_preflight: false` |

---

## 9. การทดสอบ

| ระดับ | คำสั่ง | ทดสอบอะไร |
|---|---|---|
| Unit test | `cd user-service && go test ./...` | logic ของแต่ละ layer แยกกัน (ใช้ SQLite จำลอง DB ในบาง test) |
| Integration (Docker) | `docker compose up -d --build` แล้ว curl endpoint ตรง | service คุยกับ DB จริงได้ |
| End-to-end ผ่าน Kong | `./deploy/kong/smoke-test.sh` | register→login→ใช้ token จริงผ่าน Kong, token ปลอม/ไม่มี token/หมดอายุถูกปฏิเสธจริงที่ชั้น Kong |

---

## 10. สรุป endpoint ทั้งหมด

ดูตารางแบบละเอียด (method, path, สิทธิ์, body ที่ต้องส่ง) ได้ที่
[user-service/README.md](../user-service/README.md) — มี 20 endpoints ทั้งหมด แบ่งเป็น
public (register/login/refresh/health), internal (verify), และ protected (ที่เหลือทั้งหมด)

ดู path ที่ Kong รู้จักทั้งหมด (รวมของ product/rental ที่ประกาศล่วงหน้าไว้) ที่
[deploy/kong/README.md](../deploy/kong/README.md)

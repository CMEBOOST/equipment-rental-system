# Equipment Rental System — เอกสารอธิบายโปรเจคแบบละเอียด

> เอกสารนี้สรุปภาพรวมทั้งระบบสำหรับผู้ที่ต้องการเข้าใจโปรเจคอย่างละเอียด — สถาปัตยกรรม,
> การทำงานของแต่ละ service, ฐานข้อมูล, ระบบยืนยันตัวตน/สิทธิ์, และวิธีรัน/ทดสอบระบบ
> อ้างอิงจาก [README.md](../README.md), [CONTRACT.md](../CONTRACT.md) และซอร์สโค้ดจริงในแต่ละ service

---

## 1. ภาพรวมโปรเจค

**Equipment Rental System** คือระบบเช่าอุปกรณ์แบบ **Backend-only** (ไม่มีหน้าเว็บ/แอป
เป็นของตัวเอง เปิดให้เรียกผ่าน REST API เท่านั้น) พัฒนาด้วย **Go + PostgreSQL** ตาม
สถาปัตยกรรม **Microservices** โดยแบ่งเป็น 3 service อิสระต่อกัน แต่ละ service มีฐานข้อมูล
เป็นของตัวเอง (database-per-service) และมี **Kong API Gateway** เป็นประตูทางเข้าเดียว (single
entry point) ให้ client เรียกเข้ามา

เป็นโปรเจคของนักศึกษา 3 คนทำร่วมกัน แบ่งความรับผิดชอบตาม service:

| สมาชิก | รหัสนักศึกษา | รับผิดชอบ | โฟลเดอร์ |
|---|---|---|---|
| สุรเชษฐ์ สีสา | 67114540583 | ระบบจัดการผู้ใช้งาน (JWT & RBAC) + Kong Gateway | `user-service/`, `deploy/kong/` |
| เอกพล แรกเรียง | 67114540666 | ระบบจัดการสินค้า | `product-service/` |
| วิษณุพงศ์ บัวเขียว | 67114540509 | ระบบจัดการการเช่า | `rental-service/` |

ทั้ง 3 service เสร็จสมบูรณ์แล้ว (มีโค้ดจริง ทดสอบผ่าน มี endpoint ครบตามที่ตกลงกันใน
[CONTRACT.md](../CONTRACT.md))

### คุณสมบัติหลักของระบบ

- **จัดการผู้ใช้งานและสิทธิ์** — สมัคร/เข้าสู่ระบบด้วย JWT (access + refresh token), RBAC
  3 ระดับ (`admin` / `staff` / `customer`), จัดการบัญชี/บทบาท/เปิด-ปิดผู้ใช้, ดูประวัติการ
  เข้าสู่ระบบ
- **จัดการสินค้า** — CRUD สินค้าและหมวดหมู่, ค้นหา/กรอง/แบ่งหน้า, สถานะสินค้า 3 แบบ
  (`available` / `rented` / `maintenance`) พร้อม atomic status update กัน race condition
  ตอนมีคนเช่าสินค้าชิ้นเดียวกันพร้อมกัน
- **จัดการการเช่า** — ลูกค้าขอเช่าเอง หรือ staff สร้างแทนได้, อนุมัติคำขอ, บันทึกการคืนสินค้า,
  คำนวณราคาอัตโนมัติจากจำนวนวัน, ผูกสถานะสินค้าที่ product-service ให้ตรงกับสถานะการเช่าเสมอ
- **API Gateway (Kong)** — ตรวจลายเซ็น/วันหมดอายุของ JWT ให้ทุก protected route ก่อนส่งต่อ
  ไปยัง service ปลายทาง ทำให้ product-service และ rental-service ไม่ต้องถือ secret ร่วมกัน
- **Docker Compose** — รันได้ทั้งระบบด้วยคำสั่งเดียว รวม migration อัตโนมัติและ seed บัญชี
  admin/staff เริ่มต้นให้พร้อมใช้งาน
- **Swagger UI** — เอกสาร API แบบ interactive ทดลองยิง request จริงผ่าน Kong ได้ทันที

---

## 2. สถาปัตยกรรมระบบ

```
                        ┌────────────────────────────┐
   Client  ───────────► │   Kong API Gateway (:8000)  │
                        │   (DB-less / declarative)   │
                        └───────────┬──────────────────┘
                                    │ ตรวจ JWT signature + exp
                                    │ ให้ทุก protected route
              ┌─────────────────────┼─────────────────────┐
              ▼                     ▼                     ▼
   ┌────────────────────┐ ┌─────────────────────┐ ┌─────────────────────┐
   │   user-service       │ │  product-service     │ │  rental-service      │
   │   :8081               │ │  :8082                │ │  :8083                │
   │   ถือ JWT_SECRET       │ │  decode-only          │ │  decode-only          │
   │   ออก token เอง        │ │  ไม่ถือ secret          │ │  ไม่ถือ secret          │
   └─────────┬───────────┘ └─────────┬───────────┘ └─────────┬───────────┘
             │                       │                       │
             ▼                       ▼                       ▼
       ┌───────────┐           ┌───────────┐           ┌───────────┐
       │  user-db   │           │ product-db │           │ rental-db  │
       │ (Postgres) │           │ (Postgres) │           │ (Postgres) │
       └───────────┘           └───────────┘           └───────────┘

  การเรียกข้ามservice (ไม่ผ่าน Kong เรียกตรงข้าม container name):
  rental-service ──► product-service   : ตรวจสินค้า / เปลี่ยนสถานะสินค้า
  rental-service ──► user-service      : POST /auth/verify (ยืนยันบัญชียัง active)
```

### หลักการออกแบบสำคัญ

1. **Client เรียกผ่าน Kong (`:8000`) เท่านั้น** สำหรับ flow ปกติ — พอร์ต 8081-8083 ยังเปิด
   map ไว้ในทางไว้ debug ตรง ๆ เท่านั้น ไม่ใช่ทางที่ client ควรใช้
2. **user-service เท่านั้นที่ถือ `JWT_SECRET`** และเป็นคนออก token — Kong ตรวจลายเซ็น/วัน
   หมดอายุแทน product-service และ rental-service (ผ่าน Kong `jwt` plugin) ทั้งสอง service นี้
   แค่ **decode payload** อ่าน claims (`sub`, `role`, `email`) เอาไปใช้บังคับสิทธิ์ ไม่ต้อง
   verify signature ซ้ำ และไม่ต้องถือ secret ร่วมกันเลย
3. **แต่ละ service มีฐานข้อมูล PostgreSQL เป็นของตัวเอง** — ไม่มี foreign key ข้าม database
   อ้างอิงกันด้วยค่า UUID เปล่า ๆ เท่านั้น (เช่น `rentals.user_id`, `rentals.product_id`)
   ถ้าต้องการข้อมูลของ service อื่นต้องเรียกผ่าน HTTP API เท่านั้น
4. **Kong รันแบบ DB-less** — ไม่มี Kong Postgres/Admin API, config ทั้งหมดอยู่ในไฟล์เดียว
   [`deploy/kong/kong.yml`](../deploy/kong/kong.yml) (declarative config)
5. **Kong ไม่ทำ RBAC** — ตรวจแค่ว่า token valid หรือไม่ ส่วนสิทธิ์ตาม role (`admin`/`staff`/
   `customer`) แต่ละ service บังคับเองฝั่งใน service นั้น ๆ

---

## 3. เทคโนโลยีที่ใช้ (Tech Stack)

| หมวด | เทคโนโลยี |
|---|---|
| ภาษา | Go |
| Web framework | [Gin](https://github.com/gin-gonic/gin) |
| ORM | [GORM](https://gorm.io) (postgres driver) |
| ฐานข้อมูล | PostgreSQL 16 (`postgres:16-alpine`) — คนละ container ต่อ service |
| Migration | [golang-migrate](https://github.com/golang-migrate/migrate) — raw SQL, รันอัตโนมัติตอน service start |
| API Gateway | [Kong](https://konghq.com/) 3.6 (DB-less / declarative mode) |
| Auth | JWT (HS256), bcrypt password hashing (cost 12) |
| Infra | Docker / Docker Compose |
| API Docs | OpenAPI 3 spec (`docs/openapi/*.yaml`) เสิร์ฟผ่าน Swagger UI (`swaggerapi/swagger-ui`) |
| Test (unit/integration) | ใช้ SQLite ในหน่วยความจำ ([glebarez/sqlite](https://github.com/glebarez/sqlite), pure Go ไม่ต้องมี CGO) แทน Postgres จริง — ไม่ต้องมี DB รันอยู่ตอนเทส |
| Test (end-to-end) | shell script ยิงผ่าน Kong จริง (`deploy/kong/*.sh`) |

---

## 4. โครงสร้างโปรเจกต์ (Monorepo)

```
equipment-rental-system/
├── user-service/        # :8081 → user-db     — auth, users, roles (20 endpoints)
│   ├── cmd/api/           # main.go — entry point
│   ├── internal/
│   │   ├── config/         # อ่าน env vars
│   │   ├── db/              # เชื่อมต่อ DB + รัน migration
│   │   ├── model/            # struct ตาราง
│   │   ├── dto/               # request body + validation
│   │   ├── repository/        # query DB
│   │   ├── service/            # business logic
│   │   ├── middleware/         # JWT, RBAC, internal key, CORS
│   │   ├── handler/             # HTTP handler
│   │   └── router/               # ผูก route
│   └── migrations/         # golang-migrate .sql
├── product-service/     # :8082 → product-db  — สินค้าและหมวดหมู่ (โครงสร้างเดียวกับ user-service)
├── rental-service/      # :8083 → rental-db   — คำขอเช่า/อนุมัติ/คืนสินค้า
│   └── internal/client/    # HTTP client เรียก product-service / user-service
├── deploy/
│   └── kong/               # kong.yml + smoke test scripts + README
├── docs/
│   ├── openapi/             # OpenAPI spec ของทั้ง 3 service — เสิร์ฟเป็น Swagger UI ผ่าน Kong ที่ /docs
│   ├── user-management-service-design.md
│   └── superpowers/        # เอกสารออกแบบ/แผนการ implement
├── CONTRACT.md             # ข้อตกลงระหว่าง service (พอร์ต, JWT, response format, RBAC)
├── .env.example
└── docker-compose.yml      # รันครบทั้ง 3 service + Kong + Swagger UI ด้วยคำสั่งเดียว
```

---

## 5. Authentication (JWT)

### 5.1 ใครออก token

**เฉพาะ `user-service`** เท่านั้นที่ออก token ผ่าน `POST /api/v1/auth/login` และ
`POST /api/v1/auth/refresh`

### 5.2 รูปแบบ Access Token

- ชนิด: JWT · อัลกอริทึม: **HS256** · เซ็นด้วย `JWT_SECRET` (ค่าเดียวกันทุก service ต้องยาว
  อย่างน้อย 32 ตัวอักษร) · อายุ: **15 นาที**

```json
{
  "sub": "<user UUID>",
  "email": "somchai@example.com",
  "username": "somchai",
  "role": "customer",
  "iss": "equipment-rental-system",
  "iat": 1757148000,
  "exp": 1757148900,
  "jti": "<uuid>"
}
```

### 5.3 ใครตรวจ token อย่างไร

- **Kong ตรวจ signature/expiry ให้แล้วก่อน request จะถึง service** (ผ่าน `jwt` plugin บน
  route ที่ต้อง login ใน `deploy/kong/kong.yml`, ตั้ง `claims_to_verify: ["exp"]` ไว้ด้วย)
- **product-service / rental-service ไม่ต้องถือ `JWT_SECRET` และไม่ verify signature เอง**
  — แค่อ่าน header `Authorization: Bearer <token>` แล้ว decode payload (base64) เอา claims
  `sub`/`role`/`email` ไปบังคับสิทธิ์
- **`JWT_SECRET` มี 2 ที่ต้องตรงกันแบบ byte-for-byte:** ค่าใน `.env` กับค่า hardcode ใน
  `deploy/kong/kong.yml` (`consumers[0].jwt_secrets[0].secret`) — เพราะ Kong declarative
  config อ่านค่าจาก `.env` ไม่ได้ ถ้าจะ rotate secret ต้องแก้ทั้งสองที่แล้ว
  `docker compose up -d --force-recreate kong` ไม่งั้น protected route จะ 401 เงียบ ๆ
- **เรียกตรงข้าม Kong (พอร์ต 8082/8083)** จะไม่มีใครเช็ค signature เลย — flow ปกติของ client
  ต้องผ่าน Kong (`:8000`) เท่านั้น

### 5.4 การยืนยันบัญชียัง active (`/auth/verify`)

Claims ที่ decode จาก Kong ใช้เป็นหลักได้เลย แต่ถ้าต้องมั่นใจว่าบัญชียัง active/ไม่ถูกแบน
(เช่น ตอน rental-service ยืนยันการเช่า) จะเรียก internal endpoint ของ user-service ตรง ๆ
ข้าม container (ไม่ผ่าน Kong):

```
POST http://user-service:8081/api/v1/auth/verify
Header: X-Internal-Key: <INTERNAL_API_KEY>
Body:   { "token": "<access token>" }
```

ตอบ `200` พร้อม `{ active, user_id, email, username, role, expires_at }` หรือ `401` ถ้า
token ใช้ไม่ได้

### 5.5 Refresh token

จัดการโดย user-service เท่านั้น — เป็น opaque string (ไม่ใช่ JWT), เก็บ hash ไว้ในตาราง
`refresh_tokens` ของ `user-db`, อายุ 7 วัน (168h) service อื่นไม่ต้องรู้จัก refresh token เลย

---

## 6. Authorization (RBAC)

3 บทบาท เก็บใน claim `role` ของ JWT:

| role | คำอธิบาย |
|---|---|
| `admin` | ผู้ดูแลระบบ — สิทธิ์เต็ม |
| `staff` | เจ้าหน้าที่ — จัดการสินค้า/การเช่าได้ แต่จัดการผู้ใช้ไม่ได้ |
| `customer` | ลูกค้า — เข้าถึงได้เฉพาะข้อมูล/รายการของตัวเอง |

### ตารางสิทธิ์ทั้งระบบ

| การกระทำ | admin | staff | customer |
|---|:---:|:---:|:---:|
| **User** — จัดการผู้ใช้ / กำหนดบทบาท / เปิด-ปิดบัญชี | ✅ | ❌ | ❌ |
| ดูรายชื่อ/รายละเอียดผู้ใช้ | ✅ | ✅ (อ่าน) | ❌ |
| จัดการโปรไฟล์/รหัสผ่านตนเอง | ✅ | ✅ | ✅ |
| **Product** — เพิ่ม/แก้ไข/ลบ สินค้า | ✅ | ✅ | ❌ |
| ดูรายการ/ค้นหาสินค้า | ✅ | ✅ | ✅ |
| **Rental** — สร้างรายการเช่าให้ลูกค้า | ✅ | ✅ | ❌ |
| สร้างคำขอเช่าของตนเอง | ✅ | ✅ | ✅ |
| อนุมัติคำขอ / บันทึกการคืนสินค้า | ✅ | ✅ | ❌ |
| ดูรายการเช่าทั้งหมด | ✅ | ✅ | ❌ |
| ดูประวัติการเช่าของตนเอง | ✅ | ✅ | ✅ |

แต่ละ service บังคับสิทธิ์ในฝั่งตัวเองจาก `role` ใน JWT claim; customer เข้าถึงได้เฉพาะข้อมูล
ของตัวเอง — service ต้องเทียบ `sub` ใน token กับ `user_id` ของ resource นั้น ๆ (เช่น
`GET /rentals/{id}` — เจ้าของรายการดูได้ คนอื่นที่ไม่ใช่ admin/staff โดน `403`)

---

## 7. รายละเอียดแต่ละ Service

### 7.1 user-service (พอร์ต 8081) — 20 endpoints

ฐานข้อมูล `user-db` มี 4 ตาราง:

```
roles           — id (smallint), name, description                 — seed มาให้ (admin/staff/customer)
users           — id (UUID), email, username, password_hash (bcrypt),
                  full_name, phone, role_id → roles, is_active, soft-delete (deleted_at)
refresh_tokens  — id (UUID), user_id → users, token_hash (sha256), expires_at, revoked_at
login_logs      — id (bigserial), user_id, email_attempted, success, ip_address, user_agent, created_at
```

| Method | Path | สิทธิ์ | หน้าที่ |
|---|---|---|---|
| GET | `/health` | Public | เช็คสถานะ + DB connection |
| POST | `/auth/register` | Public | สมัครสมาชิก — role เป็น `customer` เสมอ |
| POST | `/auth/login` | Public | เข้าสู่ระบบ คืน access token (15 นาที) + refresh token (7 วัน) |
| POST | `/auth/refresh` | Public + refresh token | ต่ออายุ access token, หมุน refresh token ใหม่ |
| POST | `/auth/logout` | Authenticated | revoke refresh token session นั้น |
| POST | `/auth/verify` | `X-Internal-Key` | ให้ service อื่นตรวจ token (ไม่ใช่ client) |
| GET / PUT | `/me` | Authenticated | ดู/แก้ไขโปรไฟล์ตนเอง |
| PUT | `/me/password` | Authenticated | เปลี่ยนรหัสผ่าน — revoke session อื่นทั้งหมด |
| GET | `/me/login-logs` | Authenticated | ประวัติ login ของตนเอง |
| GET | `/me/sessions` | Authenticated | refresh token ที่ยัง active |
| GET / POST | `/users` | Admin | รายการผู้ใช้ (ค้นหา/แบ่งหน้า) / สร้างผู้ใช้+กำหนดบทบาท |
| GET | `/users/{id}` | Admin, Staff | ดูรายละเอียดผู้ใช้ |
| PUT / DELETE | `/users/{id}` | Admin | แก้ไข / ลบผู้ใช้แบบ soft delete |
| PATCH | `/users/{id}/role` | Admin | เปลี่ยนบทบาท — revoke session เดิม, ห้าม demote ตัวเอง |
| PATCH | `/users/{id}/status` | Admin | เปิด/ปิดบัญชี |
| GET | `/users/{id}/login-logs` | Admin | ประวัติการเข้าใช้ของผู้ใช้รายนั้น |
| GET | `/roles` | Admin, Staff | รายการบทบาททั้งหมด |

**บัญชี seed มาให้ (สำหรับ dev/ทดสอบ):**

| Role | Email | Password |
|---|---|---|
| admin | `admin@equipment-rental.local` | `Admin123!` |
| staff | `staff@equipment-rental.local` | `Staff123!` |

เหตุผลที่ต้อง seed: `POST /auth/register` ตั้ง role เป็น `customer` เสมอ ไม่มีทางเลือก —
ถ้าไม่ seed ไว้ก่อนจะไม่มีทางสร้าง admin คนแรกได้เลย (endpoint ที่ตั้ง role อื่นได้ต้องเป็น
admin ก่อนถึงจะเรียกได้)

### 7.2 product-service (พอร์ต 8082)

ฐานข้อมูล `product-db` มี 2 ตาราง:

```
categories — id (UUID), name (unique), description, created_at        — seed มา 2 หมวด (กล้อง, เครื่องเสียง)
products   — id (UUID), category_id → categories, name, description,
             price_per_day (decimal 10,2), status (available/rented/maintenance),
             image_url, soft-delete (deleted_at)
```

| Method | Path | สิทธิ์ | หน้าที่ |
|---|---|---|---|
| GET | `/products` | Authenticated (ทุก role) | รายการสินค้า — ค้นหา `q`, กรอง `category_id`/`status`, แบ่งหน้า |
| GET | `/products/{id}` | Authenticated | รายละเอียดสินค้า |
| POST | `/products` | Admin, Staff | เพิ่มสินค้า |
| PUT | `/products/{id}` | Admin, Staff | แก้ไขสินค้า (partial update) |
| DELETE | `/products/{id}` | Admin | ลบสินค้าแบบ soft delete — ปฏิเสธด้วย `409 CONFLICT` ถ้าสถานะเป็น `rented` |
| PATCH | `/products/{id}/status` | Admin, Staff **หรือ** Internal (`X-Internal-Key`) | เปลี่ยนสถานะสินค้า |
| GET | `/categories` | Authenticated | รายการหมวดหมู่ |
| GET | `/health` | Public (เรียกตรง `:8082/health` เท่านั้น ไม่ผ่าน Kong) | สถานะบริการ |

**จุดออกแบบสำคัญ:**

- **สถานะสินค้ามี 3 ค่า** ไม่ใช่ 2: `available` / `rented` / `maintenance` — ค่าที่สามเป็นการ
  ปิดซ่อมชั่วคราว (ตั้งได้เฉพาะ admin/staff) ไม่เกี่ยวกับ flow เช่า/คืนที่ rental-service สั่ง
  เปลี่ยน (rental-service สลับแค่ `available` ↔ `rented` เท่านั้น)
- **`PATCH /products/{id}/status` เป็น atomic/conditional update เฉพาะทิศทาง `→ rented`**
  — ใช้ compare-and-swap แบบ `UPDATE products SET status='rented' WHERE id=? AND
  status='available'` แล้วเช็คว่ามีแถวถูกแก้จริง ถ้าไม่มี (สินค้าไม่ได้อยู่ในสถานะที่คาดไว้
  แล้ว) ตอบ `409 Conflict` แทนที่จะเขียนทับ — กันสองคนเช่าสินค้าชิ้นเดียวกันพร้อมกัน
  (race condition) ส่วนทิศทาง `→ available` (ตอนคืนสินค้า) เป็น **idempotent** สำเร็จได้เสมอ
  ไม่ต้องเช็ค prior-state
- **`PATCH /products/{id}/status`** รับได้ 2 ทาง: admin/staff ผ่าน Bearer token (ผ่าน Kong)
  **หรือ** rental-service เรียกตรงด้วย `X-Internal-Key` (ข้าม Kong) — เมื่อเรียกจาก
  rental-service จะมีแค่ `X-Internal-Key` ไม่มี `Authorization: Bearer` เพราะ rental-service
  ไม่ forward token ของผู้เรียกต่อ
- **ไม่มี public `/health` ผ่าน Kong โดยตั้งใจ** — เพื่อเลี่ยง route ชนกับ `/health` ของ
  user-service ใน Kong (เช็คตรงที่ `:8082/health` แทน)

### 7.3 rental-service (พอร์ต 8083)

ฐานข้อมูล `rental-db` มี 1 ตารางหลัก:

```
rentals — id (UUID), user_id (UUID, ไม่มี FK ข้าม DB), product_id (UUID, ไม่มี FK ข้าม DB),
          start_date, due_date, return_date, total_price (numeric 12,2),
          status (pending/active/returned/cancelled)
          CHECK (due_date > start_date), CHECK (return_date >= start_date)
```

| Method | Path | สิทธิ์ | หน้าที่ |
|---|---|---|---|
| GET | `/rentals` | Admin, Staff | รายการเช่าทั้งหมด — filter `status`, sort, pagination |
| GET | `/rentals/{id}` | Admin, Staff, เจ้าของรายการ | รายละเอียดการเช่า |
| POST | `/rentals` | Admin, Staff | สร้างรายการเช่าแทนลูกค้า — สถานะเริ่มที่ `active` ทันที |
| POST | `/rentals/request` | Customer | ลูกค้าขอเช่าเอง — สถานะเริ่มที่ `pending` |
| PATCH | `/rentals/{id}/approve` | Admin, Staff | `pending → active` — เช็คสินค้าว่างแล้วสั่งจอง |
| PATCH | `/rentals/{id}/return` | Admin, Staff | `active → returned` — คืนสถานะสินค้าเป็น `available` |
| GET | `/me/rentals` | Authenticated | ประวัติการเช่าของตนเอง |
| GET | `/rental/health` (ผ่าน Kong) | Public | สถานะบริการ — path ต่างจาก `/health` เพื่อเลี่ยง route ชนกัน |

**สถานะของรายการเช่า:**

```
pending ──approve──► active ──return──► returned
   │
   └──(ยกเลิก — มีอยู่ใน DB constraint แต่ยังไม่มี endpoint ในวันนี้)──► cancelled
```

**การคำนวณราคา:** `price_per_day` (ดึงจาก product-service) × จำนวนวัน (`due_date -
start_date`) บันทึกเป็น `total_price` ตอนสร้าง ไม่เปลี่ยนตามราคาสินค้าที่แก้ไขภายหลัง

**Reserve-then-compensate pattern:** ทุก action ที่แตะสถานะสินค้า (`Create`/`Approve`/
`Return`) จะ **เปลี่ยนสถานะสินค้าที่ product-service ก่อน** แล้วค่อยเขียนฐานข้อมูลของตัวเอง
— ถ้าเขียน DB ไม่สำเร็จ จะเรียก `SetStatus` ย้อนกลับให้สินค้ากลับไปสถานะเดิมทันที (ชดเชย
ธุรกรรมข้าม service ที่ไม่มี distributed transaction รองรับ)

**การเรียก service อื่น** (ตรงข้าม container name ด้วย `X-Internal-Key`, ไม่ผ่าน Kong,
ไม่มี Bearer token):

| เรียกอะไร | ใช้ทำอะไร |
|---|---|
| `GET /products/{id}` | ตรวจสินค้า สถานะ และราคาต่อวัน |
| `PATCH /products/{id}/status` | เปลี่ยนเป็น `rented` ตอนเริ่มเช่า, `available` ตอนคืน/ตีกลับ — ต้องเป็น atomic ฝั่ง product-service, ตอบ `409` แปลงเป็น `ErrProductUnavailable` |
| `POST /auth/verify` | ยืนยันว่าบัญชีของผู้สั่งการยัง active |

---

## 8. Kong API Gateway

Kong รันแบบ **DB-less (declarative mode)** — config ทั้งหมดอยู่ในไฟล์เดียว
[`deploy/kong/kong.yml`](../deploy/kong/kong.yml) ไม่มี Kong Postgres/Admin API

### Path ที่ Kong ประกาศไว้

| Service | Public (ไม่ต้อง JWT) | Protected (ต้อง JWT ที่ Kong ตรวจแล้ว) |
|---|---|---|
| user-service | `/health`, `/auth/register`, `/auth/login`, `/auth/refresh` | `/auth/logout`, `/me*`, `/users*`, `/roles` |
| user-service (internal) | — | `/auth/verify` ใช้ `X-Internal-Key` ไม่ใช่ JWT |
| rental-service | `/rental/health` | `/rentals*`, `/rentals/request`, `/me/rentals` |
| product-service | — (ยังไม่มี public health route ผ่าน Kong) | `/products*`, `/categories` |
| docs (Swagger UI) | `/docs` | — |

### ข้อควรรู้

- Kong ตั้ง `claims_to_verify: ["exp"]` และ `run_on_preflight: false` ไว้บน `jwt` plugin —
  ห้ามลบ `claims_to_verify` ไม่งั้น Kong จะไม่เช็คว่า token หมดอายุหรือยัง
- Kong **ไม่ทำ RBAC/rate-limiting/logging** — สิทธิ์ตาม role ยังเช็คในแต่ละ service เอง
- Service-to-service call (`rental→product`, `rental→user`) ไม่ผ่าน Kong เลย เรียกตรงข้าม
  container name ใน Docker network เดียวกัน (`rental-net`)
- route `/docs` ต้องใช้ `strip_path: false` (ต่างจาก service อื่นที่ strip path ออก) ไม่งั้น
  asset (JS/CSS) ของหน้า Swagger UI จะหาไฟล์ไม่เจอ

---

## 9. มาตรฐาน Response / Error (ทุก service ใช้ร่วมกัน)

**สำเร็จ (object เดียว):**
```json
{ "success": true, "data": { } }
```

**สำเร็จ (list + pagination):**
```json
{ "success": true, "data": [ ], "meta": { "page": 1, "limit": 20, "total": 57, "total_pages": 3 } }
```

**ผิดพลาด:**
```json
{ "success": false, "error": { "code": "VALIDATION_ERROR", "message": "...", "details": { } } }
```

| HTTP | `error.code` | ความหมาย |
|---|---|---|
| 400 | `VALIDATION_ERROR` / `BAD_REQUEST` | ข้อมูล request ไม่ผ่าน validation / JSON เสีย |
| 401 | `UNAUTHENTICATED` | ไม่มี token / หมดอายุ / ลายเซ็นผิด |
| 403 | `FORBIDDEN` / `ACCOUNT_DISABLED` | บทบาทไม่มีสิทธิ์ / บัญชีถูกปิด |
| 404 | `NOT_FOUND` | ไม่พบข้อมูล |
| 409 | `CONFLICT` | ข้อมูลซ้ำ / ขัดแย้งกับสถานะปัจจุบัน (เช่น สินค้าไม่ว่าง) |
| 422 | `UNPROCESSABLE` | ถูกต้องเชิงรูปแบบ แต่ทำตามไม่ได้เชิงธุรกิจ |
| 429 | `TOO_MANY_REQUESTS` | เรียกถี่เกิน |
| 500 / 503 | `INTERNAL_ERROR` | error ภายใน / dependency (service อื่น) ล่ม |

**Pagination:** `page` (default 1), `limit` (default 20, สูงสุด 100), `sort` (default
`created_at`), `order` (`asc`/`desc`, default `desc`)

---

## 10. การรันระบบ

**Requirements:** Docker และ Docker Compose

```bash
cp .env.example .env
docker compose up -d --build
docker compose ps    # user-service, user-db, product-service, product-db, rental-service, rental-db, docs, kong ต้อง healthy/running
```

ทดสอบผ่าน Kong:

```bash
curl http://localhost:8000/health          # -> 200 (user-service)
curl http://localhost:8000/rental/health   # -> 200 (rental-service)
curl http://localhost:8000/api/v1/products # -> 401 ไม่มี token
```

Login ด้วยบัญชี admin ที่ seed มาให้:

```bash
curl -X POST http://localhost:8000/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@equipment-rental.local","password":"Admin123!"}'
```

เปิด **Swagger UI** ที่ <http://localhost:8000/docs/> (เลือก service จาก dropdown มุมบนขวา)
เพื่อดู/ทดลองยิง API จริงของทั้ง 3 service ผ่าน Kong

### รัน service เดียวระหว่างพัฒนา

แต่ละคนรันแค่ service + db ของตัวเองได้เลยไม่ต้องรอคนอื่น (user-service ไม่พึ่งใคร;
product/rental ต้องการ JWT ทดสอบ — โคลน user-service มารัน หรือ mock claims เองสำหรับ
เทส business logic ปกติที่ไม่ผ่าน Kong)

---

## 11. การทดสอบ

**Unit/integration test ของแต่ละ service** (ใช้ SQLite ในหน่วยความจำ ไม่ต้องมี Postgres จริง):

```bash
cd user-service    && go test ./...
cd product-service && go test ./...
cd rental-service  && go test ./...
```

rental-service เทสครอบคลุม repository, HTTP client, middleware, service, handler, router
โดย mock การเรียก product-service/user-service ด้วย `httptest.NewServer`

**End-to-end ผ่าน Kong จริง** (ต้องมี stack ทั้งหมดรันอยู่):

```bash
./deploy/kong/smoke-test.sh                     # user-service ผ่าน Kong (6 passed)
./deploy/kong/rental-smoke-test.sh              # rental-service ผ่าน Kong (8 passed)
./deploy/kong/product-rental-e2e-smoke-test.sh  # flow เต็ม: rent -> approve -> return ข้ามทั้ง 3 service
```

`product-rental-e2e-smoke-test.sh` ยังพิสูจน์ compare-and-swap ของ `PATCH
/products/{id}/status` ด้วยการยิงตรงเพื่อทดสอบ race condition

---

## 12. Environment Variables

| ตัวแปร | ใช้ที่ไหน |
|---|---|
| `JWT_SECRET` (≥32 ตัวอักษร) | user-service (ออก+ตรวจ token) + `deploy/kong/kong.yml` (ต้องตรงกันแบบ byte-for-byte) |
| `JWT_ACCESS_TTL` (`15m`) / `JWT_REFRESH_TTL` (`168h`) | user-service |
| `INTERNAL_API_KEY` | เรียก `/auth/verify` และ endpoint ภายในของ product-service จาก service อื่น |
| `USER_*` / `PRODUCT_*` / `RENTAL_*` | ตัวแปร DB/port เฉพาะแต่ละ service (namespace กันชนกันใน `.env` เดียว — docker-compose map เป็นชื่อ unprefixed ที่แต่ละ service อ่าน) |
| `CORS_ORIGIN` | origin เดียวที่เบราว์เซอร์เรียกได้ (default `http://localhost:3000`) |
| `BCRYPT_COST` (`12`) | user-service เท่านั้น |
| `MIGRATIONS_PATH` | โฟลเดอร์ไฟล์ `.sql` migration |

ดูค่าเริ่มต้นทั้งหมดที่ [`.env.example`](../.env.example)

---

## 13. เอกสารอ้างอิงเพิ่มเติมในโปรเจค

- [README.md](../README.md) — ภาพรวมสั้น + quick start
- [CONTRACT.md](../CONTRACT.md) — ข้อตกลงกลางระหว่างทีม (พอร์ต, JWT, response format, RBAC, endpoint ภายใน, docker-compose) — เอกสารที่ทุกอย่างในไฟล์นี้อ้างอิงมา
- [docs/user-management-service-design.md](user-management-service-design.md) — เอกสารออกแบบ user-service ฉบับเต็ม
- [docs/superpowers/specs/2026-09-18-kong-api-gateway-design.md](superpowers/specs/2026-09-18-kong-api-gateway-design.md) — เอกสารออกแบบ Kong Gateway
- [deploy/kong/README.md](../deploy/kong/README.md) — path ทั้งหมดที่ Kong ประกาศ + วิธีทดสอบผ่าน gateway
- [docs/openapi/](openapi/) — OpenAPI spec ของทั้ง 3 service (เขียนขึ้นจากโค้ด/CONTRACT.md จริง)
- README ของแต่ละ service: [user-service/README.md](../user-service/README.md),
  [product-service/README.md](../product-service/README.md),
  [rental-service/README.md](../rental-service/README.md)

# CONTRACT.md — ข้อตกลงร่วมของทีม

> โครงงาน: **ระบบเช่าอุปกรณ์ (Equipment Rental System)**
> สถาปัตยกรรม: Microservices · Go · PostgreSQL (Docker) · Database per Service
>
> เอกสารนี้คือ **"สัญญา" กลางของทีม** — อะไรก็ตามที่ service ของคุณเปิดให้ service อื่นเรียก
> หรือค่าที่ต้องใช้ร่วมกัน ต้องอยู่ในไฟล์นี้ **การเปลี่ยนแปลงใด ๆ ต้องแจ้งกลุ่มก่อนและอัปเดตไฟล์นี้**

| ทีม | รหัสนักศึกษา | Service ที่รับผิดชอบ |
|---|---|---|
| เอกพล แรกเรียง | 67114540666 | `product-service` (ระบบจัดการสินค้า) |
| วิษณุพงศ์ บัวเขียว | 67114540509 | `rental-service` (ระบบจัดการการเช่า) |
| สุรเชษฐ์ สีสา | 67114540583 | `user-service` (ระบบจัดการผู้ใช้งาน JWT & RBAC) |

**สถานะเอกสาร:** ฉบับร่าง v1 — รอทีมรีวิวและยืนยันร่วมกัน

---

## สารบัญ

1. [พอร์ตและชื่อ Service](#1-พอร์ตและชื่อ-service)
2. [Environment Variables ที่ใช้ร่วมกัน](#2-environment-variables-ที่ใช้ร่วมกัน)
3. [Database ต่อ Service](#3-database-ต่อ-service)
4. [มาตรฐาน Response / Error](#4-มาตรฐาน-response--error)
5. [Authentication (JWT)](#5-authentication-jwt)
6. [Authorization (RBAC)](#6-authorization-rbac)
7. [การสื่อสารระหว่าง Service](#7-การสื่อสารระหว่าง-service)
8. [สรุป Endpoint ของแต่ละ Service](#8-สรุป-endpoint-ของแต่ละ-service)
9. [Docker & การประกอบรวม](#9-docker--การประกอบรวม)
10. [Git Workflow และการแก้สัญญา](#10-git-workflow-และการแก้สัญญา)
11. [ข้อตกลงการตั้งชื่อ](#11-ข้อตกลงการตั้งชื่อ)

---

## 1. พอร์ตและชื่อ Service

ชื่อ service ต้องตรงกันเป๊ะ เพราะใช้เป็น hostname ใน Docker network

| Service | ชื่อใน docker-compose | พอร์ต (host:container) | Base path |
|---|---|---|---|
| **Kong (gateway)** | `kong` | `8000:8000` | — (proxy หน้าทั้ง 3 service) |
| User Management | `user-service` | `8081:8081` | `/api/v1` |
| Product | `product-service` | `8082:8082` | `/api/v1` |
| Rental | `rental-service` | `8083:8083` | `/api/v1` |

> **Client เรียกผ่าน Kong (`:8000`) เท่านั้นสำหรับ flow ปกติ** ตั้งแต่นี้ไป พอร์ต `8081`-`8083`
> ยังเปิด map ไว้เพื่อ debug/health check ตรงเท่านั้น ไม่ใช่ทางที่ client ควรใช้อีกต่อไป

ฐานข้อมูล (คอนเทนเนอร์แยกต่อ service)

| DB | ชื่อคอนเทนเนอร์ | พอร์ตภายใน | เจ้าของ |
|---|---|---|---|
| `user-db` | `user-db` | 5432 | user-service |
| `product-db` | `product-db` | 5432 | product-service |
| `rental-db` | `rental-db` | 5432 | rental-service |

> เรียกข้าม service ภายใน network ใช้ชื่อ service: `http://user-service:8081/api/v1/...`
> เรียกจากเครื่อง dev (นอก Docker) ใช้ `http://localhost:8081/api/v1/...`

---

## 2. Environment Variables ที่ใช้ร่วมกัน

ค่าเหล่านี้ **ต้องเหมือนกันทุก service** — เก็บใน `.env.example` ที่ commit เข้า repo (ห้าม commit `.env` จริง)

| ตัวแปร | ค่า dev (ตัวอย่าง) | ใช้ทำอะไร |
|---|---|---|
| `JWT_SECRET` | `dev-secret-please-change-min-32-characters` | คีย์เซ็น/ตรวจ JWT — **ทุก service ใช้ค่าเดียวกัน** |
| `JWT_ACCESS_TTL` | `15m` | อายุ access token |
| `JWT_REFRESH_TTL` | `168h` | อายุ refresh token (7 วัน) — เฉพาะ user-service ใช้ |
| `INTERNAL_API_KEY` | `dev-internal-key` | คีย์เรียก endpoint ภายใน (`/auth/verify`) |
| `TZ` | `Asia/Bangkok` | timezone ของคอนเทนเนอร์ |

> **Kong เก็บ `JWT_SECRET` อีกชุดแยกไว้ต่างหาก** ใน `deploy/kong/kong.yml`
> (`consumers[0].jwt_secrets[0].secret`) — ไฟล์นี้ไม่ได้อ่านค่าจาก `.env` เพราะเป็น
> declarative config ของ Kong เอง ถ้าจะหมุน (rotate) `JWT_SECRET` ต้องแก้ **ทั้งสอง
> ที่** ให้ตรงกันแบบ byte-for-byte แล้ว restart container `kong` ด้วย
> (`docker compose up -d --force-recreate kong`) ไม่งั้น route ที่ต้อง login จะ
> 401 แบบเงียบๆ โดยไม่มี error บอกสาเหตุ

ค่าเฉพาะ service (แต่ละคนตั้งเอง): `APP_PORT`, `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`

---

## 3. Database ต่อ Service

**กฎเหล็ก:**

1. แต่ละ service เป็นเจ้าของ DB ตัวเอง **ห้าม service อื่นต่อ DB ตรง**
2. **ห้ามมี Foreign Key ข้าม DB** — เก็บได้แค่ค่า id (เช่น `rentals.user_id UUID`, `rentals.product_id UUID`) โดยไม่มี FK constraint ข้ามฐาน
3. อยากได้ข้อมูลของ service อื่น → เรียกผ่าน HTTP API ของ service นั้น หรืออ่านจาก JWT claim
4. แต่ละคน migrate DB ตัวเอง ไม่ยุ่ง schema คนอื่น

**ชนิดข้อมูล id ที่ใช้อ้างอิงข้าม service:**

| ข้อมูล | ชนิด | มาจาก |
|---|---|---|
| user id | `UUID` (v4) | user-service |
| product id | `UUID` (v4) | product-service |
| rental id | `UUID` (v4) | rental-service |

> ทุก service ใช้ **UUID v4** เป็น primary key เพื่อความสม่ำเสมอ

---

## 4. มาตรฐาน Response / Error

ทุก service ต้องตอบด้วยรูปแบบเดียวกัน

### 4.1 สำเร็จ (single object)

```json
{ "success": true, "data": { } }
```

### 4.2 สำเร็จ (list + pagination)

```json
{
  "success": true,
  "data": [ ],
  "meta": { "page": 1, "limit": 20, "total": 57, "total_pages": 3 }
}
```

### 4.3 ผิดพลาด

```json
{
  "success": false,
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "ข้อความอ่านเข้าใจง่ายสำหรับผู้ใช้",
    "details": { "email": "รูปแบบอีเมลไม่ถูกต้อง" }
  }
}
```

`details` เป็น `null` ได้ถ้าไม่มีรายละเอียดเพิ่ม

### 4.4 รหัส Error ที่ใช้ร่วมกัน

| HTTP | `error.code` | ความหมาย |
|---|---|---|
| 400 | `VALIDATION_ERROR` | ข้อมูล request ไม่ผ่าน validation (`details` = map ของ field → ข้อความ) |
| 400 | `BAD_REQUEST` | รูปแบบ request ผิด เช่น JSON เสีย |
| 401 | `UNAUTHENTICATED` | ไม่มี token / token หมดอายุ / ลายเซ็นผิด |
| 403 | `FORBIDDEN` | บทบาทไม่มีสิทธิ์ |
| 403 | `ACCOUNT_DISABLED` | บัญชีถูกปิด |
| 404 | `NOT_FOUND` | ไม่พบข้อมูล |
| 409 | `CONFLICT` | ข้อมูลซ้ำ / ขัดแย้งกับสถานะปัจจุบัน |
| 422 | `UNPROCESSABLE` | request ถูกต้องเชิงรูปแบบ แต่ทำตามไม่ได้เชิงธุรกิจ |
| 429 | `TOO_MANY_REQUESTS` | เรียกถี่เกิน (rate limit) |
| 500 | `INTERNAL_ERROR` | ข้อผิดพลาดภายใน |

(user-service มีรหัสเฉพาะเพิ่ม เช่น `INVALID_CREDENTIALS`, `EMAIL_ALREADY_EXISTS` — ดูเอกสารของ user-service)

### 4.5 ข้อตกลง Pagination

| Query param | ค่าเริ่มต้น | หมายเหตุ |
|---|---|---|
| `page` | `1` | เริ่มที่ 1 |
| `limit` | `20` | สูงสุด 100 |
| `sort` | `created_at` | ชื่อ field |
| `order` | `desc` | `asc` / `desc` |

### 4.6 Health check (ทุก service ต้องมี)

`GET /health` → `200`

```json
{ "success": true, "data": { "status": "ok", "service": "product-service", "db": "up" } }
```

ถ้า DB ต่อไม่ได้ → `503` + `INTERNAL_ERROR`

---

## 5. Authentication (JWT)

### 5.1 ใครออก token

**เฉพาะ `user-service`** เท่านั้นที่ออก token ผ่าน `/api/v1/auth/login` และ `/api/v1/auth/refresh`

### 5.2 รูปแบบ Access Token

- ชนิด: JWT · อัลกอริทึม: **HS256** · เซ็นด้วย `JWT_SECRET`
- อายุ: 15 นาที

**Claims**

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

`iss` เป็นค่าคงที่เดียวกันทุก token (ไม่ผูกกับ user) — Kong's `jwt` plugin ใช้ค่านี้จับคู่กับ
credential ที่ตั้งไว้ใน `deploy/kong/kong.yml` เพื่อเลือก secret มาตรวจลายเซ็น

### 5.3 product-service / rental-service ตรวจ token อย่างไร

**Kong ตรวจ signature/expiry ให้แล้วก่อน request จะมาถึง service** (ผ่าน `jwt` plugin บน route
ที่ต้อง login ใน `deploy/kong/kong.yml`) ดังนั้น `product-service` และ `rental-service`:

1. **ไม่ต้องถือ `JWT_SECRET`** และ**ไม่ต้อง verify signature เอง**
2. อ่าน header `Authorization: Bearer <token>` (Kong forward ให้โดยไม่ตัดออก)
3. Decode ส่วน payload (base64) อ่าน claims `sub` / `role` / `email` ไปใช้บังคับสิทธิ์ — ไม่ต้อง verify signature ซ้ำ
4. เรียกตรงพอร์ต 8082/8083 (ข้าม Kong) จะไม่มีใครเช็ค signature — ต้องทดสอบ reject flow ผ่าน Kong (`:8000`) เท่านั้น

**กรณีต้องมั่นใจว่าบัญชียัง active / ไม่ถูกแบน** — เรียก introspection ของ user-service เหมือนเดิม
(ไม่เปลี่ยน จาก Kong ไม่ทำ RBAC หรือเช็ค active/ban):

```
POST http://user-service:8081/api/v1/auth/verify
Header: X-Internal-Key: <INTERNAL_API_KEY>
Body:   { "token": "<access token>" }
```

ตอบ `200` พร้อม `{ active, user_id, email, username, role, expires_at }`
หรือ `401` ถ้า token ใช้ไม่ได้

> แนะนำ: ใช้ claims ที่ decode จาก Kong เป็นหลัก เรียก `/auth/verify` เฉพาะ action สำคัญ
> (เช่น ยืนยันการเช่า) เพื่อลด network call — endpoint นี้เรียกตรงข้าม container เหมือนเดิม ไม่ผ่าน Kong

### 5.4 Refresh token

- จัดการโดย user-service เท่านั้น (opaque string, เก็บ hash ใน `user-db`)
- service อื่นไม่ต้องรู้จัก refresh token

---

## 6. Authorization (RBAC)

### 6.1 บทบาท

| role (ค่าใน JWT) | คำอธิบาย |
|---|---|
| `admin` | ผู้ดูแลระบบ |
| `staff` | เจ้าหน้าที่ |
| `customer` | ลูกค้า |

### 6.2 ตารางสิทธิ์ (ทั้งระบบ)

| การกระทำ | admin | staff | customer |
|---|:---:|:---:|:---:|
| **User** |
| จัดการผู้ใช้ / กำหนดบทบาท / เปิด-ปิดบัญชี | ✅ | ❌ | ❌ |
| ดูรายชื่อ/รายละเอียดผู้ใช้ | ✅ | ✅ (อ่าน) | ❌ |
| จัดการโปรไฟล์/รหัสผ่านตนเอง | ✅ | ✅ | ✅ |
| **Product** |
| เพิ่ม / แก้ไข / ลบ สินค้า | ✅ | ✅ | ❌ |
| ดูรายการ / ค้นหาสินค้า | ✅ | ✅ | ✅ |
| **Rental** |
| สร้างรายการเช่า (ให้ลูกค้า) | ✅ | ✅ | ❌ |
| สร้างคำขอเช่าของตนเอง | ✅ | ✅ | ✅ |
| บันทึกการคืนสินค้า | ✅ | ✅ | ❌ |
| ดูรายการเช่าทั้งหมด | ✅ | ✅ | ❌ |
| ดูประวัติการเช่าของตนเอง | ✅ | ✅ | ✅ |

### 6.3 การบังคับสิทธิ์

- แต่ละ service บังคับสิทธิ์ในฝั่งตัวเองจาก `role` ใน JWT claim
- customer เข้าถึงได้เฉพาะข้อมูลของตัวเอง → service ต้องเทียบ `sub` ใน token กับ `user_id` ของ resource

---

## 7. การสื่อสารระหว่าง Service

### 7.1 ใครเรียกใคร

```
Client ──► user-service     (login, จัดการผู้ใช้)
Client ──► product-service  (ดู/จัดการสินค้า)
Client ──► rental-service   (เช่า/คืน)

rental-service ──► product-service   : ตรวจว่าสินค้ามีอยู่ / ว่าง / ราคาเท่าไร
rental-service ──► user-service      : (option) /auth/verify หรือดึงชื่อผู้เช่า
product-service ──► rental-service   : (option) เช็คว่าสินค้าถูกเช่าอยู่ไหม ก่อนลบ
```

> ทางเลือกที่ง่ายกว่าการให้ product เรียก rental: ให้ **rental-service เป็นคนอัปเดตสถานะ** โดยเรียก `PATCH /products/{id}/status` ของ product-service ตอนเช่า/คืน — ทีมเลือกแนวทางนี้ร่วมกัน

> **Kong ไม่เกี่ยวกับ diagram ข้างบนนี้เลย** — ทุกลูกศรใน 7.1 (`rental→product`, `rental→user`,
> `product→rental`) ยังเรียกตรงข้าม container name เหมือนเดิม ไม่ผ่าน Kong (`:8000`) Kong เป็นแค่
> ทางเข้าสำหรับ **client** เท่านั้น

### 7.2 กติกาการเรียกข้าม service

1. เรียกผ่าน HTTP REST เท่านั้น ใช้ base URL `http://<service-name>:<port>/api/v1`
2. ส่ง header `X-Internal-Key: <INTERNAL_API_KEY>` สำหรับ endpoint ภายใน
3. ถ้า service ปลายทางล่ม → ตอบ client ด้วย `503` + `INTERNAL_ERROR` (ไม่ค้าง)
4. ห้ามเก็บ cache ข้อมูลของ service อื่นแบบถาวร — ดึงสดตอนใช้งาน (project ขนาดนี้ยังไม่ต้องทำ cache)

### 7.3 Endpoint ภายในที่แต่ละ service เปิดให้ service อื่น (ไม่ใช่ client)

| Endpoint | เจ้าของ | ผู้เรียก | ใช้ทำอะไร |
|---|---|---|---|
| `POST /api/v1/auth/verify` | user-service | product, rental | ตรวจ token + ดึง user/role |
| `GET /api/v1/products/{id}` | product-service | rental-service | ดึงข้อมูล/ราคา/สถานะสินค้า |
| `PATCH /api/v1/products/{id}/status` | product-service | rental-service | เปลี่ยนสถานะสินค้า (`available` / `rented`) |

> product / rental ยืนยันและเติมรายละเอียด (parameter, response) ของ endpoint ตัวเองในหัวข้อ 8

---

## 8. สรุป Endpoint ของแต่ละ Service

Base path ทุก service = `/api/v1` · ทุก endpoint (ยกเว้นที่ระบุ Public) ต้องมี `Authorization: Bearer <token>`

### 8.1 user-service (เจ้าของ: สุรเชษฐ์) — ✅ ยืนยันแล้ว

รายละเอียดเต็มอยู่ที่ `docs/user-management-service-design.md`

| Method | Path | สิทธิ์ | หน้าที่ |
|---|---|---|---|
| POST | `/auth/register` | Public | สมัครสมาชิก (เป็น customer) |
| POST | `/auth/login` | Public | เข้าสู่ระบบ คืน access + refresh token |
| POST | `/auth/refresh` | Public + refresh token | ต่ออายุ access token |
| POST | `/auth/logout` | Authenticated | ออกจากระบบ (revoke refresh token) |
| POST | `/auth/verify` | Internal key | ให้ service อื่นตรวจ token |
| GET | `/me` | Authenticated | ดูโปรไฟล์ตนเอง |
| PUT | `/me` | Authenticated | แก้ไขโปรไฟล์ตนเอง |
| PUT | `/me/password` | Authenticated | เปลี่ยนรหัสผ่านตนเอง |
| GET | `/me/login-logs` | Authenticated | ประวัติการเข้าใช้ของตนเอง |
| GET | `/me/sessions` | Authenticated | รายการ session ที่ยัง active |
| GET | `/users` | Admin | รายการผู้ใช้ (ค้นหา/แบ่งหน้า) |
| POST | `/users` | Admin | สร้างผู้ใช้ + กำหนดบทบาท |
| GET | `/users/{id}` | Admin, Staff | ดูรายละเอียดผู้ใช้ |
| PUT | `/users/{id}` | Admin | แก้ไขข้อมูลผู้ใช้ |
| DELETE | `/users/{id}` | Admin | ลบผู้ใช้ (soft delete) |
| PATCH | `/users/{id}/role` | Admin | เปลี่ยนบทบาท |
| PATCH | `/users/{id}/status` | Admin | เปิด/ปิดบัญชี |
| GET | `/users/{id}/login-logs` | Admin | ประวัติการเข้าใช้ของผู้ใช้รายนั้น |
| GET | `/roles` | Admin, Staff | รายการบทบาท |
| GET | `/health` | Public | สถานะบริการ |

### 8.2 product-service (เจ้าของ: เอกพล) — ⏳ ร่าง รอเจ้าของยืนยัน

| Method | Path | สิทธิ์ | หน้าที่ |
|---|---|---|---|
| GET | `/products` | Authenticated (ทุก role) | รายการสินค้า (ค้นหา/กรอง/แบ่งหน้า) |
| GET | `/products/{id}` | Authenticated | รายละเอียดสินค้า |
| POST | `/products` | Admin, Staff | เพิ่มสินค้า |
| PUT | `/products/{id}` | Admin, Staff | แก้ไขสินค้า |
| DELETE | `/products/{id}` | Admin | ลบสินค้า |
| PATCH | `/products/{id}/status` | Admin, Staff, Internal | เปลี่ยนสถานะ (`available`/`rented`) |
| GET | `/categories` | Authenticated | รายการหมวดหมู่ |
| GET | `/health` | Public | สถานะบริการ |

> ต้องมีอย่างน้อย: `id` (UUID), `name`, `description`, `category`, `price_per_day`, `status`, `created_at`
>
> **`PATCH /products/{id}/status` ต้องเป็น atomic/conditional update** — compare-and-swap บนเงื่อนไข
> `status = 'available'` ก่อนเปลี่ยนเป็น `rented` (และเทียบเท่าตอนเปลี่ยนกลับ) เช่น
> `UPDATE products SET status = 'rented' WHERE id = ? AND status = 'available'` แล้วเช็คว่ามีแถวถูกแก้จริง
> ถ้าไม่มีแถวถูกแก้ (สินค้าไม่ได้อยู่ในสถานะที่คาดไว้แล้ว) ต้องตอบ `409 Conflict` แทนที่จะเขียนทับ
> เงื่อนไขนี้จำเป็นเพื่อกันสอง caller เช่าสินค้าชิ้นเดียวกันพร้อมกัน (race condition) —
> rental-service's `internal/client/product_client.go` (`ProductClient.SetStatus`) ตีความ `409`
> เป็น `ErrProductUnavailable` ไว้แล้วตั้งแต่ตอนส่งมอบงาน โค้ดฝั่ง rental-service พร้อมใช้เงื่อนไขนี้
> ทันทีที่ product-service ทำ endpoint นี้ให้ตรงกัน

### 8.3 rental-service (เจ้าของ: วิษณุพงศ์) — ⏳ ร่าง รอเจ้าของยืนยัน

| Method | Path | สิทธิ์ | หน้าที่ |
|---|---|---|---|
| GET | `/rentals` | Admin, Staff | รายการเช่าทั้งหมด |
| GET | `/rentals/{id}` | Admin, Staff, เจ้าของรายการ | รายละเอียดการเช่า |
| POST | `/rentals` | Admin, Staff | สร้างรายการเช่าให้ลูกค้า |
| POST | `/rentals/request` | Customer | ลูกค้าสร้างคำขอเช่าของตนเอง |
| PATCH | `/rentals/{id}/return` | Admin, Staff | บันทึกการคืนสินค้า + เปลี่ยนสถานะสินค้าเป็นว่าง |
| PATCH | `/rentals/{id}/approve` | Admin, Staff | อนุมัติคำขอเช่า |
| GET | `/me/rentals` | Authenticated | ประวัติการเช่าของตนเอง |
| GET | `/rental/health` (ผ่าน Kong) | Public | สถานะบริการ — path ต่างจาก `/health` ที่ service เสิร์ฟเอง เพื่อเลี่ยง route ชนกันใน Kong (ดู 9.2) |

> ต้องมีอย่างน้อย: `id` (UUID), `user_id` (UUID), `product_id` (UUID), `start_date`, `due_date`, `return_date`, `total_price`, `status` (`pending`/`active`/`returned`/`cancelled`), `created_at`
> การคำนวณค่าเช่า: ดึง `price_per_day` จาก product-service × จำนวนวัน

---

## 9. Docker & การประกอบรวม

### 9.1 Network กลาง

ทุก service + DB อยู่ใน network เดียว

```yaml
networks:
  rental-net:
    name: rental-net
    driver: bridge
```

### 9.2 docker-compose รวม (repo `deploy/` หรือ root ของ monorepo)

```yaml
version: "3.9"

services:
  user-service:
    build: ./user-service
    ports: ["8081:8081"]
    env_file: [./.env]
    depends_on: { user-db: { condition: service_healthy } }
    networks: [rental-net]
  user-db:
    image: postgres:16-alpine
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

  product-service:
    build: ./product-service
    ports: ["8082:8082"]
    env_file: [./.env]
    depends_on: { product-db: { condition: service_healthy } }
    networks: [rental-net]
  product-db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: product_service
      POSTGRES_PASSWORD: secret
      POSTGRES_DB: product_db
    volumes: ["product-db-data:/var/lib/postgresql/data"]
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U product_service -d product_db"]
      interval: 5s
      timeout: 5s
      retries: 5
    networks: [rental-net]

  rental-service:
    build: ./rental-service
    ports: ["8083:8083"]
    env_file: [./.env]
    depends_on: { rental-db: { condition: service_healthy } }
    networks: [rental-net]
  rental-db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: rental_service
      POSTGRES_PASSWORD: secret
      POSTGRES_DB: rental_db
    volumes: ["rental-db-data:/var/lib/postgresql/data"]
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U rental_service -d rental_db"]
      interval: 5s
      timeout: 5s
      retries: 5
    networks: [rental-net]

  kong:
    image: kong:3.6
    environment:
      KONG_DATABASE: "off"
      KONG_DECLARATIVE_CONFIG: /kong/kong.yml
      KONG_PROXY_ACCESS_LOG: /dev/stdout
      KONG_ADMIN_ACCESS_LOG: /dev/stdout
      KONG_PROXY_ERROR_LOG: /dev/stderr
      KONG_ADMIN_ERROR_LOG: /dev/stderr
    volumes:
      - ./deploy/kong/kong.yml:/kong/kong.yml:ro
    ports:
      - "8000:8000"
    depends_on:
      - user-service
      - product-service
      - rental-service
    networks: [rental-net]

volumes:
  user-db-data:
  product-db-data:
  rental-db-data:

networks:
  rental-net:
    name: rental-net
    driver: bridge
```

> **สถานะปัจจุบัน (2026-09-21):** `product-service`/`rental-service` ยังไม่มีโค้ด ใน
> docker-compose.yml จริงตอนนี้ `kong.depends_on` จึงมีแค่ `user-service` — คนที่เพิ่ม
> product-service/rental-service เข้า compose ทีหลัง ต้องเพิ่มชื่อ service นั้นเข้า
> `kong.depends_on` ด้วย
>
> คนที่เพิ่ม `product-service`/`rental-service` เข้า Kong ทีหลัง: ให้ตั้ง path
> health check ของแต่ละ service เป็นค่าที่ไม่ซ้ำกัน (เช่น `/product/health`,
> `/rental/health`) ห้ามใช้ `/health` ร่วมกันซ้ำ — เดิม `product-public`/
> `rental-public` เคยประกาศ `/health` เหมือนกันทั้งคู่ ทำให้ Kong เลือก resolve
> ไปที่ `user-service` เสมอ (route collision) จึงถูกลบออกไปแล้วใน `kong.yml`

### 9.3 คำสั่งรันทั้งระบบ

```bash
cp .env.example .env
docker compose up -d --build
docker compose ps          # เช็คว่าทุกตัว healthy
curl http://localhost:8081/health   # ตรง — debug เท่านั้น
curl http://localhost:8082/health   # ตรง — debug เท่านั้น
curl http://localhost:8083/health   # ตรง — debug เท่านั้น
curl http://localhost:8000/api/v1/products   # ผ่าน Kong — ทางที่ client ควรใช้
```

### 9.4 พัฒนาแยกเครื่อง

- แต่ละคนรันแค่ service + db ของตัวเองระหว่างพัฒนา
- user-service ไม่พึ่งใคร รันได้เลย
- product / rental ต้องการ JWT ทดสอบ → โคลน user-service มารัน หรือขอ test token จากสุรเชษฐ์
- นัด integrate รวม (ทั้ง 3 service) อย่างน้อยสัปดาห์ละ 1 ครั้ง
- ทดสอบว่า token ปลอม/หมดอายุถูก reject จริง ต้องยิงผ่าน Kong (`:8000`) เท่านั้น — solo dev ที่ทดสอบ
  business logic ปกติไม่ต้องผ่าน Kong ก็ได้ (ยิง mock claims ตรง port service ได้เลย)

---

## 10. Git Workflow และการแก้สัญญา

### 10.1 โครงสร้าง repo

**ตัวเลือก A — monorepo (แนะนำสำหรับส่งอาจารย์):**

```
equipment-rental-system/
├── user-service/
├── product-service/
├── rental-service/
├── deploy/            # docker-compose รวม, Postman collection
├── CONTRACT.md        # ไฟล์นี้
├── .env.example
└── README.md
```

**ตัวเลือก B — polyrepo:** แยก 4 repo (3 service + deploy)

### 10.2 กติกา

1. แต่ละคนทำงานบน branch ตัวเอง เช่น `feature/user-login` แล้ว PR เข้า `main`
2. ห้าม push ตรงเข้า `main`
3. คนอื่นรีวิว PR อย่างน้อย 1 คนก่อน merge
4. `.env` จริงอยู่ใน `.gitignore` — commit แค่ `.env.example`

### 10.3 การแก้ CONTRACT.md

- ถ้าจะเปลี่ยน endpoint / response / claim ที่คนอื่นเรียกใช้ → **แจ้งกลุ่มก่อน**
- เปิด PR แก้ `CONTRACT.md` พร้อมกับโค้ด ให้ทุกคน approve
- เวอร์ชันสัญญาไว้ท้ายไฟล์ (changelog)

---

## 11. ข้อตกลงการตั้งชื่อ

| หัวข้อ | ข้อตกลง | ตัวอย่าง |
|---|---|---|
| JSON key | `snake_case` | `price_per_day`, `created_at` |
| วันที่-เวลาใน response | ISO 8601 UTC (`Z`) | `2026-09-06T09:00:00Z` |
| วันที่ (ไม่มีเวลา) ใน request | `YYYY-MM-DD` | `2026-09-10` |
| primary key | UUID v4 string | `9b1c7c2e-...` |
| ชื่อ path | พหูพจน์ คั่นด้วย `/` | `/products`, `/users/{id}/login-logs` |
| path parameter | `{id}` ใน doc, `:id` ในโค้ด Gin | `/rentals/{id}/return` |
| เงิน/ราคา | ตัวเลข (บาท) ทศนิยม 2 ตำแหน่ง | `150.00` |
| enum/status | ตัวพิมพ์เล็ก | `available`, `pending`, `returned` |

---

## Changelog

| เวอร์ชัน | วันที่ | การเปลี่ยนแปลง | โดย |
|---|---|---|---|
| v1 (ร่าง) | 2026-09-06 | ร่างฉบับแรก | สุรเชษฐ์ |
| v2 | 2026-09-21 | เพิ่ม Kong API Gateway เป็น single entry point, ย้าย JWT verify ไป gateway | สุรเชษฐ์ |
| v3 | 2026-09-26 | รับ rental-service เข้า docker-compose/Kong (`/rental/health`), แก้ endpoint table §8.3, เพิ่มข้อกำหนด atomic update ให้ §8.2 | สุรเชษฐ์ |

---

## ✅ Checklist ให้ทีมยืนยันร่วมกัน

- [ ] พอร์ตและชื่อ service (ข้อ 1)
- [ ] ค่า `JWT_SECRET` / `INTERNAL_API_KEY` ร่วมกัน (ข้อ 2)
- [ ] รูปแบบ response envelope + รหัส error (ข้อ 4)
- [ ] ยืนยันว่า product/rental ไม่ต้อง verify signature เอง — Kong ตรวจให้แล้วผ่าน `jwt` plugin, service แค่ decode claims (ข้อ 5.3)
- [ ] ยืนยันการใช้ Kong เป็น entry point + ย้าย JWT verify ไป gateway (ข้อ 1, 5.2, 5.3, 9.2)
- [ ] ตารางสิทธิ์ RBAC (ข้อ 6.2)
- [ ] ใครอัปเดตสถานะสินค้าตอนเช่า/คืน — product หรือ rental (ข้อ 7.1)
- [ ] เอกพลเติม endpoint product-service (ข้อ 8.2)
- [ ] วิษณุพงศ์เติม endpoint rental-service (ข้อ 8.3)
- [ ] เอกพลยืนยัน `PATCH /products/{id}/status` จะทำ atomic/conditional update ตามที่ระบุใหม่ในข้อ 8.2
- [ ] monorepo หรือ polyrepo (ข้อ 10.1)

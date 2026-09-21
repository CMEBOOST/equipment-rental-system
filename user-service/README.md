# user-service

ระบบจัดการผู้ใช้งาน (JWT & RBAC) — ผู้รับผิดชอบ: สุรเชษฐ์ สีสา

ดูเอกสารออกแบบที่ [../docs/user-management-service-design.md](../docs/user-management-service-design.md)
และสัญญาระหว่าง service ที่ [../CONTRACT.md](../CONTRACT.md)

## รันด้วย Docker (วิธีหลัก)

จาก **root ของ repo**:

```bash
cp .env.example .env
docker compose up -d --build
```

ตรวจสอบว่าขึ้นแล้ว:

```bash
curl http://localhost:8081/health
# {"data":{"db":"up","service":"user-service","status":"ok"},"success":true}
```

**ไม่ต้องรัน migration เอง** — service จะรัน migration ทั้งหมดใน `migrations/`
ให้อัตโนมัติตอน start (ดู `internal/db/migrate.go` ซึ่งถูกเรียกจาก
`cmd/api/main.go` ก่อนเปิด HTTP server) ดังนั้น `docker compose up` บน volume
เปล่าก็ใช้งานได้ทันที ตาราง + role seed (admin/staff/customer) + บัญชี admin/staff
เริ่มต้นพร้อมใช้ (ดูหัวข้อถัดไป)

## บัญชี admin/staff เริ่มต้น (seed มาให้แล้ว)

`POST /auth/register` ตั้ง role เป็น `customer` เสมอ ไม่มีทางเลือก — ถ้าไม่ seed
account ไว้ก่อน จะไม่มีทางสร้าง admin คนแรกได้เลย (endpoint ที่ตั้ง role อื่นได้ต้องเป็น
admin ก่อนถึงจะเรียกได้) migration `000004_seed_admin_staff.up.sql` เลยสร้างให้ 2 บัญชีนี้
ไว้ตั้งแต่ตอน migrate:

| Role | Email | Username | Password |
|---|---|---|---|
| admin | `admin@equipment-rental.local` | `admin` | `Admin123!` |
| staff | `staff@equipment-rental.local` | `staff` | `Staff123!` |

```bash
curl -X POST http://localhost:8081/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@equipment-rental.local","password":"Admin123!"}'
```

เอา `access_token` ที่ได้ไปใช้เรียก endpoint ที่ต้อง role admin (เช่น `GET /users`,
`PATCH /users/:id/role`) ตั้ง admin/staff คนอื่นเพิ่มผ่าน API ได้ตามปกติจากจุดนี้

> ⚠️ **รหัสผ่านนี้เป็นค่า dev เท่านั้น** ใครก็อ่านไฟล์นี้ได้ก็รู้รหัสผ่าน admin — ห้ามใช้ค่านี้
> จริงถ้าเอาไป deploy จริง (เปลี่ยนรหัสผ่านผ่าน `PUT /me/password` ทันทีหลัง deploy)

ลองสมัครและเข้าสู่ระบบ (บัญชีปกติ จะได้ role `customer`):

```bash
curl -X POST http://localhost:8081/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"someone@example.com","username":"someone","password":"Passw0rd1","full_name":"Some One"}'

curl -X POST http://localhost:8081/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"someone@example.com","password":"Passw0rd1"}'
```

เริ่มใหม่หมด (ลบข้อมูลทั้งหมด แล้วให้ migration รันใหม่ตั้งแต่ต้น):

```bash
docker compose down -v && docker compose up -d --build
```

## รันแบบ local (ไม่ใช้ Docker)

ต้องมี Postgres ที่เข้าถึงได้ตามค่าใน `.env` แล้วรันจากโฟลเดอร์ `user-service/`:

```bash
cp .env.example .env
go run ./cmd/api
```

`MIGRATIONS_PATH` ค่า default คือ `migrations` (relative กับ working directory)
ส่วนใน Docker image ตั้งเป็น `/migrations`

## ตัวแปรสภาพแวดล้อม

| ตัวแปร | ค่า default | หมายเหตุ |
| --- | --- | --- |
| `APP_PORT` | `8081` | |
| `DB_HOST` / `DB_PORT` / `DB_USER` / `DB_PASSWORD` / `DB_NAME` | `localhost` / `5432` / `user_service` / `secret` / `user_db` | ใน docker-compose ถูก map มาจากตัวแปรที่ขึ้นต้นด้วย `USER_` |
| `JWT_SECRET` | — | **บังคับ** และต้องยาวอย่างน้อย 32 ตัวอักษร ไม่ตั้ง service จะไม่ start |
| `JWT_ACCESS_TTL` | `15m` | |
| `JWT_REFRESH_TTL` | `168h` | |
| `INTERNAL_API_KEY` | — | **บังคับ** ใช้กับ header `X-Internal-Key` ของ `POST /auth/verify` |
| `BCRYPT_COST` | `12` | |
| `CORS_ORIGIN` | `http://localhost:3000` | origin เดียวที่เบราว์เซอร์เรียกได้ |
| `MIGRATIONS_PATH` | `migrations` | โฟลเดอร์ไฟล์ `.sql` |

## Endpoints (20 ทั้งหมด)

Base path ทุกอันคือ `/api/v1` ยกเว้น `/health` — ยิงตรง `localhost:8081` หรือผ่าน
Kong (`localhost:8000`) ก็ได้ (ดู [../deploy/kong/README.md](../deploy/kong/README.md))
คอลัมน์ "สิทธิ์" ว่าง = public ไม่ต้องมี token

| Method | Path | สิทธิ์ | Body | หมายเหตุ |
| --- | --- | --- | --- | --- |
| GET | `/health` | — | — | เช็ค DB connection ด้วย |
| POST | `/auth/register` | — | `{"email","username","password","full_name","phone"}` | `password` ≥8 ตัว มีทั้งตัวอักษร+เลข, `full_name`/`phone` ไม่บังคับ |
| POST | `/auth/login` | — | `{"email","password"}` | คืน `access_token` (15 นาที) + `refresh_token` (opaque, 7 วัน) |
| POST | `/auth/refresh` | — | `{"refresh_token"}` | ออก access token ใหม่, หมุน refresh token ใหม่ |
| POST | `/auth/logout` | token | `{"refresh_token"}` | revoke session นั้น |
| POST | `/auth/verify` | `X-Internal-Key` header | `{"token"}` | สำหรับ service อื่นเรียก ไม่ใช่ client — คืน `{active,user_id,email,username,role,expires_at}` |
| GET | `/me` | token | — | ข้อมูลตัวเอง |
| PUT | `/me` | token | `{"full_name","phone"}` | ทั้งสอง field ไม่บังคับ ส่งเฉพาะที่จะแก้ก็ได้ |
| PUT | `/me/password` | token | `{"current_password","new_password"}` | `new_password` ≥8 ตัว — revoke session อื่นทั้งหมด |
| GET | `/me/login-logs` | token | — | ประวัติ login ตัวเอง — query: `page`, `limit` (default 1/20) |
| GET | `/me/sessions` | token | — | refresh token ที่ยัง active อยู่ |
| GET | `/users` | admin | — | query: `q`, `role`, `is_active`, `sort`, `order`, `page`, `limit` |
| POST | `/users` | admin | `{"email","username","password","full_name","phone","role","is_active"}` | `role` ต้องเป็น `admin`/`staff`/`customer`, `is_active` ไม่บังคับ (default true) |
| GET | `/users/:id` | admin, staff | — | ดูผู้ใช้คนเดียว |
| PUT | `/users/:id` | admin | `{"email","username","full_name","phone"}` | ทุก field ไม่บังคับ ส่งเฉพาะที่จะแก้ |
| DELETE | `/users/:id` | admin | — | soft delete (email/username เอาไปสมัครใหม่ได้ทันที) |
| PATCH | `/users/:id/role` | admin | `{"role"}` | `admin`/`staff`/`customer` — revoke session เดิม, ห้าม demote ตัวเอง |
| PATCH | `/users/:id/status` | admin | `{"is_active"}` | เปิด/ปิดบัญชี |
| GET | `/users/:id/login-logs` | admin | — | query: `success` (true/false), `page`, `limit` |
| GET | `/roles` | admin, staff | — | รายชื่อ role ทั้งหมด (admin/staff/customer) |

ทุก response ใช้ envelope เดียวกัน: `{"success":true,"data":...}` หรือ
`{"success":false,"error":{"code":...,"message":...,"details":...}}` — รายการที่มี
pagination จะมี `"meta":{"page","limit","total","total_pages"}` เพิ่มมา

## เทสต์

```bash
cd user-service
go test ./...
```

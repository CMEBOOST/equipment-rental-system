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
เปล่าก็ใช้งานได้ทันที ตาราง + role seed (admin/staff/customer) พร้อมใช้

ลองสมัครและเข้าสู่ระบบ:

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

| Method | Path | สิทธิ์ | หมายเหตุ |
| --- | --- | --- | --- |
| GET | `/health` | — | เช็ค DB connection ด้วย |
| POST | `/auth/register` | — | body: `email`, `username`, `password` (≥8 ตัว มีทั้งตัวอักษร+เลข), `full_name`, `phone` (2 ตัวหลังไม่บังคับ) |
| POST | `/auth/login` | — | body: `email`, `password` → คืน `access_token` (15 นาที) + `refresh_token` (opaque, 7 วัน) |
| POST | `/auth/refresh` | — | body: `refresh_token` → ออก access token ใหม่, หมุน refresh token ใหม่ |
| POST | `/auth/logout` | token | body: `refresh_token` → revoke session นั้น |
| POST | `/auth/verify` | `X-Internal-Key` header | สำหรับ service อื่นเรียก ไม่ใช่ client — body: `token` → คืน active/user_id/email/username/role/expires_at |
| GET | `/me` | token | ข้อมูลตัวเอง |
| PUT | `/me` | token | แก้ `full_name`/`phone` |
| PUT | `/me/password` | token | body: `old_password`, `new_password` → revoke session อื่นทั้งหมด |
| GET | `/me/login-logs` | token | ประวัติ login ตัวเอง — query: `page`, `limit` (default 1/20) |
| GET | `/me/sessions` | token | refresh token ที่ยัง active อยู่ |
| GET | `/users` | admin | รายชื่อผู้ใช้ทั้งหมด — query: `q`, `role`, `is_active`, `sort`, `order`, `page`, `limit` |
| POST | `/users` | admin | สร้างผู้ใช้ใหม่โดย admin (กำหนด role ได้ทันที) |
| GET | `/users/:id` | admin, staff | ดูผู้ใช้คนเดียว |
| PUT | `/users/:id` | admin | แก้ข้อมูลผู้ใช้คนอื่น |
| DELETE | `/users/:id` | admin | soft delete (email/username เอาไปสมัครใหม่ได้ทันที) |
| PATCH | `/users/:id/role` | admin | เปลี่ยน role — revoke session เดิม, ห้าม demote ตัวเอง |
| PATCH | `/users/:id/status` | admin | เปิด/ปิดบัญชี (`is_active`) |
| GET | `/users/:id/login-logs` | admin | ประวัติ login ของคนอื่น — query: `success` (true/false), `page`, `limit` |
| GET | `/roles` | admin, staff | รายชื่อ role ทั้งหมด (admin/staff/customer) |

ทุก response ใช้ envelope เดียวกัน: `{"success":true,"data":...}` หรือ
`{"success":false,"error":{"code":...,"message":...,"details":...}}` — รายการที่มี
pagination จะมี `"meta":{"page","limit","total","total_pages"}` เพิ่มมา

## เทสต์

```bash
cd user-service
go test ./...
```

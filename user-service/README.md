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

## เทสต์

```bash
cd user-service
go test ./...
```

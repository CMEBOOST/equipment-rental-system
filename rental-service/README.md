# rental-service

ระบบจัดการการเช่า — ผู้รับผิดชอบ: วิษณุพงศ์ บัวเขียว (67114540509)

บริการนี้จัดการคำขอเช่า การสร้างรายการเช่า การอนุมัติ และการคืนสินค้า โดยใช้ PostgreSQL
เป็นฐานข้อมูลของตัวเอง และเรียก product-service/user-service ผ่าน HTTP ภายใน Docker network
ตาม [CONTRACT.md](../CONTRACT.md)

**JWT:** Kong เป็นคนตรวจลายเซ็นและวันหมดอายุของ token ก่อนส่งต่อมาที่ service นี้แล้ว
`middleware.RequireAuth()` จึงแค่ decode payload ของ JWT (ไม่ verify signature ซ้ำ) เพื่อดึง
`sub` (user id) และ `role` ใส่ไว้ใน context — ดู [CONTRACT.md §8.3](../CONTRACT.md)

## Endpoints

Base path คือ `/api/v1` ทุก endpoint นอกจาก health ต้องมี `Authorization: Bearer <token>`
ที่ผ่าน Kong แล้ว (`X-Internal-Key` เป็นเรื่องของการเรียกระหว่าง service เท่านั้น ไม่ใช่ client)

| Method | Path | สิทธิ์ | Body | หมายเหตุ |
| --- | --- | --- | --- | --- |
| GET | `/health` (ผ่าน Kong คือ `/rental/health`) | public | — | เช็ค DB connection ด้วย |
| GET | `/rentals` | admin, staff | — | list ทั้งหมด — query: `status`, `sort`, `order`, `page`, `limit` (default 1/20, สูงสุด 100) |
| GET | `/rentals/{id}` | admin, staff, เจ้าของรายการ | — | เจ้าของรายการดูของตัวเองได้ คนอื่นที่ไม่ใช่ admin/staff โดน 403 |
| POST | `/rentals` | admin, staff | `{"user_id","product_id","start_date","due_date"}` | สร้างแทนลูกค้า สถานะเริ่มที่ `active` ทันที (จอง product-service ให้เลย) |
| POST | `/rentals/request` | customer | `{"product_id","start_date","due_date"}` | ลูกค้าขอเช่าเอง สถานะเริ่มที่ `pending` (ยังไม่แตะสถานะสินค้า จนกว่าจะ approve) |
| PATCH | `/rentals/{id}/approve` | admin, staff | — | `pending` → `active` — เช็คสินค้าว่างแล้วสั่งจองที่ product-service |
| PATCH | `/rentals/{id}/return` | admin, staff | `{"return_date"}` | `active` → `returned` — คืนสถานะสินค้าเป็น `available` |
| GET | `/me/rentals` | authenticated | — | รายการเช่าของตัวเอง — query เหมือน `/rentals` |

`start_date`/`due_date`/`return_date` ใช้รูปแบบ `YYYY-MM-DD`, `due_date` ต้องมากกว่า
`start_date`, `return_date` ต้องไม่ก่อน `start_date` การคิดราคาใช้
`price_per_day × (due_date - start_date)` (วัน) และบันทึกเป็น `total_price` ของรายการเช่า
ตอนสร้าง จึงไม่เปลี่ยนตามราคาสินค้าในภายหลัง

### สถานะของรายการเช่า (`status`)

```
pending ──approve──▶ active ──return──▶ returned
   │
   └──(ยกเลิก, ยังไม่มี endpoint ใน API วันนี้)──▶ cancelled
```

`cancelled` มีอยู่ใน DB constraint (`model.StatusCancelled`) แต่ยังไม่มี endpoint ให้เปลี่ยนเป็น
สถานะนี้ — เผื่อไว้สำหรับอนาคต

### รูปแบบ response และ error code

Response สำเร็จ: `{"success":true,"data":...}` (list เพิ่ม `"meta":{"page","limit","total","total_pages"}`)
Response ผิดพลาด: `{"success":false,"error":{"code","message","details"}}`

| HTTP | `code` | เกิดเมื่อ |
| --- | --- | --- |
| 400 | `VALIDATION_ERROR` | body ไม่ผ่าน binding, วันที่ผิดรูปแบบ/ผิดเงื่อนไข |
| 401 | `UNAUTHENTICATED` | ไม่มี token หรือ token ถอดรหัสไม่ได้ |
| 403 | `FORBIDDEN` | role ไม่มีสิทธิ์ หรือไม่ใช่เจ้าของรายการ |
| 403 | `ACCOUNT_DISABLED` | user-service บอกว่าบัญชีถูกปิดใช้งาน |
| 404 | `NOT_FOUND` | ไม่พบรายการเช่า หรือไม่พบสินค้า |
| 409 | `CONFLICT` | สินค้าไม่ว่าง หรือรายการเช่าอยู่ผิดสถานะสำหรับ action นี้ |
| 503 | `INTERNAL_ERROR` | เรียก product-service/user-service ไม่สำเร็จ (dependency ล่ม) |
| 500 | `INTERNAL_ERROR` | error อื่นที่ไม่คาดคิด |

## การทำงานร่วมกับบริการอื่น

เรียกตรงถึง service อื่นด้วย Docker service name (ไม่ผ่าน Kong) พร้อม header `X-Internal-Key`
เท่านั้น (ไม่มี Bearer token):

- `GET /products/{id}`: ตรวจสินค้า สถานะ และ `price_per_day`
- `PATCH /products/{id}/status`: เปลี่ยนเป็น `rented` เมื่อเริ่มเช่า และ `available` เมื่อตีกลับหรือคืน
  — endpoint นี้ต้องเป็น atomic/conditional update ฝั่ง product-service (ดู [CONTRACT.md §8.2](../CONTRACT.md))
  ไม่งั้นสองคนอาจเช่าสินค้าชิ้นเดียวกันพร้อมกันได้ ตอบ `409` เมื่อสถานะไม่ตรงกับที่คาด
  แล้ว `ProductClient.SetStatus` จะแปลงเป็น `ErrProductUnavailable` (`CONFLICT`) ให้เอง
- `POST /auth/verify`: ตรวจ access token ของผู้ที่สั่งให้เกิดการเปลี่ยนสถานะ เพื่อยืนยันว่าบัญชียังใช้งานได้

ทุก action ที่แตะสถานะสินค้า (`Create`/`Approve`/`Return`) ใช้ pattern reserve-then-compensate:
เปลี่ยนสถานะสินค้าก่อน แล้วค่อยเขียนฐานข้อมูลตัวเอง — ถ้าเขียน DB ไม่สำเร็จจะเรียก
`SetStatus` ย้อนกลับให้สินค้ากลับไปสถานะเดิมทันที

## Environment variables

| ตัวแปร | ค่า default | หมายเหตุ |
| --- | --- | --- |
| `APP_PORT` | `8083` | ใน docker-compose ถูก map มาจาก `RENTAL_APP_PORT` |
| `DB_HOST` / `DB_PORT` / `DB_USER` / `DB_PASSWORD` / `DB_NAME` | `localhost` / `5432` / `rental_service` / `secret` / `rental_db` | ใน docker-compose ถูก map มาจากตัวแปรที่ขึ้นต้นด้วย `RENTAL_` |
| `INTERNAL_API_KEY` | — | **บังคับ** service จะไม่ start ถ้าไม่ตั้ง ใช้เป็น `X-Internal-Key` เวลาเรียก product-service/user-service |
| `PRODUCT_SERVICE_URL` | `http://product-service:8082/api/v1` | |
| `USER_SERVICE_URL` | `http://user-service:8081/api/v1` | |
| `CORS_ORIGIN` | `http://localhost:3000` | origin เดียวที่เบราว์เซอร์เรียกได้ |
| `MIGRATIONS_PATH` | `migrations` | โฟลเดอร์ไฟล์ `.sql` |

ค่า default ทั้งหมดตรงกับที่ตั้งไว้แล้วใน [`.env.example`](../.env.example) (ตัวแปรฝั่ง compose
ขึ้นต้นด้วย `RENTAL_`, ตัวแปรที่ตัว service อ่านจริงไม่มี prefix — docker-compose เป็นคน map ให้)

## รันด้วย Docker

จาก root ของ repository:

```bash
cp .env.example .env
docker compose up -d --build
curl http://localhost:8000/rental/health
```

`/rental/health` เป็น public route ผ่าน Kong ส่วน API ปกติเรียกผ่าน `localhost:8000`
(ตรงเข้า container โดยไม่ผ่าน Kong ได้ที่ `localhost:8083/health` เผื่อ debug)

## รันแบบ local (ไม่ใช้ Docker)

ต้องมี PostgreSQL รันอยู่แล้วเอง (หรือรันแค่ `rental-db` ด้วย
`docker compose up -d rental-db`) และตั้ง env vars ด้านบนเอง (หรือสร้างไฟล์ `.env`
ในโฟลเดอร์นี้ — `config.Load()` โหลดด้วย `godotenv` อัตโนมัติ):

```bash
cd rental-service
cp .env.example .env   # แก้ DB_HOST เป็น localhost ถ้า Postgres รันนอก Docker
go run ./cmd/api
```

`main.go` รัน migration (`migrations/*.sql`) ให้อัตโนมัติทุกครั้งที่ start ก่อนเปิดรับ request
(idempotent — รันซ้ำได้ไม่พัง)

## Testing

```bash
cd rental-service
go test ./... -v -count=1
```

ใช้ `github.com/glebarez/sqlite` (in-memory, pure Go ไม่ต้องมี CGO) แทน Postgres จริงตอนเทส
และ `httptest.NewServer` mock การเรียก product-service/user-service — ไม่ต้องมี service อื่น
รันอยู่ก็เทสผ่านได้ครบ ครอบคลุม repository, HTTP client, middleware, service, handler, และ router

## โครงสร้างโปรเจกต์

```
rental-service/
├── cmd/api/main.go              # entry point — โหลด config, ต่อ DB, รัน migration, start server
├── internal/
│   ├── client/                  # HTTP client เรียก product-service / user-service
│   ├── config/                  # โหลด env vars
│   ├── db/                      # เชื่อมต่อ DB + รัน migration
│   ├── dto/                     # request body structs
│   ├── handler/                 # HTTP handler (Gin) + mapError
│   ├── middleware/               # RequireAuth (decode JWT), RequireRole, CORS
│   ├── model/                   # Rental struct + GORM tag
│   ├── repository/              # query ฐานข้อมูล (filter/sort/pagination)
│   ├── router/                  # ผูก route + middleware เข้าด้วยกัน
│   └── service/                 # business logic (reserve-then-compensate)
└── migrations/                  # golang-migrate .sql files
```

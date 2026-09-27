# Equipment Rental System

ระบบเช่าอุปกรณ์แบบ Backend-only เขียนด้วย **Go + PostgreSQL** สถาปัตยกรรม **Microservices**
แยกเป็น 3 service อิสระต่อกัน (user, product, rental) แต่ละตัวมีฐานข้อมูลเป็นของตัวเอง
และมี **Kong API Gateway** เป็น single entry point ให้ client เรียกเข้ามาทางเดียว

## สมาชิกและความรับผิดชอบ

| สมาชิก | รหัสนักศึกษา | Service | โฟลเดอร์ | สถานะ |
|---|---|---|---|---|
| สุรเชษฐ์ สีสา | 67114540583 | ระบบจัดการผู้ใช้งาน (JWT & RBAC) + Kong Gateway | `user-service/`, `deploy/kong/` | ✅ เสร็จแล้ว |
| เอกพล แรกเรียง | 67114540666 | ระบบจัดการสินค้า | `product-service/` | ✅ เสร็จแล้ว |
| วิษณุพงศ์ บัวเขียว | 67114540509 | ระบบจัดการการเช่า | `rental-service/` | ✅ เสร็จแล้ว |

## คุณสมบัติหลัก

- **จัดการผู้ใช้งานและสิทธิ์ (user-service)** — สมัคร/เข้าสู่ระบบด้วย JWT (access + refresh
  token), RBAC 3 ระดับ (`admin` / `staff` / `customer`), จัดการบัญชี/บทบาท/เปิด-ปิดผู้ใช้,
  ประวัติการเข้าสู่ระบบ
- **จัดการสินค้า (product-service)** — CRUD สินค้าและหมวดหมู่, ค้นหา/กรอง/แบ่งหน้า, สถานะสินค้า
  3 แบบ (`available` / `rented` / `maintenance`) พร้อม atomic status update กัน race condition
  ตอนมีคนเช่าสินค้าชิ้นเดียวกันพร้อมกัน
- **จัดการการเช่า (rental-service)** — ลูกค้าขอเช่าเอง หรือ staff สร้างแทนได้, อนุมัติ/คืนสินค้า,
  คำนวณราคาอัตโนมัติจากจำนวนวัน, ผูกสถานะสินค้าที่ product-service ให้ตรงกับสถานะการเช่าเสมอ
- **API Gateway (Kong)** — ตรวจ JWT signature/expiry ให้ทุก protected route ก่อนส่งต่อไปยัง
  service ปลายทาง ทำให้ service ภายในไม่ต้องถือ secret ร่วมกัน
- **Docker Compose** พร้อมใช้งานทั้งระบบด้วยคำสั่งเดียว รวม migration อัตโนมัติและ seed
  บัญชี admin/staff เริ่มต้น

## สถาปัตยกรรม

```
Client ──► Kong (proxy :8000) ──► user-service    :8081  (ถือ JWT_SECRET, ออก token)
                               ├─► product-service :8082  (decode-only, ไม่ถือ secret)
                               └─► rental-service  :8083  (decode-only, ไม่ถือ secret)

rental-service ──► product-service               : เรียกตรง ไม่ผ่าน Kong (service-to-service)
rental-service ──► user-service (/auth/verify)   : เรียกตรง ไม่ผ่าน Kong
```

- **Client เรียกผ่าน Kong (`:8000`) เท่านั้น** — Kong ตรวจ JWT signature/expiry ให้ทุก
  protected route ก่อนถึง service ปลายทาง พอร์ต 8081-8083 เปิดไว้เพื่อ debug ตรงเท่านั้น
- **user-service เท่านั้นที่ถือ `JWT_SECRET`** และเป็นคนออก token — product-service และ
  rental-service แค่ decode payload อ่าน claims เอา ไม่ต้อง verify signature ซ้ำ
- แต่ละ service มีฐานข้อมูล PostgreSQL เป็นของตัวเอง ไม่มี FK ข้าม database
- Kong รันแบบ **DB-less** (ไม่มี Kong Postgres/Admin API) — config อยู่ในไฟล์เดียวคือ
  [deploy/kong/kong.yml](deploy/kong/kong.yml)

## Tech Stack

- **ภาษา/Framework:** Go, [Gin](https://github.com/gin-gonic/gin), [GORM](https://gorm.io)
- **ฐานข้อมูล:** PostgreSQL 16, migration ด้วย [golang-migrate](https://github.com/golang-migrate/migrate)
- **API Gateway:** [Kong](https://konghq.com/) (DB-less mode)
- **Auth:** JWT (HS256), bcrypt password hashing
- **Infra:** Docker / Docker Compose

## โครงสร้างโปรเจกต์

```
equipment-rental-system/
├── user-service/       # :8081 → user-db     — auth, users, roles (20 endpoints)
├── product-service/    # :8082 → product-db  — สินค้าและหมวดหมู่
├── rental-service/     # :8083 → rental-db   — คำขอเช่า/อนุมัติ/คืนสินค้า
├── deploy/
│   └── kong/            # kong.yml + smoke test scripts + README
├── docs/
│   ├── openapi/          # OpenAPI spec ของทั้ง 3 service — เสิร์ฟเป็น Swagger UI ผ่าน Kong ที่ /docs
│   ├── user-management-service-design.md
│   └── superpowers/     # เอกสารออกแบบ/แผนการ implement
├── CONTRACT.md          # ข้อตกลงระหว่าง service (พอร์ต, JWT, response format, RBAC)
├── .env.example
└── docker-compose.yml   # รันครบทั้ง 3 service + Kong ด้วยคำสั่งเดียว
```

## เริ่มต้นใช้งาน

**Requirements:** Docker และ Docker Compose

```bash
cp .env.example .env
docker compose up -d --build
docker compose ps    # user-service, user-db, product-service, product-db, rental-service, rental-db, docs, kong ต้อง healthy/running
```

> ถ้า service ไหน exit ไปเองตอนเพิ่ง `up` ครั้งแรก (เช็คด้วย `docker compose ps -a`) — ส่วนใหญ่เป็นเพราะ
> Postgres ของมันยัง "starting up" ไม่ทันตอน service เริ่ม migrate สั่ง `docker compose up -d <service>`
> ซ้ำอีกรอบพอ และถ้ายิง endpoint แล้วได้ `503`/`name resolution failed` ทั้งที่ container ที่ควรตอบ
> รันอยู่จริง — Kong cache DNS ไว้ตั้งแต่ตอน container นั้นยังไม่ขึ้น สั่ง
> `docker compose restart kong` แก้ได้

ตรวจสอบว่าระบบทำงานผ่าน Kong:

```bash
curl http://localhost:8000/health          # -> 200 (user-service)
curl http://localhost:8000/rental/health   # -> 200 (rental-service)
curl http://localhost:8000/api/v1/products # -> 401 ไม่มี token (product-service ยังไม่มี public /health route แยก ดู deploy/kong/README.md)
```

เปิด **Swagger UI** ที่ <http://localhost:8000/docs/> (เลือก service จาก dropdown มุมบนขวา) เพื่อดู/ทดลองยิง
API จริงของทั้ง 3 service ผ่าน Kong ได้ทันที — spec อยู่ที่ `docs/openapi/*.yaml`

รัน smoke test ทั้งระบบ:

```bash
./deploy/kong/smoke-test.sh                     # user-service ผ่าน Kong
./deploy/kong/rental-smoke-test.sh              # rental-service ผ่าน Kong
./deploy/kong/product-rental-e2e-smoke-test.sh  # flow เต็ม: rent -> approve -> return ข้ามทั้ง 3 service
```

user-service seed บัญชี admin/staff เริ่มต้นไว้ให้แล้ว (ดูรหัสผ่านที่
[user-service/README.md](user-service/README.md#บัญชี-adminstaff-เริ่มต้น-seed-มาให้แล้ว))
ใช้ login เพื่อขอ token แล้วเรียก endpoint อื่นต่อได้ทันที:

```bash
curl -X POST http://localhost:8000/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@equipment-rental.local","password":"Admin123!"}'
```

## เอกสาร API

ทุก response ใช้ envelope เดียวกัน: `{"success":true,"data":...}` หรือ
`{"success":false,"error":{"code","message","details"}}` (list เพิ่ม `"meta":{"page","limit","total","total_pages"}`)

| Service | Base path (ผ่าน Kong) | สรุป | รายละเอียด |
|---|---|---|---|
| user-service | `/api/v1/auth/*`, `/api/v1/me`, `/api/v1/users`, `/api/v1/roles` | สมัคร/login/refresh, จัดการโปรไฟล์, จัดการผู้ใช้และบทบาท (admin/staff/customer) | [user-service/README.md](user-service/README.md) |
| product-service | `/api/v1/products`, `/api/v1/categories` | CRUD สินค้า/หมวดหมู่, ค้นหา/กรอง/แบ่งหน้า, เปลี่ยนสถานะสินค้า | [product-service/README.md](product-service/README.md) |
| rental-service | `/api/v1/rentals`, `/api/v1/me/rentals` | สร้าง/อนุมัติ/คืนรายการเช่า, ประวัติการเช่าของตัวเอง | [rental-service/README.md](rental-service/README.md) |

**Swagger UI (แบบ interactive):** <http://localhost:8000/docs/> — เลือก service จาก dropdown แล้ว
ลองยิง request จริงผ่าน Kong ได้จากในหน้านั้นเลย (กด "Authorize" ใส่ Bearer token ที่ได้จาก
`/api/v1/auth/login`) ไฟล์ spec อยู่ที่ [docs/openapi/](docs/openapi/) เขียนขึ้นจากโค้ด/CONTRACT.md
จริง — ถ้า endpoint เปลี่ยนต้องแก้ไฟล์ spec ตามด้วย (ไม่ได้ generate อัตโนมัติจากโค้ด)

ดู path ทั้งหมดที่ Kong ประกาศไว้และวิธีทดสอบผ่าน gateway ที่
[deploy/kong/README.md](deploy/kong/README.md) และดูข้อตกลงกลางระหว่าง service (RBAC,
response format, การตรวจ JWT แบบ decode-only, atomic update) ที่ [CONTRACT.md](CONTRACT.md)

## Environment Variables

ดูค่าเริ่มต้นทั้งหมดที่ `.env.example` (ค่าที่ใช้ร่วมกันทุก service อยู่บนสุด, ค่าเฉพาะ
service แยกเป็นบล็อกด้านล่าง) และรายละเอียดใน [CONTRACT.md §2](CONTRACT.md)

| ตัวแปร | ใช้ที่ไหน |
|---|---|
| `JWT_SECRET`, `JWT_ACCESS_TTL`, `JWT_REFRESH_TTL` | user-service (ออก token) + `deploy/kong/kong.yml` (ต้องตรงกัน, ดู [deploy/kong/README.md](deploy/kong/README.md)) |
| `INTERNAL_API_KEY` | เรียก `POST /auth/verify` ที่ user-service และ endpoint ภายในของ product-service จาก service อื่น |
| `USER_*` / `PRODUCT_*` / `RENTAL_*` | ตัวแปร DB/port เฉพาะแต่ละ service (namespace กันชนกันใน `.env` เดียว) |

## การทดสอบ

รันเทสของแต่ละ service แยกกัน (unit/integration test ในตัวเอง ไม่ต้องมี Postgres จริง):

```bash
cd user-service    && go test ./...
cd product-service && go test ./...
cd rental-service  && go test ./...
```

รัน end-to-end ผ่าน Kong ด้วยสคริปต์ใน `deploy/kong/` ตามหัวข้อ [เริ่มต้นใช้งาน](#เริ่มต้นใช้งาน)
ด้านบน (ต้องมี stack ทั้งหมดรันอยู่ด้วย `docker compose up -d --build`)

## เอกสารเพิ่มเติม

- [CONTRACT.md](CONTRACT.md) — ข้อตกลงกลางระหว่าง service (พอร์ต, JWT, response format, RBAC, endpoint ภายใน, docker-compose)
- [docs/user-management-service-design.md](docs/user-management-service-design.md) — เอกสารออกแบบ user-service
- [docs/superpowers/specs/2026-09-18-kong-api-gateway-design.md](docs/superpowers/specs/2026-09-18-kong-api-gateway-design.md) — เอกสารออกแบบ Kong Gateway

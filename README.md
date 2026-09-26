# Equipment Rental System

ระบบเช่าอุปกรณ์ — Backend (Go + PostgreSQL) สถาปัตยกรรม Microservices, มี **Kong API
Gateway** เป็น single entry point หน้าทั้ง 3 service

## สมาชิกและความรับผิดชอบ

| สมาชิก | รหัสนักศึกษา | Service | โฟลเดอร์ | สถานะ |
|---|---|---|---|---|
| สุรเชษฐ์ สีสา | 67114540583 | ระบบจัดการผู้ใช้งาน (JWT & RBAC) + Kong Gateway | `user-service/`, `deploy/kong/` | ✅ เสร็จแล้ว |
| เอกพล แรกเรียง | 67114540666 | ระบบจัดการสินค้า | `product-service/` | ✅ เสร็จแล้ว |
| วิษณุพงศ์ บัวเขียว | 67114540509 | ระบบจัดการการเช่า | `rental-service/` | ✅ เสร็จแล้ว |

ทั้ง 3 service เสร็จและ merge เข้า `main` แล้ว ([PR #4](https://github.com/CMEBOOST/equipment-rental-system/pull/4)
rental-service, [PR #5](https://github.com/CMEBOOST/equipment-rental-system/pull/5) product-service)

## Architecture

```
Client ──► Kong (proxy :8000) ──► user-service    :8081  (JWT_SECRET, ตรวจ token ให้ทุก service)
                               ├─► product-service :8082  (decode-only, ไม่ถือ secret)
                               └─► rental-service  :8083  (decode-only, ไม่ถือ secret)

rental-service ──► product-service   : เรียกตรง ไม่ผ่าน Kong (service-to-service)
rental-service ──► user-service (/auth/verify) : เรียกตรง ไม่ผ่าน Kong
```

- **Client เรียกผ่าน Kong (`:8000`) เท่านั้น** สำหรับ flow ปกติ — Kong ตรวจ JWT
  signature/expiry ให้ทุก protected route ก่อนถึง service ปลายทาง พอร์ต 8081-8083 ยัง
  เปิดไว้เพื่อ debug ตรงเฉยๆ ไม่ใช่ทางที่ client ควรใช้
- **user-service เท่านั้นที่ถือ `JWT_SECRET`** และเป็นคนออก token — product-service และ
  rental-service แค่ decode payload อ่าน claims เอา ไม่ต้อง verify signature ซ้ำ
  (ดู [CONTRACT.md §5.3](CONTRACT.md))
- Kong รันแบบ **DB-less** (ไม่มี Kong Postgres/Admin API) — config อยู่ที่
  [deploy/kong/kong.yml](deploy/kong/kong.yml) ไฟล์เดียว

## โครงสร้าง repo (monorepo)

```
equipment-rental-system/
├── user-service/       # :8081  → user-db      ✅ เสร็จแล้ว 20 endpoints
├── product-service/    # :8082  → product-db   ✅ เสร็จแล้ว
├── rental-service/      # :8083  → rental-db    ✅ เสร็จแล้ว
├── deploy/
│   └── kong/            # kong.yml, smoke-test.sh, rental-smoke-test.sh, product-rental-e2e-smoke-test.sh, README
├── docs/
│   ├── user-management-service-design.md
│   └── superpowers/     # plans/specs ที่ใช้ implement (เก็บไว้เป็น record)
├── CONTRACT.md          # ⚠️ ข้อตกลงกลางของทีม — อ่านก่อนเริ่มโค้ด
├── .env.example
└── docker-compose.yml   # user-service + product-service + rental-service + kong ครบทั้ง 3 service
```

## เริ่มต้น

```bash
cp .env.example .env
docker compose up -d --build
docker compose ps                          # user-service, user-db, product-service, product-db, rental-service, rental-db, kong ต้อง healthy/running
curl http://localhost:8000/health          # ผ่าน Kong -> 200 (user-service)
curl http://localhost:8000/rental/health   # ผ่าน Kong -> 200 (rental-service)
curl http://localhost:8000/api/v1/products # ผ่าน Kong -> 401 (ไม่มี token; product-service ยังไม่มี public /health route แยก ดู deploy/kong/README.md)
./deploy/kong/smoke-test.sh                     # user-service ผ่าน Kong -> 6 passed, 0 failed
./deploy/kong/rental-smoke-test.sh              # rental-service ผ่าน Kong -> 8 passed, 0 failed
./deploy/kong/product-rental-e2e-smoke-test.sh  # rent -> approve -> return จริงข้ามทั้ง 3 service ผ่าน Kong
```

รายละเอียดแต่ละ service:
- [user-service/README.md](user-service/README.md) — endpoint ทั้ง 20 ทาง, env vars, วิธีรัน/เทส
- [product-service/README.md](product-service/README.md) — endpoint ของระบบจัดการสินค้า, atomic status update, วิธีรัน/เทส
- [rental-service/README.md](rental-service/README.md) — endpoint ของระบบเช่า, การทำงานร่วมกับ product/user-service
- [deploy/kong/README.md](deploy/kong/README.md) — path ที่ Kong รู้จัก, วิธีทดสอบผ่าน gateway

## เอกสาร

- [CONTRACT.md](CONTRACT.md) — ข้อตกลงกลาง (พอร์ต, JWT, response format, RBAC, endpoint ภายใน, docker-compose)
- [docs/user-management-service-design.md](docs/user-management-service-design.md) — เอกสารออกแบบ user-service
- [docs/superpowers/specs/2026-09-18-kong-api-gateway-design.md](docs/superpowers/specs/2026-09-18-kong-api-gateway-design.md) — เอกสารออกแบบ Kong Gateway

## แนวทางถ้าจะเพิ่ม microservice ใหม่เข้าระบบ

ทั้ง 3 service ที่มีตอนนี้ทำตาม pattern เดียวกัน (ดู [PR #4](https://github.com/CMEBOOST/equipment-rental-system/pull/4)
สำหรับ rental-service และ [PR #5](https://github.com/CMEBOOST/equipment-rental-system/pull/5)
สำหรับ product-service เป็นตัวอย่างจริง) ถ้ามี service ใหม่ในอนาคต ให้ทำตามลำดับนี้:

### 1. เขียน service ตาม CONTRACT.md ก่อน

- อ่าน endpoint ที่ต้อง expose ตาม CONTRACT.md §8 (path, method, สิทธิ์ต่อ role ตามตารางนั้น)
- **วิธีตรวจ JWT (สำคัญ):** อ่าน [CONTRACT.md §5.3](CONTRACT.md) — service ใหม่ **ไม่ต้อง
  ถือ `JWT_SECRET` และไม่ต้อง verify signature เอง** เพราะ Kong ตรวจให้แล้วก่อน request
  จะมาถึง แค่:
  1. อ่าน header `Authorization: Bearer <token>` (Kong forward มาให้ ไม่ตัดออก)
  2. Decode ส่วน payload (base64) อ่าน claims `sub`/`role`/`email` — **ห้าม verify
     signature ซ้ำ** (ไม่มี secret จะ verify ก็ทำไม่ได้อยู่แล้ว)
  3. ใช้ `role`/`sub` บังคับสิทธิ์ตาม RBAC table ที่ [CONTRACT.md §6](CONTRACT.md)
  4. ถ้าต้องมั่นใจว่าบัญชียัง active/ไม่ถูกแบน (เช่น ก่อนยืนยันการเช่า) ค่อยเรียก
     `POST http://user-service:8081/api/v1/auth/verify` (ตรงข้าม container ไม่ผ่าน Kong,
     ใส่ header `X-Internal-Key`) — ไม่ต้องเรียกทุก request
- Database: DB แยกของตัวเอง ห้ามต่อ DB ของคนอื่นตรง, ห้าม FK ข้าม DB (ดู [CONTRACT.md §3](CONTRACT.md))
- endpoint ที่เรียกแบบ service-to-service (`X-Internal-Key` เท่านั้น ไม่มี `Authorization: Bearer`
  แนบมาด้วย) ต้องรับ internal key เดี่ยวๆ ได้ ไม่บังคับ Bearer token เหมือน endpoint ทั่วไป —
  ดูตัวอย่างจริงที่ `product-service/internal/middleware/internal.go` (ใช้กับ
  `GET /products/{id}` และ `PATCH /products/{id}/status` ที่ rental-service เรียก)
- endpoint ที่แก้ state ร่วมกันข้าม request (เช่น เปลี่ยนสถานะสินค้าเป็น `rented`) ต้องทำ
  **atomic/conditional update** (compare-and-swap) กัน race condition — ดูตัวอย่างจริงที่
  `product-service/internal/repository/product_repo.go`
- Response format ต้องตรงกับ [CONTRACT.md §4](CONTRACT.md) (envelope
  `{success,data/error}`, `snake_case`, ISO 8601 UTC)
- ทำ Dockerfile ของ service ตัวเอง — ดู `user-service/Dockerfile` เป็นตัวอย่าง (multi-stage
  build, ไม่ต้อง root)

### 2. เพิ่มเข้า root `docker-compose.yml`

เพิ่ม service + db ของตัวเอง ตามแพทเทิร์นเดียวกับ `user-service`/`product-service`/`rental-service`
ที่มีอยู่แล้ว และเพิ่มตัวแปร `<PREFIX>_APP_PORT`/`<PREFIX>_DB_*` ใหม่ใน `.env.example`

### 3. เพิ่มชื่อ service เข้า Kong's `depends_on`

ใน `docker-compose.yml`'s `kong:` block เพิ่มชื่อ service ตัวเองเข้า `depends_on:`
(ตอนนี้มี `user-service`, `product-service`, `rental-service`)

### 4. ตั้ง health route ผ่าน Kong ให้ไม่ชนกัน

**สำคัญ:** ถ้าประกาศ path `/health` แบบเดียวกันซ้ำกันหลาย service ใน `deploy/kong/kong.yml`
Kong จะ route ผิดไปที่ service อื่น (เจอบั๊กนี้มาแล้วจริงตอน implement Kong ตอนแรก —
rental-service แก้ด้วย route `/rental/health` แยกต่างหาก ส่วน product-service เลือกไม่เปิด
public health route เลยเพราะยังไม่มีใครต้องใช้ — ดู [deploy/kong/kong.yml](deploy/kong/kong.yml)
เป็นตัวอย่างจริงทั้งสองแบบ) service ใหม่ที่ต้องการ health route สาธารณะให้ตั้ง path ที่ไม่ซ้ำ
กับใคร เช่น `/<ชื่อ service>/health`

### 5. ทดสอบก่อนเปิด PR

```bash
docker compose up -d --build
docker compose up -d --force-recreate kong   # ให้ Kong โหลด kong.yml ที่แก้ใหม่
```

รัน `./deploy/kong/smoke-test.sh`, `./deploy/kong/rental-smoke-test.sh` และ
`./deploy/kong/product-rental-e2e-smoke-test.sh` อีกครั้งด้วยว่ายังผ่านเหมือนเดิม (ไม่มีอะไรพัง)
เขียน smoke test ของ service ใหม่เพิ่มด้วยก็ได้ ตามแบบสคริปต์ที่มีอยู่

### 6. เปิด PR

ถ้าแก้ `CONTRACT.md` / `docker-compose.yml` / `deploy/kong/kong.yml` → **แจ้งกลุ่มก่อน**
แล้วขอให้ทุกคน approve ก่อน merge (กระทบ endpoint ที่คนอื่นเรียกใช้ — ดู [CONTRACT.md §10.3](CONTRACT.md))

## Environment Variables

ดูรายละเอียดทั้งหมดที่ `.env.example` (ค่าที่ใช้ร่วมกันทุก service อยู่บนสุด, ค่าเฉพาะ
service แยกเป็นบล็อกด้านล่าง) และ [CONTRACT.md §2](CONTRACT.md)

| ตัวแปร | ใช้ที่ไหน |
|---|---|
| `JWT_SECRET`, `JWT_ACCESS_TTL`, `JWT_REFRESH_TTL` | user-service (ออก token) + `deploy/kong/kong.yml` (ต้องตรงกัน, ดู [deploy/kong/README.md](deploy/kong/README.md)) |
| `INTERNAL_API_KEY` | `POST /auth/verify` ที่ user-service, เรียกจาก service อื่น |
| `USER_*` / `PRODUCT_*` / `RENTAL_*` | ตัวแปร DB/port เฉพาะแต่ละ service (namespace กันชนกันใน `.env` เดียว) |

## Git Workflow

1. `main` = ความจริงเดียว มีโค้ดของทุกคน — ห้าม push ตรง
2. แตก branch งานย่อยจาก `main`: `feature/<ชื่องาน>` เช่น `feature/product-catalog`
3. เสร็จแล้วเปิด Pull Request → เพื่อน review อย่างน้อย 1 คน → merge เข้า `main`
4. ก่อนเริ่มงานใหม่ทุกครั้ง: `git checkout main && git pull`
5. จะแก้ `CONTRACT.md` / `docker-compose.yml` / `deploy/kong/kong.yml` → แจ้งกลุ่มก่อน

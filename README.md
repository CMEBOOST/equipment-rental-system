# Equipment Rental System

ระบบเช่าอุปกรณ์ — Backend (Go + PostgreSQL) สถาปัตยกรรม Microservices, มี **Kong API
Gateway** เป็น single entry point หน้าทั้ง 3 service

## สมาชิกและความรับผิดชอบ

| สมาชิก | รหัสนักศึกษา | Service | โฟลเดอร์ | สถานะ |
|---|---|---|---|---|
| สุรเชษฐ์ สีสา | 67114540583 | ระบบจัดการผู้ใช้งาน (JWT & RBAC) + Kong Gateway | `user-service/`, `deploy/kong/` | ✅ เสร็จแล้ว |
| เอกพล แรกเรียง | 67114540666 | ระบบจัดการสินค้า | `product-service/` | ⏳ ยังไม่เริ่ม (มีแค่ README) |
| วิษณุพงศ์ บัวเขียว | 67114540509 | ระบบจัดการการเช่า | `rental-service/` | ✅ เสร็จแล้ว |

## Architecture

```
Client ──► Kong (proxy :8000) ──► user-service    :8081  (JWT_SECRET, ตรวจ token ให้ทุก service)
                               ├─► product-service :8082  (decode-only, ไม่ถือ secret) ⏳
                               └─► rental-service  :8083  (decode-only, ไม่ถือ secret) ✅

rental-service ──► product-service   : เรียกตรง ไม่ผ่าน Kong (service-to-service)
rental-service ──► user-service (/auth/verify) : เรียกตรง ไม่ผ่าน Kong
```

- **Client เรียกผ่าน Kong (`:8000`) เท่านั้น** สำหรับ flow ปกติ — Kong ตรวจ JWT
  signature/expiry ให้ทุก protected route ก่อนถึง service ปลายทาง พอร์ต 8081-8083 ยัง
  เปิดไว้เพื่อ debug ตรงเฉยๆ ไม่ใช่ทางที่ client ควรใช้
- **user-service เท่านั้นที่ถือ `JWT_SECRET`** และเป็นคนออก token — rental-service (และ
  product-service เมื่อมีโค้ดแล้ว) แค่ decode payload อ่าน claims เอา ไม่ต้อง verify
  signature ซ้ำ (ดู [CONTRACT.md §5.3](CONTRACT.md))
- Kong รันแบบ **DB-less** (ไม่มี Kong Postgres/Admin API) — config อยู่ที่
  [deploy/kong/kong.yml](deploy/kong/kong.yml) ไฟล์เดียว

## โครงสร้าง repo (monorepo)

```
equipment-rental-system/
├── user-service/       # :8081  → user-db      ✅ เสร็จแล้ว 20 endpoints
├── product-service/    # :8082  → product-db   ⏳ ยังไม่มีโค้ด
├── rental-service/      # :8083  → rental-db    ✅ เสร็จแล้ว
├── deploy/
│   └── kong/            # kong.yml, smoke-test.sh, rental-smoke-test.sh, README
├── docs/
│   ├── user-management-service-design.md
│   └── superpowers/     # plans/specs ที่ใช้ implement (เก็บไว้เป็น record)
├── CONTRACT.md          # ⚠️ ข้อตกลงกลางของทีม — อ่านก่อนเริ่มโค้ด
├── .env.example
└── docker-compose.yml   # ตอนนี้มี user-service + rental-service + kong (product-service ยังไม่เข้า)
```

## เริ่มต้น

```bash
cp .env.example .env
docker compose up -d --build
docker compose ps                          # user-service, user-db, kong, rental-service, rental-db ต้อง healthy/running
curl http://localhost:8000/health          # ผ่าน Kong -> 200 (user-service)
curl http://localhost:8000/rental/health   # ผ่าน Kong -> 200 (rental-service)
./deploy/kong/smoke-test.sh                # user-service ผ่าน Kong -> 6 passed, 0 failed
./deploy/kong/rental-smoke-test.sh         # rental-service ผ่าน Kong -> 8 passed, 0 failed
```

รายละเอียดแต่ละ service:
- [user-service/README.md](user-service/README.md) — endpoint ทั้ง 20 ทาง, env vars, วิธีรัน/เทส
- [rental-service/README.md](rental-service/README.md) — endpoint ของระบบเช่า, การทำงานร่วมกับ product/user-service
- [deploy/kong/README.md](deploy/kong/README.md) — path ที่ Kong รู้จัก, วิธีทดสอบผ่าน gateway

## เอกสาร

- [CONTRACT.md](CONTRACT.md) — ข้อตกลงกลาง (พอร์ต, JWT, response format, RBAC, endpoint ภายใน, docker-compose)
- [docs/user-management-service-design.md](docs/user-management-service-design.md) — เอกสารออกแบบ user-service
- [docs/superpowers/specs/2026-09-18-kong-api-gateway-design.md](docs/superpowers/specs/2026-09-18-kong-api-gateway-design.md) — เอกสารออกแบบ Kong Gateway

## เมื่อ product-service เริ่มมีโค้ดจริง

rental-service ทำตามขั้นตอนนี้สำเร็จไปแล้ว (ดู [PR #4](https://github.com/CMEBOOST/equipment-rental-system/pull/4)
เป็นตัวอย่างจริง) เหลือแค่ product-service ที่ยังต้องทำ — ทำตามลำดับนี้:

### 1. เขียน service ตาม CONTRACT.md ก่อน

- อ่าน endpoint ที่ต้อง expose: [CONTRACT.md §8.2](CONTRACT.md) — path, method, สิทธิ์ต่อ role ตามตารางนั้น
- **วิธีตรวจ JWT (สำคัญ):** อ่าน [CONTRACT.md §5.3](CONTRACT.md) — service คุณ **ไม่ต้อง
  ถือ `JWT_SECRET` และไม่ต้อง verify signature เอง** เพราะ Kong ตรวจให้แล้วก่อน request
  จะมาถึง แค่:
  1. อ่าน header `Authorization: Bearer <token>` (Kong forward มาให้ ไม่ตัดออก)
  2. Decode ส่วน payload (base64) อ่าน claims `sub`/`role`/`email` — **ห้าม verify
     signature ซ้ำ** (ไม่มี secret จะ verify ก็ทำไม่ได้อยู่แล้ว)
  3. ใช้ `role`/`sub` บังคับสิทธิ์ตาม RBAC table ที่ [CONTRACT.md §6](CONTRACT.md)
  4. ถ้าต้องมั่นใจว่าบัญชียัง active/ไม่ถูกแบน (เช่น ก่อนยืนยันการเช่า) ค่อยเรียก
     `POST http://user-service:8081/api/v1/auth/verify` (ตรงข้าม container ไม่ผ่าน Kong,
     ใส่ header `X-Internal-Key`) — ไม่ต้องเรียกทุก request
- Database: DB แยกของตัวเอง ห้ามต่อ DB ของคนอื่นตรง, ห้าม FK ข้าม DB (ดู
  [CONTRACT.md §3](CONTRACT.md)) — ตัวแปร DB มา prefix ไว้แล้วใน `.env.example`
  (`PRODUCT_DB_*`)
- **สำคัญสำหรับ product-service โดยเฉพาะ:** rental-service เรียก `GET /products/{id}` และ
  `PATCH /products/{id}/status` ด้วย `X-Internal-Key` เท่านั้น **ไม่มี** `Authorization: Bearer`
  แนบมาด้วย (rental-service ไม่ forward token ของ user ต่อ) — endpoint สองตัวนี้ต้องรับ
  internal key เดี่ยวๆ ได้ ไม่ใช่บังคับ Bearer token เหมือน endpoint ทั่วไป (ดู
  [CONTRACT.md §8.2](CONTRACT.md) ที่แก้ไว้แล้ว) และ `PATCH /products/{id}/status` ต้องทำ
  **atomic/conditional update** (compare-and-swap บน `status`) ตอนเปลี่ยนเป็น `rented` ด้วย
  กัน race condition ตอนมีคนเช่าพร้อมกัน 2 คน — `→available` ทำแบบ idempotent ได้เลย ไม่ต้อง
  compare-and-swap
- Response format ต้องตรงกับ [CONTRACT.md §4](CONTRACT.md) (envelope
  `{success,data/error}`, `snake_case`, ISO 8601 UTC)
- ทำ Dockerfile ของ service ตัวเอง — ดู `user-service/Dockerfile` เป็นตัวอย่าง (multi-stage
  build, ไม่ต้อง root)

### 2. เพิ่มเข้า root `docker-compose.yml`

เพิ่ม service + db ของตัวเอง ตามแพทเทิร์นเดียวกับ `user-service`/`rental-service` ที่มีอยู่แล้ว
(ตัวแปร `PRODUCT_APP_PORT`/`PRODUCT_DB_*` มีอยู่ใน `.env.example` แล้ว รอแค่ service block):

```yaml
  product-service:
    build: ./product-service
    container_name: product-service
    ports: ["8082:8082"]
    env_file: [./.env]
    environment:
      APP_PORT: ${PRODUCT_APP_PORT}
      DB_HOST: ${PRODUCT_DB_HOST}
      DB_PORT: ${PRODUCT_DB_PORT}
      DB_USER: ${PRODUCT_DB_USER}
      DB_PASSWORD: ${PRODUCT_DB_PASSWORD}
      DB_NAME: ${PRODUCT_DB_NAME}
    depends_on:
      product-db:
        condition: service_healthy
    networks: [rental-net]

  product-db:
    image: postgres:16-alpine
    container_name: product-db
    environment:
      POSTGRES_USER: ${PRODUCT_DB_USER}
      POSTGRES_PASSWORD: ${PRODUCT_DB_PASSWORD}
      POSTGRES_DB: ${PRODUCT_DB_NAME}
    volumes: ["product-db-data:/var/lib/postgresql/data"]
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${PRODUCT_DB_USER} -d ${PRODUCT_DB_NAME}"]
      interval: 5s
      timeout: 5s
      retries: 5
    networks: [rental-net]
```

อย่าลืมเพิ่ม `product-db-data:` เข้า `volumes:` ท้ายไฟล์ด้วย

### 3. เพิ่มชื่อ service เข้า Kong's `depends_on`

ใน `docker-compose.yml`'s `kong:` block เพิ่มชื่อ service ตัวเองเข้า `depends_on:`
(ตอนนี้มี `user-service` กับ `rental-service`):

```yaml
    depends_on:
      - user-service
      - rental-service
      - product-service   # เพิ่มตอนที่ service มีโค้ดจริงแล้ว
```

### 4. ตั้ง health route ผ่าน Kong ให้ไม่ชนกัน

**สำคัญ:** `deploy/kong/kong.yml` **ไม่มี** route `/health` ให้ `product-service`
ตอนนี้โดยตั้งใจ — เพราะถ้าประกาศ path `/health` แบบเดียวกันซ้ำกันหลาย service Kong จะ route
ผิดไปที่ service อื่น (เจอบั๊กนี้มาแล้วจริงตอน implement Kong ตอนแรก — rental-service แก้ไปแล้ว
ด้วย route `/rental/health` แยกต่างหาก ดู [deploy/kong/kong.yml](deploy/kong/kong.yml) เป็นตัวอย่างจริง)

ให้เพิ่ม route health ของตัวเองด้วย **path ที่ไม่ซ้ำกับใคร** เช่น:

```yaml
  - name: product-service
    url: http://product-service:8082
    routes:
      - name: product-public
        paths:
          - /product/health        # ห้ามใช้ "/health" ตรงๆ ซ้ำกับ user-service
        strip_path: false
      - name: product-protected     # อันนี้มีอยู่แล้ว ไม่ต้องแก้
        paths:
          - /api/v1/products
          - /api/v1/categories
        strip_path: false
        plugins:
          - name: jwt
            config:
              claims_to_verify: ["exp"]
              run_on_preflight: false
```

`product-protected` (route หลักที่ใช้งานจริง) **มีอยู่แล้วครบ** ใน
kong.yml ไม่ต้องเพิ่ม — แค่เพิ่ม public health route เท่านั้น

### 5. ทดสอบก่อนเปิด PR

```bash
docker compose up -d --build
docker compose up -d --force-recreate kong   # ให้ Kong โหลด kong.yml ที่แก้ใหม่
curl http://localhost:8000/product/health    # ต้องได้ 200 จาก service ตัวเอง (ไม่ใช่ 503)
curl http://localhost:8000/api/v1/products    # ไม่มี token -> 401 จาก Kong เอง
```

รัน `./deploy/kong/smoke-test.sh` และ `./deploy/kong/rental-smoke-test.sh` อีกครั้งด้วย —
ต้องยังผ่าน 6/6 และ 8/8 เหมือนเดิม (ไม่มีอะไรพัง) เขียน `deploy/kong/product-smoke-test.sh`
ของตัวเองเพิ่มด้วยก็ได้ ตามแบบ `rental-smoke-test.sh`

### 6. เปิด PR

- ถ้าแก้ `CONTRACT.md` / `docker-compose.yml` / `deploy/kong/kong.yml` → **แจ้งกลุ่มก่อน**
  แล้วขอให้ทุกคน approve ก่อน merge (กระทบ endpoint ที่คนอื่นเรียกใช้ — ดู
  [CONTRACT.md §10.3](CONTRACT.md))
- อัปเดต checklist ท้าย `CONTRACT.md` ให้ตรงกับสถานะจริง (เปลี่ยนจาก "⏳ ร่าง" เป็น
  "✅ ยืนยันแล้ว" ตรงหัวข้อ 8.2 และยืนยันข้อ atomic update ที่ระบุไว้)

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

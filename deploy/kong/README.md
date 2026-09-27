# Kong API Gateway

Kong (DB-less/declarative mode) เป็น single entry point หน้าทั้ง 3 service — client
ยิงผ่าน `localhost:8000` เท่านั้นสำหรับ flow ปกติ ไม่ยิงตรงพอร์ต 8081-8083 อีกต่อไป
(8081-8083 ยังเปิดไว้เพื่อ debug ตรงเฉยๆ)

ดูเอกสารออกแบบฉบับเต็มที่ [../../docs/superpowers/specs/2026-09-18-kong-api-gateway-design.md](../../docs/superpowers/specs/2026-09-18-kong-api-gateway-design.md)
และสัญญาระหว่าง service ที่ [../../CONTRACT.md](../../CONTRACT.md) (หัวข้อ 1, 5.2, 5.3, 9.2-9.4)

## ไฟล์ในนี้

| ไฟล์ | หน้าที่ |
|---|---|
| `kong.yml` | Kong declarative config — services, routes, JWT consumer/credential |
| `smoke-test.sh` | ทดสอบ end-to-end ผ่าน Kong จริง (public/protected route, token หมดอายุ/ปลอม) — คลุม user-service |
| `rental-smoke-test.sh` | ทดสอบ end-to-end ผ่าน Kong จริง สำหรับ rental-service (role-gating, การเรียก product-service จริง) |
| `product-rental-e2e-smoke-test.sh` | flow เต็มข้ามทั้ง 3 service จริง: rent → approve → return ผ่าน Kong รวมถึงพิสูจน์ atomic status update |

## รันและทดสอบ

จาก **root ของ repo**:

```bash
cp -n .env.example .env
docker compose up -d --build
docker compose ps          # user-service, user-db, product-service, product-db, rental-service, rental-db, docs, kong ต้อง healthy/running ทั้งหมด
```

รัน smoke test อัตโนมัติ (ครอบคลุมสุดในคำสั่งเดียว):

```bash
./deploy/kong/smoke-test.sh
./deploy/kong/rental-smoke-test.sh
./deploy/kong/product-rental-e2e-smoke-test.sh
```

`smoke-test.sh` ต้องได้ `6 passed, 0 failed` — เช็ค public route ไม่ต้องมี token, protected route ปฏิเสธ
ทั้งกรณีไม่มี token/token ปลอม/token หมดอายุ (401 จาก Kong เอง ก่อนถึง service), และ
register+login+เข้าถึง protected route จริงด้วย token ที่ได้จริง (200)

`rental-smoke-test.sh` ต้องได้ `8 passed, 0 failed` — เช็ค role-gating ของ rental endpoint ผ่าน Kong
(admin/staff/customer) และการเรียก product-service จริง (ขอเช่าสินค้าที่ไม่มีอยู่จริงต้องได้ `404
NOT_FOUND` จาก product-service ไม่ใช่ `503` แบบตอนที่ product-service ยังไม่มีโค้ด)

`product-rental-e2e-smoke-test.sh` ทดสอบ flow เต็มจริงข้ามทั้ง 3 service (rent → approve → return)
รวมถึงยิง `PATCH /products/{id}/status` ตรงเพื่อพิสูจน์ compare-and-swap กัน race condition

### เช็คด้วยมือทีละจุด

```bash
curl -i http://localhost:8000/health                    # ผ่าน Kong -> 200
curl -i http://localhost:8000/api/v1/me                  # ไม่มี token -> 401 (Kong ปฏิเสธเอง)
curl -X OPTIONS -i http://localhost:8000/api/v1/me       # preflight -> 204 (CORS ไม่พัง)
```

## Path ที่ Kong รู้จัก

### user-service (มีโค้ดจริง ทำงานได้)

| Path | ต้องมี JWT? |
|---|---|
| `POST /api/v1/auth/register`, `/auth/login`, `/auth/refresh`, `GET /health` | ไม่ (public) |
| `POST /api/v1/auth/verify` | ไม่ผ่าน JWT — ใช้ `X-Internal-Key` (service เรียกกันเอง ไม่ใช่ client) |
| `POST /api/v1/auth/logout`, `/me*`, `/users*`, `/roles` | ✅ ต้องมี token ที่ Kong ตรวจแล้ว |

### rental-service (มีโค้ดจริง ทำงานได้)

| Path | ต้องมี JWT? |
|---|---|
| `GET /rental/health` | ไม่ (public) — path ต่างจาก `/health` ที่ service เสิร์ฟเอง โดยตั้งใจ (ดูด้านล่าง) |
| `POST /api/v1/rentals`, `/rentals/request`, `/rentals/{id}/approve`, `/rentals/{id}/return`, `GET /api/v1/rentals*`, `/api/v1/me/rentals` | ✅ ต้องมี token ที่ Kong ตรวจแล้ว — สิทธิ์ตาม role เช็คในตัว rental-service เอง |

rental-service เรียก product-service ต่อ (`GET /products/{id}`, `PATCH /products/{id}/status`)
ด้วย `X-Internal-Key` โดยตรงไปที่ container ไม่ผ่าน Kong — ดู CONTRACT.md §8.2 และ §7.3

### product-service (มีโค้ดจริง ทำงานได้)

| Path | ต้องมี JWT? |
|---|---|
| `GET /api/v1/products`, `/api/v1/products/{id}`, `/api/v1/categories`, `POST /api/v1/products`, `PUT /api/v1/products/{id}`, `DELETE /api/v1/products/{id}`, `PATCH /api/v1/products/{id}/status` | ✅ ต้องมี token ที่ Kong ตรวจแล้ว — สิทธิ์ตาม role เช็คในตัว product-service เอง |

**ไม่มี public `/health` ของ product-service ผ่าน Kong ตอนนี้ โดยตั้งใจ** — path `/health` แบบเดียวกัน
ถ้าประกาศซ้ำกันหลาย service ทำให้ Kong route ชนกัน (ดู kong.yml คอมเมนต์ — rental-service แก้ปัญหานี้
ด้วย path แยก `/rental/health`) ตอนนี้ยังไม่มีใครต้องใช้ health check ของ product-service ผ่าน gateway
เลยยังไม่ได้เพิ่ม route ให้ — เช็คได้โดยตรงที่ `localhost:8082/health` (ข้าม Kong) ถ้าต้องการเพิ่ม
public route ในอนาคตให้ตั้ง path ที่ไม่ซ้ำกับใคร เช่น `/product/health`

### docs (Swagger UI — ไม่ใช่ backend service จริง)

| Path | ต้องมี JWT? |
|---|---|
| `GET /docs` | ไม่ (public) — เสิร์ฟ Swagger UI จาก container `docs` (image `swaggerapi/swagger-ui`) |

`docs` เป็น container แยกที่ mount spec จาก `docs/openapi/*.yaml` เข้ามาแสดงผล ไม่ใช่ business
logic service ตั้ง `BASE_URL=/docs` ในตัว container เอง ดังนั้น route นี้ต้องใช้
**`strip_path: false`** (ต่างจาก service อื่นที่ strip path ออก) ไม่งั้น asset (JS/CSS) ของหน้า
UI จะหาไฟล์ไม่เจอ

## ข้อควรรู้ก่อนแก้ไข

- **`JWT_SECRET` มี 2 ที่ต้องตรงกัน:** ค่าใน `.env` และค่า hardcode ใน `kong.yml`
  (`consumers[0].jwt_secrets[0].secret`) — Kong แบบ declarative อ่าน env var ไม่ได้ ถ้า
  rotate secret ต้องแก้ทั้งสองที่แล้ว `docker compose up -d --force-recreate kong`
  ไม่งั้นทุก protected route จะ 401 เงียบๆโดยไม่รู้สาเหตุ
- **JWT plugin ตั้ง `claims_to_verify: ["exp"]` และ `run_on_preflight: false` ไว้แล้ว** —
  ห้ามลบ `claims_to_verify` ออก ไม่งั้น Kong จะไม่เช็คว่า token หมดอายุหรือยัง (เคยเป็นบั๊กจริง
  มาก่อน แก้แล้ว มี smoke test คลุมเคสนี้)
- Kong ไม่ทำ RBAC/rate-limiting/logging — สิทธิ์ตาม role ยังเช็คในแต่ละ service เหมือนเดิม
- Service-to-service call (`rental→product`, `rental→user /auth/verify`) ยังเรียกตรงข้าม
  container name เหมือนเดิม ไม่ผ่าน Kong

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
| `smoke-test.sh` | ทดสอบ end-to-end ผ่าน Kong จริง (public/protected route, token หมดอายุ/ปลอม) |

## รันและทดสอบ

จาก **root ของ repo**:

```bash
cp -n .env.example .env
docker compose up -d --build
docker compose ps          # user-service, user-db, kong ต้อง healthy/running ทั้ง 3
```

รัน smoke test อัตโนมัติ (ครอบคลุมสุดในคำสั่งเดียว):

```bash
./deploy/kong/smoke-test.sh
```

ต้องได้ `6 passed, 0 failed` — เช็ค public route ไม่ต้องมี token, protected route ปฏิเสธ
ทั้งกรณีไม่มี token/token ปลอม/token หมดอายุ (401 จาก Kong เอง ก่อนถึง service), และ
register+login+เข้าถึง protected route จริงด้วย token ที่ได้จริง (200)

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

### product-service / rental-service (ยังไม่มีโค้ดจริง — Kong ประกาศ path ไว้ล่วงหน้า)

| Path | Service |
|---|---|
| `/api/v1/products`, `/api/v1/categories` | product-service (ยิงแล้วได้ 503 จนกว่าจะมี service จริง) |
| `/api/v1/rentals`, `/api/v1/me/rentals` | rental-service (เช่นกัน) |

**ไม่มี `/health` ของ product/rental ตอนนี้โดยตั้งใจ** — path `/health` แบบเดียวกันถ้าประกาศ
ซ้ำกันทั้ง 3 service ทำให้ Kong route ชนกัน (ดู kong.yml คอมเมนต์) เอกพลกับวิษณุพงศ์ต้องเพิ่ม
health route ของตัวเองด้วย path ที่ไม่ซ้ำ เช่น `/product/health`, `/rental/health` ตอนสร้าง
service จริง

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

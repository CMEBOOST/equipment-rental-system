# Kong API Gateway — Design Spec

> โครงงาน: Equipment Rental System · เจ้าของงานนี้: สุรเชษฐ์ สีสา (`user-service`)
> วันที่: 2026-09-18 · สถานะ: ร่างสำหรับให้ทีม review ก่อน implement

## 1. บริบทและเป้าหมาย

ปัจจุบันระบบมี 3 microservices (`user-service`, `product-service`, `rental-service`) แต่ละตัวเปิด
port ตัวเอง (8081-8083) ให้ client เรียกตรง และแต่ละตัว verify JWT เอง (ตาม `CONTRACT.md` ข้อ 5.3)
โดยถือ `JWT_SECRET` เดียวกันทั้ง 3 service

**เป้าหมาย:** เพิ่ม Kong API Gateway เป็น single entry point หน้าทั้ง 3 service และย้าย JWT
signature/expiry validation ไปทำที่ gateway ที่เดียว เพื่อ:

- ลด service ที่ต้องถือ `JWT_SECRET` จาก 3 เหลือ 2 (user-service + Kong)
- ลด/ตัด logic verify JWT ที่ต้องเขียนซ้ำใน `product-service` และ `rental-service`
  (ทั้งสองยังไม่มีโค้ด — เป็นจังหวะที่เหมาะสมที่สุดที่จะกำหนดไว้ก่อนเริ่มเขียน)
- ได้ single entry point ที่ทำ routing รวม (ต่อยอด rate-limit/logging ได้ในอนาคต ถ้าต้องการ)

**Non-goals (ไม่ทำในรอบนี้):**

- ไม่ทำ RBAC ที่ Kong (การเช็คสิทธิ์ตาม role ต่อ endpoint ยังอยู่ในแต่ละ service เหมือนเดิม)
- ไม่ทำ rate limiting / logging plugin เพิ่มเติมที่ Kong (ปูทางไว้เฉยๆ ไม่ implement)
- ไม่ทำ Kong DB mode / Admin API — ใช้ DB-less (declarative `kong.yml`) เท่านั้น
- ไม่เปลี่ยน service-to-service call เดิม (`rental→product`, `rental→user /auth/verify`)
  ยังเรียกตรงข้าม container เหมือนเดิม ไม่ผ่าน Kong

## 2. Architecture overview

```
Client ──► Kong (proxy :8000) ──► user-service    :8081  (JWT_SECRET ยังอยู่ที่นี่)
                               ├─► product-service :8082  (decode-only, ไม่ถือ secret)
                               └─► rental-service  :8083  (decode-only, ไม่ถือ secret)

rental-service ──► product-service   : เรียกตรง ไม่ผ่าน Kong (เหมือนเดิม)
rental-service ──► user-service (/auth/verify) : เรียกตรง ไม่ผ่าน Kong (เหมือนเดิม)
```

- Deployment model: ทุก service + Kong รวมอยู่ใน `docker-compose` เดียวกัน คนละ container
  บน host เดียวกัน (ตามที่ `CONTRACT.md` ข้อ 9 ร่างไว้อยู่แล้ว) — ใช้ Docker service name
  เป็น hostname เหมือนเดิม ไม่ต้องยุ่งเรื่อง cross-machine networking
  - **หมายเหตุ:** ระหว่าง dev แต่ละคนยังรันแยกเครื่องตัวเองได้ตามปกติ (ข้อ 9.4) การรวมเป็น
    docker-compose เดียวใช้เฉพาะตอน integrate/demo
- Client เรียกผ่าน Kong (`:8000`) เท่านั้นสำหรับ flow ปกติ — port 8081-8083 ยังเปิดไว้เพื่อ
  debug/health check ตรง แต่ไม่ใช่ทางที่ client ควรใช้อีกต่อไป
- Kong รันแบบ **DB-less** (`KONG_DATABASE=off` + `KONG_DECLARATIVE_CONFIG`) — ไม่มี Kong Postgres
  เพิ่ม config ทั้งหมดอยู่ในไฟล์ `deploy/kong/kong.yml` เดียว commit เข้า git ได้

## 3. JWT claims — เพิ่ม `iss`

Kong's `jwt` plugin จับคู่ token กับ credential ผ่าน claim `iss` (ไม่ใช่ผ่าน `sub`) ต้องเพิ่ม claim
นี้เข้าไปตอน user-service ออก token:

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

`iss` เป็นค่าคงที่เดียวกันทุก token (ไม่ผูกกับ user) ใช้แค่บอก Kong ว่าจะไปหา credential ไหนมา
ตรวจ signature — งานฝั่ง `user-service`: เพิ่ม field นี้ตอนสร้าง JWT (1 บรรทัดโค้ด)

## 4. Kong declarative config (`deploy/kong/kong.yml`)

```yaml
_format_version: "3.0"

consumers:
  - username: rental-system-issuer
    jwt_secrets:
      - key: equipment-rental-system   # ต้องตรงกับ claim "iss" ใน token
        algorithm: HS256
        secret: dev-secret-please-change-min-32-characters  # เท่ากับ JWT_SECRET ใน .env.example

services:
  - name: user-service
    url: http://user-service:8081
    routes:
      - name: user-public
        paths:
          - /api/v1/auth/register
          - /api/v1/auth/login
          - /api/v1/auth/refresh
          - /health
        strip_path: false
      - name: user-internal
        paths:
          - /api/v1/auth/verify
        strip_path: false
        # ไม่ผ่าน jwt plugin — endpoint นี้ใช้ X-Internal-Key ไม่ใช่ client JWT
      - name: user-protected
        paths:
          - /api/v1/auth/logout
          - /api/v1/me
          - /api/v1/users
          - /api/v1/roles
        strip_path: false
        plugins:
          - name: jwt

  - name: product-service
    url: http://product-service:8082
    routes:
      - name: product-public
        paths: ["/health"]
        strip_path: false
      - name: product-protected
        paths:
          - /api/v1/products
          - /api/v1/categories
        strip_path: false
        plugins:
          - name: jwt

  - name: rental-service
    url: http://rental-service:8083
    routes:
      - name: rental-public
        paths: ["/health"]
        strip_path: false
      - name: rental-protected
        paths:
          - /api/v1/rentals
          - /api/v1/me/rentals
        strip_path: false
        plugins:
          - name: jwt
```

**ข้อจำกัดที่รู้อยู่แล้ว (accepted):** kong.yml แบบ DB-less ไม่รองรับอ่าน environment variable
(`${JWT_SECRET}`) ตรงๆ ต้อง hardcode ค่า secret ซ้ำในไฟล์นี้ให้ตรงกับ `.env.example` — ยอมรับได้
เพราะ `.env.example` ก็ commit ค่า dev secret ไว้ตรงๆอยู่แล้ว (ความเสี่ยงระดับเดียวกับที่ทีมยอมรับอยู่แล้ว
สำหรับ dev secret ไม่ใช่ของ production จริง)

## 5. เปลี่ยน product-service / rental-service

ทั้งสอง service **ไม่ต้อง verify JWT signature เอง และไม่ต้องถือ `JWT_SECRET`** เพราะ Kong ตรวจ
signature/expiry ให้แล้วก่อน request จะมาถึง เหลือแค่:

1. อ่าน header `Authorization: Bearer <token>` (Kong ส่งผ่านให้ไม่ตัดออก)
2. Decode ส่วน payload (base64) อ่าน claims `sub` / `role` / `email` — **ไม่ต้อง verify signature**
3. ใช้ `role` / `sub` บังคับสิทธิ์ตาม RBAC table เดิม (`CONTRACT.md` ข้อ 6) — ไม่เปลี่ยนจากเดิม

## 6. Docker Compose

เพิ่ม service `kong` เข้า `docker-compose.yml`:

```yaml
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
```

(port 8081-8083 ของแต่ละ service ยังคง map ออกมาเหมือนเดิมเพื่อ debug ตรงได้)

## 7. Diff ที่ต้องแก้ใน `CONTRACT.md`

| หัวข้อ | เดิม | แก้เป็น |
|---|---|---|
| 1. พอร์ต | client ยิงตรง 8081/8082/8083 | เพิ่มแถว Kong `kong \| 8000:8000` — client เรียกผ่าน 8000 เท่านั้นสำหรับ flow ปกติ; 8081-8083 เปิดไว้แค่ debug/internal |
| 5.2 JWT claims | ไม่มี `iss` | เพิ่ม `"iss": "equipment-rental-system"` |
| 5.3 ใครตรวจ JWT | product/rental verify เอง หรือเรียก `/auth/verify` | Kong ตรวจ signature/exp ที่ gateway ผ่าน `jwt` plugin; product/rental แค่ decode payload อ่าน claims ไม่ verify ซ้ำ ไม่ต้องถือ `JWT_SECRET`; `/auth/verify` ยังใช้เหมือนเดิมสำหรับเช็ค active/ban |
| 7 การสื่อสารระหว่าง service | — | เพิ่มบรรทัดชี้แจงว่า service-to-service call ยังตรงเหมือนเดิม ไม่ผ่าน Kong |
| 9.2 docker-compose | 3 service + 3 db | เพิ่ม service `kong` + ไฟล์ `deploy/kong/kong.yml` |
| 9.3 คำสั่งรัน | curl ตรง port service | เพิ่มตัวอย่างเรียกผ่าน Kong: `curl localhost:8000/api/v1/products` |
| 9.4 dev แยกเครื่อง | clone user-service ขอ token | เพิ่ม note: test ว่า token ปลอม/หมดอายุถูก reject จริง ต้องผ่าน Kong; solo dev ปกติไม่ต้อง |
| Checklist ท้ายไฟล์ | — | เพิ่มข้อ "ยืนยันการใช้ Kong เป็น entry point + ย้าย JWT verify ไป gateway" |
| Changelog | v1 | เพิ่ม v2 — "เพิ่ม Kong API Gateway, ย้าย JWT verify ไป gateway" โดยสุรเชษฐ์ |

การแก้ `CONTRACT.md` ต้องเปิด PR พร้อมโค้ด ให้เอกพลและวิษณุพงศ์ approve ก่อน merge
(ตาม git workflow ข้อ 10.3 — คนอื่นได้รับผลกระทบเรื่อง JWT flow)

## 8. Rollout / ผลกระทบต่อ dev workflow

- **Solo dev (ปกติ):** ไม่กระทบมาก — ทดสอบ product/rental service คนเดียวได้ง่ายขึ้นด้วยซ้ำ เพราะ
  ไม่ต้อง verify signature อีก (ใช้ JWT ปลอม/mock claims ตรงรูปแบบก็ทดสอบ business logic ได้
  โดยไม่ต้องขอ token จริงจาก user-service)
- **Test ว่า token ปลอม/หมดอายุ ถูก reject จริง:** ต้องยิงผ่าน Kong (`:8000`) เพราะเป็นจุดเดียวที่ verify
  signature — ยิงตรง port service (8081-8083) จะไม่มีใครเช็ค signature เลย
- **Integration test รวมทีม (สัปดาห์ละครั้ง ตามข้อ 9.4):** ต้องรัน Kong ผ่าน `docker-compose` ด้วย

## 9. Testing plan

- Unit: `user-service` ออก token มี claim `iss` ถูกต้อง
- Integration ผ่าน Kong: token ถูกต้อง → 200, token หมดอายุ/signature ผิด → Kong ตอบ 401 ก่อนถึง service ปลายทาง
- Integration ผ่าน Kong: เรียก path public (`/auth/login`, `/health`) โดยไม่มี token → ผ่านได้ปกติ
- Manual: `product-service`/`rental-service` decode claims จาก header ที่ Kong forward มา ใช้บังคับ RBAC ถูกต้องตามตาราง CONTRACT.md ข้อ 6.2

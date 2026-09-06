# Equipment Rental System

ระบบเช่าอุปกรณ์ — Backend (Go + PostgreSQL) สถาปัตยกรรม Microservices

## สมาชิกและความรับผิดชอบ

| สมาชิก | รหัสนักศึกษา | Service | โฟลเดอร์ |
|---|---|---|---|
| เอกพล แรกเรียง | 67114540666 | ระบบจัดการสินค้า | `product-service/` |
| วิษณุพงศ์ บัวเขียว | 67114540509 | ระบบจัดการการเช่า | `rental-service/` |
| สุรเชษฐ์ สีสา | 67114540583 | ระบบจัดการผู้ใช้งาน (JWT & RBAC) | `user-service/` |

## โครงสร้าง repo (monorepo)

```
equipment-rental-system/
├── user-service/       # :8081  → user-db
├── product-service/    # :8082  → product-db
├── rental-service/     # :8083  → rental-db
├── deploy/             # docker-compose รวม, Postman collection
├── docs/               # เอกสารออกแบบแต่ละ service
├── CONTRACT.md         # ⚠️ ข้อตกลงกลางของทีม — อ่านก่อนเริ่มโค้ด
├── .env.example
└── docker-compose.yml
```

## เริ่มต้น

```bash
cp .env.example .env
docker compose up -d --build
curl http://localhost:8081/health
```

## เอกสาร

- [CONTRACT.md](CONTRACT.md) — ข้อตกลงกลาง (พอร์ต, JWT, response format, RBAC, endpoint ภายใน)
- [docs/user-management-service-design.md](docs/user-management-service-design.md) — เอกสารออกแบบ user-service

## Git Workflow

1. `main` = ความจริงเดียว มีโค้ดของทุกคน — ห้าม push ตรง
2. แตก branch งานย่อยจาก `main`: `feature/<ชื่องาน>` เช่น `feature/user-login`
3. เสร็จแล้วเปิด Pull Request → เพื่อน review อย่างน้อย 1 คน → merge เข้า `main`
4. ก่อนเริ่มงานใหม่ทุกครั้ง: `git checkout main && git pull`
5. จะแก้ `CONTRACT.md` / `docker-compose.yml` → แจ้งกลุ่มก่อน

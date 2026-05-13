# FullTank Garage API

Go/Gin API สำหรับ FullTank Garage รองรับระบบรับประกันสินค้า, Serial Number, Film และ Promotion สำหรับ LINE LIFF apps กับ Admin Home

## Core Endpoints

- `GET /api/serial-numbers/:serial` ตรวจ Serial Number
- `POST /api/warranty/register` ลงทะเบียนรับประกันสินค้าแบบ multipart form
- `GET /api/public/films` ข้อมูลฟิล์มสำหรับ LIFF
- `GET /api/public/promotions` ข้อมูลโปรโมชันสำหรับ LIFF
- `POST /api/auth/login` เข้าสู่ระบบ Admin

## Admin Endpoints

ทุก endpoint ด้านล่างใช้ `Authorization: Bearer <token>`

- `GET/POST /api/serial-numbers`
- `GET /api/warranty/registrations`
- `GET/POST/PATCH/DELETE /api/films`
- `GET/POST/PATCH/DELETE /api/promotions`

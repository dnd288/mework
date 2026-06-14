# Hướng dẫn Triển khai Mework Server (Deployment Guide)

Tài liệu này hướng dẫn chi tiết cách cấu hình, vận hành và triển khai **Mework Server** (Go HTTP backend + PostgreSQL) lên môi trường Production.

---

## 1. Yêu cầu Hệ thống (System Requirements)

- **Hệ điều hành**: Linux (Ubuntu 22.04 LTS hoặc mới hơn được khuyến nghị), macOS hoặc Windows.
- **Go**: Phiên bản `1.25.7` trở lên (để build từ mã nguồn).
- **PostgreSQL**: Phiên bản `13` trở lên.
- **Docker & Docker Compose** (Tùy chọn - nếu triển khai qua container).

---

## 2. Các tham số Cấu hình (Configuration)

Mework Server được cấu hình hoàn toàn qua các biến môi trường (Environment Variables). 

| Biến Môi trường | Kiểu dữ liệu | Bắt buộc | Mặc định | Mô tả |
|-----------------|--------------|----------|----------|-------|
| `DATABASE_URL`  | String       | **Có**   | Không    | DSN kết nối PostgreSQL (Ví dụ: `postgres://user:password@host:port/dbname?sslmode=disable`) |
| `SERVER_KEY`    | String       | **Có**   | Không    | Khóa bí mật dùng để mã hóa và kiểm tra mã xác thực `rt_token` bằng HMAC-SHA256. |
| `LISTEN_ADDR`   | String       | Không    | `:8080`  | Địa chỉ IP và Port mà HTTP Server sẽ lắng nghe. |
| `WEBHOOK_SECRET`| String       | Không    | Trống    | Khóa dùng để xác thực chữ ký (HMAC signature verification) từ Mello Webhook. |

---

## 3. Khởi tạo và Cấu hình Cơ sở Dữ liệu

Mework Server tích hợp sẵn cơ chế tự động chạy Migration khi khởi động (Auto-migration). Tuy nhiên, bạn cần tạo trước cơ sở dữ liệu trống trong PostgreSQL.

### Sử dụng Docker để chạy PostgreSQL nhanh (cho Test/Staging):
```bash
# Tạo container Postgres với database 'mework_test'
docker run --name mework-postgres -e POSTGRES_PASSWORD=mysecretpassword -e POSTGRES_DB=mework -p 5432:5432 -d postgres:16-alpine
```

### Sử dụng Dịch vụ PostgreSQL có sẵn:
Kết nối vào PostgreSQL bằng lệnh `psql` và thực hiện tạo database:
```sql
CREATE DATABASE mework;
```

---

## 4. Biên dịch Mã nguồn (Build)

Để biên dịch Mework Server sang mã máy:

```bash
# Biên dịch cả CLI (mework) và Server (mework-server)
make build

# Biên dịch riêng Mework Server
make build-server
```

File thực thi được tạo ra tại `bin/mework-server`.

---

## 5. Triển khai Môi trường Production (Production Deployment)

### Cách A: Chạy trực tiếp dưới dạng Systemd Service (Khuyến nghị trên VPS)

1. Sao chép file thực thi vào thư mục hệ thống:
   ```bash
   sudo cp bin/mework-server /usr/local/bin/
   ```

2. Tạo file cấu hình dịch vụ Systemd `/etc/systemd/system/mework-server.service`:
   ```ini
   [Unit]
   Description=Mework Central Server
   After=network.target postgresql.service

   [Service]
   Type=simple
   User=nobody
   Group=nogroup
   Environment="DATABASE_URL=postgres://postgres:mysecretpassword@localhost:5432/mework?sslmode=disable"
   Environment="SERVER_KEY=vui-long-thay-the-bang-mot-chuoi-ngau-nhien-dai-va-an-toan"
   Environment="LISTEN_ADDR=:8080"
   Environment="WEBHOOK_SECRET=chuoi-secret-webhook-mello"
   ExecStart=/usr/local/bin/mework-server
   Restart=always
   RestartSec=5
   LimitNOFILE=65535

   [Install]
   WantedBy=multi-user.target
   ```

3. Kích hoạt và khởi chạy dịch vụ:
   ```bash
   sudo systemctl daemon-reload
   sudo systemctl enable mework-server
   sudo systemctl start mework-server
   ```

4. Kiểm tra trạng thái và Logs:
   ```bash
   sudo systemctl status mework-server
   journalctl -u mework-server.service -f
   ```

---

### Cách B: Triển khai với Docker Compose

Tạo file `docker-compose.yml` ở thư mục dự án của bạn:

```yaml
version: '3.8'

services:
  postgres:
    image: postgres:16-alpine
    container_name: mework-db
    environment:
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: strongpassword123
      POSTGRES_DB: mework
    ports:
      - "5432:5432"
    volumes:
      - mework_db_data:/var/lib/postgresql/data
    restart: always

  server:
    build:
      context: .
      dockerfile: Dockerfile # (Nếu có Dockerfile tương ứng)
    # Hoặc kéo từ Image Registry của bạn
    # image: registry.yourdomain.com/mework-server:latest
    container_name: mework-server
    environment:
      - DATABASE_URL=postgres://postgres:strongpassword123@postgres:5432/mework?sslmode=disable
      - SERVER_KEY=mot-khoa-bao-mat-tieu-chuan-hmac-sha256-o-day
      - LISTEN_ADDR=:8080
      - WEBHOOK_SECRET=secret_token_tu_mello
    ports:
      - "8080:8080"
    depends_on:
      - postgres
    restart: always

volumes:
  mework_db_data:
```

Khởi chạy hệ thống bằng lệnh:
```bash
docker compose up -d
```

---

## 6. Kiểm tra Sức khỏe Hệ thống (Health Check)

Hệ thống cung cấp một API kiểm tra sức khỏe tại `/healthz`. API này sẽ trả về HTTP `200 OK` nếu kết nối PostgreSQL hoạt động bình thường, ngược lại trả về `503 Service Unavailable`.

### Kiểm tra bằng curl:
```bash
curl -i http://localhost:8080/healthz
```

**Kết quả thành công (200 OK):**
```http
HTTP/1.1 200 OK
Content-Type: application/json
Content-Length: 15

{"status":"ok"}
```

---

## 7. Sao lưu & Phục hồi (Backup & Recovery)

Do toàn bộ dữ liệu quan trọng (Tài khoản, Runtime, Profiles, Lịch sử Jobs) đều lưu trong Postgres, bạn chỉ cần thực hiện sao lưu định kỳ database:

### Sao lưu (Backup):
```bash
pg_dump -U postgres -h localhost -d mework > mework_backup_$(date +%Y%m%d).sql
```

### Phục hồi (Restore):
```bash
psql -U postgres -h localhost -d mework < mework_backup_xxxx.sql
```

# Hướng dẫn Sử dụng Mework System (User Guide)

Tài liệu này hướng dẫn cách cấu hình, sử dụng CLI (`mework`) và chạy tác vụ AI tự động (Agent Daemon) thông qua nền tảng Kanban Mello.

---

## 1. Tổng quan Luồng hoạt động (Architecture & Flow)

Mework kết nối máy trạm chạy Daemon cục bộ của bạn với máy chủ trung tâm (Mework Server) và hệ thống quản lý công việc (ví dụ: bảng Kanban Mello):

1. **Webhook**: Khi người dùng bình luận `/run <code_runtime>` trên thẻ công việc, hệ thống gửi webhook tới Mework Server.
2. **Hàng đợi (Queue)**: Mework Server xác thực, phân tích webhook và lưu tác vụ vào hàng đợi PostgreSQL dưới dạng Job kèm theo mã lệnh, chỉ dẫn cấu hình và thông tin thẻ công việc.
3. **Báo nhận tác vụ (Long-Polling Claim)**: Daemon cục bộ trên máy trạm của bạn gửi yêu cầu long-polling (bằng `rt_token` bảo mật) lên server để kiểm tra và nhận Job được chỉ định cho runtime đó.
4. **Thực thi AI (Agent Run)**: Daemon tải profile chỉ dẫn, chạy AI CLI (`claude`, `codex`, hoặc `opencode`) cục bộ trên thư mục cách ly của máy trạm để xử lý yêu cầu.
5. **Ghi nhận & Ghi ngược (Ack & Write-back)**: Sau khi hoàn thành, Daemon gửi kết quả thực thi về Mework Server. Server tiếp nhận kết quả (ack) và đưa vào hàng đợi outbox bền vững (durable outbox queue) để tự động thực hiện ghi ngược (write-back) kết quả (như bình luận phản hồi, cập nhật trạng thái) lên hệ thống quản lý công việc (như Mello). Việc ghi ngược này hoàn toàn do Server xử lý thay vì Daemon chạy cục bộ.

---

## 2. Cấu hình CLI (`mework` CLI)

### Bước 1: Đăng nhập bằng Mello PAT (Personal Access Token)
Đăng nhập để liên kết CLI của bạn với tài khoản Mello cá nhân:
```bash
mework login --token mello_pat_xxxxxx
```
*Mẹo: Nếu không truyền trực tiếp token sau flag `--token`, CLI sẽ nhắc nhập ẩn để không lưu vào lịch sử dòng lệnh (shell history).*

### Bước 2: Cấu hình trên Server trung tâm
Các thông số cấu hình như địa chỉ máy chủ MCP (`mcp_url`), thông tin kết nối và quản lý Workspace (`workspace_id`) giờ đây được cấu hình trực tiếp trên Mework Server (hoặc thông qua các lệnh quản lý kết nối provider trên server) thay vì cấu hình cục bộ ở máy trạm. Máy trạm chỉ cần đăng nhập và đăng ký agent runtime để bắt đầu nhận việc.

---

## 3. Đăng ký Runtime và Quản lý Profiles (Từ Phase 2 trở đi)

*Lưu ý: Các lệnh quản lý đăng ký runtime và profile trên server trung tâm sẽ khả dụng đầy đủ sau khi hoàn thành Phase 2 và Phase 4.*

### Đăng ký một Agent Runtime cục bộ:
Mỗi máy trạm cần đăng ký một mã định danh runtime duy nhất trong tài khoản của bạn:
```bash
mework runtime add --code macbook-claude --label "MacBook Pro Claude 3.5 Sonnet"
```
Khi đăng ký thành công, server sẽ trả về một khóa **`rt_token` (ví dụ: `rt_abc123...`)**. Khóa này chỉ xuất hiện duy nhất 1 lần. Daemon sẽ lưu trữ token này cục bộ trong máy của bạn tại `~/.mework/` nhằm xác thực với server mà không cần lưu khóa PAT chính.

### Thiết lập Profile chỉ dẫn (System Prompt):
Tạo các tệp cấu hình chứa prompt nghiệp vụ chuyên sâu và đồng bộ lên Server:
```bash
mework profile add --name frontend-fix --file ./my-prompts/frontend.md
```

---

## 4. Vận hành Agent Daemon cục bộ

Daemon cục bộ chịu trách nhiệm giữ kết nối và xử lý Job.

### Khởi chạy Daemon:
```bash
# Chạy ngầm (Background)
mework daemon start

# Chạy trực tiếp hiển thị log ở terminal (Foreground)
mework daemon start --foreground
```

### Kiểm tra Trạng thái Daemon:
```bash
mework daemon status
```

### Xem Log thời gian thực:
```bash
mework daemon logs -f
```

### Dừng Daemon:
```bash
mework daemon stop
```

---

## 5. Kích hoạt AI tự động trên thẻ công việc (Trigger Agent)

Khi Daemon đang chạy, bạn có thể chỉ thị AI thực hiện công việc bằng cách viết bình luận trực tiếp trên thẻ công việc Mello với cú pháp:

```markdown
/run <code_runtime> [--profile <name_profile>] <yêu_cầu_chi_tiết_cho_AI>
```

### Ví dụ:
- Yêu cầu AI sửa lỗi test ngẫu nhiên bằng backend mặc định:
  ```markdown
  /run macbook-claude sửa lại các lỗi type error trong file internal/server/health.go
  ```
- Yêu cầu AI viết giao diện Frontend với bộ prompt chuyên biệt:
  ```markdown
  /run macbook-claude --profile frontend-fix tạo cho tôi component Button với thuộc tính hover animation
  ```

### Các bước tự động xử lý của hệ thống:
1. Thẻ công việc ghi nhận comment `/run`.
2. Mework Server phân tích, tạo Job và đẩy vào hàng đợi của `macbook-claude`.
3. Daemon trên máy bạn kéo Job về, đọc nội dung tóm tắt thẻ (Title/Description) và cấu hình Profile (nếu có) được nhúng sẵn trong payload của Job.
4. Khởi chạy AI Engine cục bộ trong một thư mục làm việc cách ly (`~/.mework/work/<job-id>/`).
5. Gửi kết quả thực thi về Mework Server để xác nhận hoàn thành (ack Job).
6. Mework Server tiếp nhận kết quả và thực hiện ghi ngược (write-back) các phản hồi hoặc cập nhật trạng thái lên thẻ công việc thông qua hàng đợi outbox bền vững của server.

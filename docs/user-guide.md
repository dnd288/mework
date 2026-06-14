# Hướng dẫn Sử dụng Mework System (User Guide)

Tài liệu này hướng dẫn cách cấu hình, sử dụng CLI (`mework`) và chạy tác vụ AI tự động (Agent Daemon) thông qua nền tảng Kanban Mello.

---

## 1. Tổng quan Luồng hoạt động (Architecture & Flow)

Mework kết nối máy trạm chạy Daemon cục bộ của bạn với máy chủ trung tâm (Mework Server) và bảng Kanban Mello:

1. **Webhook**: Khi người dùng bình luận `/run <code_runtime>` trên thẻ Mello, hệ thống Mello gửi webhook tới Mework Server.
2. **Hàng đợi (Queue)**: Mework Server lưu tác vụ vào hàng đợi PostgreSQL dưới dạng Job kèm theo mã lệnh và nội dung tóm tắt của thẻ công việc.
3. **Báo nhận tác vụ (Long-Polling Claim)**: Daemon cục bộ trên máy tính của bạn gửi yêu cầu long-polling (bằng `rt_token` bảo mật) lên server để kiểm tra và nhận Job được chỉ định.
4. **Thực thi AI (Agent Run)**: Daemon tải profile chỉ dẫn, chạy AI CLI (`claude`, `codex`, hoặc `opencode`) cục bộ trên thư mục cách ly của máy trạm và trực tiếp phản hồi trạng thái/kết quả lên kênh thông báo **Mezon** (hoặc Mello comment).

---

## 2. Cấu hình CLI (`mework` CLI)

### Bước 1: Đăng nhập bằng Mello PAT (Personal Access Token)
Đăng nhập để liên kết CLI của bạn với tài khoản Mello cá nhân:
```bash
mework login --token mello_pat_xxxxxx
```
*Mẹo: Nếu không truyền trực tiếp token sau flag `--token`, CLI sẽ nhắc nhập ẩn để không lưu vào lịch sử dòng lệnh (shell history).*

### Bước 2: Cấu hình Endpoint máy chủ MCP
Cấu hình địa chỉ máy chủ Mello MCP phục vụ việc ghi/phản hồi kết quả:
```bash
mework config set mcp_url https://<your-mello-mcp-endpoint>
```

### Bước 3: Cấu hình Workspace mặc định (Tùy chọn)
```bash
mework config set workspace_id <id-workspace-cua-ban>
```

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
3. Daemon trên máy bạn kéo Job về, đọc nội dung tóm tắt thẻ (Title/Description) và chỉ dẫn Profile (nếu có).
4. Khởi chạy AI Engine cục bộ trong một thư mục làm việc cách ly (`~/.mework/work/<ticket-id>/`).
5. Gửi thông tin tiến trình và kết quả cuối cùng phản hồi lại qua tin nhắn **Mezon** hoặc comment của thẻ công việc.

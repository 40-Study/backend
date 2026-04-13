# 40Study LMS - API Documentation

> Tài liệu tổng hợp tất cả API endpoints và phân tích các tính năng còn thiếu cho hệ thống LMS hoàn chỉnh.
>
> Cập nhật: 2026-04-10

---

## Mục lục

1. [Tổng quan](#tổng-quan)
2. [API Endpoints hiện có](#api-endpoints-hiện-có)
3. [Phân tích tính năng còn thiếu](#phân-tích-tính-năng-còn-thiếu)
4. [Đề xuất API mới](#đề-xuất-api-mới)

---

## Tổng quan

### Thống kê

| Metric | Số lượng |
|--------|----------|
| Tổng số endpoints | 200+ |
| Router files | 27 |
| Handler files | 43 |
| DTO files | 40+ |
| Public endpoints | ~40 |
| Auth-required endpoints | ~160 |

### Modules chính

1. Authentication & Authorization
2. Courses & Learning Content
3. Classes & Students
4. Orders, Payments & Cart
5. Livestream & Interactive Features
6. Assignments & Submissions
7. Role-Based Access Control
8. Gamification & Analytics
9. Wallet & Transactions
10. Notifications & Communication

---

## API Endpoints hiện có

### 1. Authentication & Authorization

#### 1.1 Auth Core

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| POST | `/api/auth/register/request` | RequestRegister | Gửi OTP đăng ký |
| POST | `/api/auth/register` | Register | Đăng ký (verify OTP) |
| POST | `/api/auth/login` | Login | Đăng nhập |
| POST | `/api/auth/select-role` | SelectRole | Chọn role khi login |
| POST | `/api/auth/refresh-token` | RefreshToken | Làm mới access token |
| POST | `/api/auth/reset-password/request` | RequestPasswordReset | Gửi OTP reset password |
| POST | `/api/auth/reset-password` | ResetPassword | Reset password |
| GET | `/api/auth/system-roles` | GetSystemRoleOptions | Lấy danh sách system roles |

#### 1.2 OAuth

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| GET | `/api/auth/oauth/:provider` | RedirectToProvider | Redirect tới OAuth provider |
| GET | `/api/auth/oauth/:provider/callback` | ProviderCallback | Callback từ OAuth |
| GET | `/api/auth/linked-accounts` | ListLinkedAccounts | Lấy linked accounts |
| DELETE | `/api/auth/linked-accounts/:provider` | DisconnectProvider | Hủy liên kết OAuth |

#### 1.3 User Profile

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| GET | `/api/auth/me` | GetMe | Lấy profile hiện tại |
| PUT | `/api/auth/me` | UpdateMe | Cập nhật profile |
| DELETE | `/api/auth/me` | DeleteAccount | Xóa tài khoản |
| GET | `/api/auth/my-roles` | GetMyRoles | Lấy roles của user |
| POST | `/api/auth/switch-role` | SwitchRole | Chuyển role |
| GET | `/api/auth/me/profiles` | GetMyProfiles | Lấy tất cả profiles |
| POST | `/api/auth/me/profiles` | CreateProfile | Tạo profile mới |
| DELETE | `/api/auth/me/profiles/:id` | DeleteProfile | Xóa profile |

#### 1.4 Devices & Sessions

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| GET | `/api/auth/devices` | GetAllDevices | Lấy tất cả devices đã login |
| POST | `/api/auth/logout` | LogoutOneDevice | Logout 1 device |
| POST | `/api/auth/logout-all` | LogoutAll | Logout tất cả devices |
| PUT | `/api/auth/change-password` | ChangePassword | Đổi mật khẩu |

#### 1.5 Me Extra

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| GET | `/api/me/children` | GetChildren | Lấy con (cho parent) |
| GET | `/api/me/organizations` | GetOrganizations | Lấy organizations |
| GET | `/api/me/org-roles` | GetMyOrgRoles | Lấy org roles |
| GET | `/api/me/system-roles` | GetMySystemRoles | Lấy system roles |

---

### 2. Role & Permission Management

#### 2.1 System Roles

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| GET | `/api/system-roles` | GetAllSystemRoles | Lấy tất cả system roles |
| POST | `/api/system-roles` | CreateSystemRole | Tạo system role |
| GET | `/api/system-roles/:id` | GetSystemRole | Lấy system role theo ID |
| PUT | `/api/system-roles/:id` | UpdateSystemRole | Cập nhật system role |
| DELETE | `/api/system-roles/:id` | DeleteSystemRole | Xóa system role |
| PATCH | `/api/system-roles/:id/restore` | RestoreSystemRole | Khôi phục system role |
| GET | `/api/system-roles/:id/permissions` | GetSystemRolePermissions | Lấy permissions |
| POST | `/api/system-roles/:id/permissions` | AddPermissionsToSystemRole | Thêm permissions |
| PUT | `/api/system-roles/:id/permissions` | SetSystemRolePermissions | Set permissions |
| DELETE | `/api/system-roles/:id/permissions` | RemovePermissionsFromSystemRole | Xóa permissions |
| GET | `/api/system-roles/:system_role_id/users` | GetUsersBySystemRole | Lấy users có role |

#### 2.2 Organization Roles

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| POST | `/api/org-roles` | CreateRole | Tạo org role |
| GET | `/api/org-roles` | GetAllRoles | Lấy tất cả org roles |
| GET | `/api/org-roles/:id` | GetRole | Lấy org role theo ID |
| PUT | `/api/org-roles/:id` | UpdateRole | Cập nhật org role |
| DELETE | `/api/org-roles/:id` | DeleteRole | Xóa org role |
| PATCH | `/api/org-roles/:id/restore` | RestoreRole | Khôi phục org role |
| GET | `/api/org-roles/:id/permissions` | GetRolePermissions | Lấy permissions |
| POST | `/api/org-roles/:id/permissions` | AddPermissionsToRole | Thêm permissions |
| PUT | `/api/org-roles/:id/permissions` | SetRolePermissions | Set permissions |
| DELETE | `/api/org-roles/:id/permissions` | RemovePermissionsFromRole | Xóa permissions |
| GET | `/api/org-roles/:role_id/users` | GetUsersWithOrgRoleSimple | Lấy users có role |

#### 2.3 User-Role Mapping

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| GET | `/api/users/:user_id/system-roles` | GetUserSystemRoles | Lấy system roles của user |
| POST | `/api/users/:user_id/system-roles` | AssignSystemRolesToUser | Gán system roles |
| DELETE | `/api/users/:user_id/system-roles/:system_role_id` | RevokeSystemRoleFromUser | Thu hồi system role |
| GET | `/api/users/:user_id/org-roles` | GetUserOrgRoles | Lấy org roles của user |
| POST | `/api/users/:user_id/org-roles` | AssignOrgRolesToUser | Gán org roles |
| DELETE | `/api/users/:user_id/org-roles/:org_role_id` | RevokeOrgRoleFromUser | Thu hồi org role |

#### 2.4 Permissions

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| GET | `/api/permissions` | GetAllPermissions | Lấy tất cả permissions |
| GET | `/api/permissions/:id` | GetPermissionByID | Lấy permission theo ID |
| PUT | `/api/permissions/:id` | UpdatePermission | Cập nhật permission |

---

### 3. Organizations

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| POST | `/api/organizations` | CreateOrganization | Tạo organization |
| GET | `/api/organizations` | GetAllOrganizations | Lấy tất cả organizations |
| GET | `/api/organizations/:id` | GetOrganization | Lấy organization theo ID |
| PUT | `/api/organizations/:id` | UpdateOrganization | Cập nhật organization |
| DELETE | `/api/organizations/:id` | DeleteOrganization | Xóa organization |
| GET | `/api/organizations/:organization_id/members` | GetOrganizationMembers | Lấy members |
| GET | `/api/organizations/:organization_id/roles/:role_id/users` | GetUsersWithOrgRole | Lấy users với role |

---

### 4. Courses

#### 4.1 Course CRUD

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| GET | `/api/courses` | GetAllCourses | Lấy tất cả courses (public) |
| GET | `/api/courses/slug/:slug` | GetCourseBySlug | Lấy course theo slug |
| GET | `/api/courses/:id` | GetCourseByID | Lấy course theo ID |
| POST | `/api/courses` | CreateCourse | Tạo course |
| PUT | `/api/courses/:id` | UpdateCourse | Cập nhật course |
| DELETE | `/api/courses/:id` | DeleteCourse | Xóa course |

**Request DTO (CreateCourseDTO):**
```json
{
  "instructor_id": "uuid",
  "category_id": "uuid",
  "title": "string",
  "short_description": "string",
  "description": "string",
  "thumbnail_url": "string",
  "preview_video_url": "string",
  "level": "beginner|intermediate|advanced",
  "language": "string",
  "price": 0,
  "discount_price": 0,
  "discount_expires_at": "RFC3339",
  "requirements": ["string"],
  "objectives": ["string"],
  "target_audience": ["string"],
  "is_free": false,
  "tag_ids": ["uuid"]
}
```

#### 4.2 Sections

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| POST | `/api/courses/:course_id/sections` | CreateSection | Tạo section |
| GET | `/api/courses/:course_id/sections` | GetAllSections | Lấy sections |
| GET | `/api/courses/:course_id/sections/:id` | GetSectionByID | Lấy section theo ID |
| PUT | `/api/courses/:course_id/sections/reorder` | ReorderSections | Sắp xếp lại sections |
| PUT | `/api/courses/:course_id/sections/:id` | UpdateSection | Cập nhật section |
| DELETE | `/api/courses/:course_id/sections/:id` | DeleteSection | Xóa section |

#### 4.3 Lessons

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| POST | `/api/sections/:section_id/lessons` | CreateLesson | Tạo lesson |
| GET | `/api/sections/:section_id/lessons` | GetAllLessons | Lấy lessons |
| PUT | `/api/sections/:section_id/lessons/reorder` | ReorderLessons | Sắp xếp lại lessons |
| GET | `/api/lessons/:id` | GetLessonByID | Lấy lesson theo ID |
| PUT | `/api/lessons/:id` | UpdateLesson | Cập nhật lesson |
| DELETE | `/api/lessons/:id` | DeleteLesson | Xóa lesson |

#### 4.4 Lesson Content

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| POST | `/api/lessons/:lesson_id/contents` | CreateContent | Tạo content |
| GET | `/api/lessons/:lesson_id/contents` | GetContent | Lấy contents |
| PUT | `/api/lessons/:lesson_id/contents/reorder` | ReorderContents | Sắp xếp lại contents |
| PUT | `/api/lessons/:lesson_id/contents/:id` | UpdateContent | Cập nhật content |
| DELETE | `/api/lessons/:lesson_id/contents/:id` | DeleteContent | Xóa content |

#### 4.5 Categories & Tags

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| GET | `/api/categories` | GetAllCategories | Lấy categories (public) |
| GET | `/api/categories/:id` | GetCategoryByID | Lấy category theo ID |
| POST | `/api/categories` | CreateCategory | Tạo category |
| PUT | `/api/categories/:id` | UpdateCategory | Cập nhật category |
| DELETE | `/api/categories/:id` | DeleteCategory | Xóa category |
| GET | `/api/tags` | GetAllTags | Lấy tags (public) |
| GET | `/api/tags/:id` | GetTagByID | Lấy tag theo ID |
| POST | `/api/tags` | CreateTag | Tạo tag |
| PUT | `/api/tags/:id` | UpdateTag | Cập nhật tag |
| DELETE | `/api/tags/:id` | DeleteTag | Xóa tag |

---

### 5. Classes

#### 5.1 Class CRUD

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| POST | `/api/classes` | CreateClass | Tạo class |
| GET | `/api/classes` | GetAllClasses | Lấy classes (public) |
| GET | `/api/classes/me` | GetMyClasses | Lấy classes của tôi |
| GET | `/api/classes/:id` | GetClassByID | Lấy class theo ID |
| PUT | `/api/classes/:id` | UpdateClass | Cập nhật class |
| DELETE | `/api/classes/:id` | DeleteClass | Xóa class |
| GET | `/api/courses/:course_id/classes` | GetClassesByCourseID | Lấy classes trong course |
| POST | `/api/courses/:course_id/classes` | CreateClassForCourse | Tạo class trong course |
| GET | `/api/courses/:course_id/classes/:id/contents` | GetContentScheduleForClass | Lấy schedule content |

**Model fields:**
- `start_date` - Ngày bắt đầu lớp
- `end_date` - Ngày kết thúc lớp

#### 5.2 Teacher-Class

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| POST | `/api/classes/:id/teachers` | AssignTeacherToClass | Gán teacher |
| DELETE | `/api/classes/:id/teachers/:teacherId` | RemoveTeacherFromClass | Xóa teacher |
| GET | `/api/classes/:id/teachers` | GetTeachersByClass | Lấy teachers |

#### 5.3 Student-Class

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| POST | `/api/classes/:id/students` | EnrollStudentToClass | Ghi danh student |
| DELETE | `/api/classes/:id/students/:studentId` | RemoveStudentFromClass | Xóa student |
| GET | `/api/classes/:id/students` | GetStudentsByClass | Lấy students |

#### 5.4 Class Lesson Content (Schedule)

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| POST | `/api/lesson-contents/:id/classes/bulk` | BulkAssignClassesToContent | Gán nhiều classes |
| POST | `/api/lesson-contents/:id/classes` | AssignClassToContent | Gán class với schedule |
| GET | `/api/lesson-contents/:id/classes` | GetClassesForContent | Lấy classes của content |
| PUT | `/api/lesson-contents/:id/classes/:class_id` | UpdateClassContentSchedule | Cập nhật schedule |
| DELETE | `/api/lesson-contents/:id/classes/:class_id` | RemoveClassFromContent | Xóa class khỏi content |

**Schedule fields:**
- `open_date` - Ngày mở content
- `due_date` - Deadline
- `scheduled_at` - Giờ livestream bắt đầu
- `end_at` - Giờ livestream kết thúc

---

### 6. Attendance

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| POST | `/api/classes/:classId/attendances` | MarkAttendance | Điểm danh (bulk) |
| GET | `/api/classes/:classId/attendances` | GetAllAttendances | Lấy attendances |
| GET | `/api/classes/:classId/attendances/:id` | GetAttendanceByID | Lấy attendance theo ID |
| PUT | `/api/classes/:classId/attendances/:id` | UpdateAttendance | Cập nhật attendance |
| DELETE | `/api/classes/:classId/attendances/:id` | DeleteAttendance | Xóa attendance |

**Model fields:**
- `date` - Ngày điểm danh (DATE type)
- `status` - present/absent/late/excused

---

### 7. Enrollment & Progress

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| POST | `/api/courses/:courseId/enroll` | Enroll | Ghi danh course |
| DELETE | `/api/courses/:courseId/enroll` | Unenroll | Hủy ghi danh |
| GET | `/api/enrollments` | GetMyEnrollments | Lấy enrollments của tôi |
| GET | `/api/enrollments/:id` | GetEnrollmentDetail | Chi tiết enrollment |
| PUT | `/api/lessons/:lessonId/progress` | UpdateLessonProgress | Cập nhật tiến độ |

---

### 8. Cart & Orders

#### 8.1 Cart

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| GET | `/api/cart` | GetCart | Lấy giỏ hàng |
| POST | `/api/cart` | AddToCart | Thêm vào giỏ |
| DELETE | `/api/cart` | RemoveFromCart | Xóa khỏi giỏ |
| DELETE | `/api/cart/clear` | ClearCart | Xóa toàn bộ giỏ |
| GET | `/api/cart/check/:courseID` | CheckCourseInCart | Kiểm tra course trong giỏ |

#### 8.2 Orders

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| POST | `/api/orders` | CreateOrder | Tạo order |
| GET | `/api/orders/me` | GetUserOrders | Lấy orders của tôi |
| GET | `/api/orders/:id` | GetOrder | Chi tiết order |
| POST | `/api/orders/:id/cancel` | CancelOrder | Hủy order |
| POST | `/api/orders/:id/payment-intent` | CreatePaymentIntent | Tạo payment intent |
| GET | `/api/orders/:id/payment-status` | GetPaymentStatus | Lấy trạng thái payment |
| POST | `/api/orders/:id/check-payment` | CheckPayment | Kiểm tra payment |

**Request DTO (CreateOrderRequest):**
```json
{
  "source": "cart|buy_now",
  "course_ids": ["uuid"],
  "coupon_code": "string",
  "note": "string",
  "idempotency_key": "string"
}
```

---

### 9. Vouchers

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| GET | `/api/vouchers/public` | GetPublicVouchers | Lấy vouchers public |
| GET | `/api/vouchers/code/:code` | GetVoucherByCode | Lấy voucher theo code |
| GET | `/api/vouchers/me` | GetUserSavedVouchers | Lấy vouchers đã lưu |
| POST | `/api/vouchers/:id/save` | SaveVoucher | Lưu voucher |
| DELETE | `/api/vouchers/:id/save` | UnsaveVoucher | Bỏ lưu voucher |
| POST | `/api/vouchers` | CreateVoucher | Tạo voucher |
| GET | `/api/vouchers` | GetAllVouchers | Lấy tất cả vouchers |
| GET | `/api/vouchers/:id` | GetVoucher | Chi tiết voucher |
| PUT | `/api/vouchers/:id` | UpdateVoucher | Cập nhật voucher |
| DELETE | `/api/vouchers/:id` | DeleteVoucher | Xóa voucher |
| POST | `/api/vouchers/:id/restore` | RestoreVoucher | Khôi phục voucher |
| POST | `/api/vouchers/:id/activate` | ActivateVoucher | Kích hoạt voucher |
| POST | `/api/vouchers/:id/deactivate` | DeactivateVoucher | Vô hiệu hóa voucher |
| GET | `/api/vouchers/:id/stats` | GetVoucherStats | Thống kê voucher |

---

### 10. Teachers

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| GET | `/api/teachers` | GetAllTeachers | Lấy teachers (public) |
| GET | `/api/teachers/:id` | GetTeacher | Chi tiết teacher |
| DELETE | `/api/teachers/:id` | DeleteTeacher | Xóa teacher |
| GET | `/api/teachers/me/students` | GetMyStudents | Lấy students của tôi |
| POST | `/api/teacher-profiles` | CreateTeacherProfile | Tạo teacher profile |
| GET | `/api/teacher-profiles` | GetAllTeacherProfiles | Lấy teacher profiles |
| GET | `/api/teacher-profiles/:id` | GetTeacherProfileByID | Chi tiết profile |
| PUT | `/api/teacher-profiles/:id` | UpdateTeacherProfile | Cập nhật profile |
| DELETE | `/api/teacher-profiles/:id` | DeleteTeacherProfile | Xóa profile |

---

### 11. File Upload & Media

#### 11.1 Basic Upload

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| POST | `/api/upload` | UploadImage | Upload ảnh |
| POST | `/api/upload/any` | Upload | Upload file bất kỳ |
| DELETE | `/api/upload` | DeleteFile | Xóa file |

#### 11.2 Video Upload

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| GET | `/api/videos/health` | Health | Health check |
| POST | `/api/videos/upload/init` | InitVideoUpload | Khởi tạo upload |
| POST | `/api/videos/upload/presigned-urls` | GetPresignedURLs | Lấy presigned URLs |
| POST | `/api/videos/upload/chunk-complete` | CompleteChunkUpload | Hoàn thành chunk |
| POST | `/api/videos/upload/complete` | CompleteVideoUpload | Hoàn thành upload |
| GET | `/api/videos/upload/:upload_id/status` | GetUploadStatus | Trạng thái upload |
| GET | `/api/videos/upload/:upload_id/resume` | GetResumeInfo | Thông tin resume |
| GET | `/api/videos/upload/incomplete` | GetIncompleteUploads | Uploads chưa hoàn thành |
| DELETE | `/api/videos/upload/:upload_id` | AbortUpload | Hủy upload |
| POST | `/api/videos/upload/:upload_id/reprocess` | ReprocessVideo | Re-process video |
| GET | `/api/videos/processing/queue` | GetProcessingQueue | Queue đang xử lý |

#### 11.3 HLS Streaming (Public)

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| GET | `/api/hls/:upload_id/info` | GetVideoInfo | Thông tin video |
| GET | `/api/hls/:upload_id/master.m3u8` | GetMasterPlaylist | Master playlist |
| GET | `/api/hls/:upload_id/video.mp4` | StreamOriginalVideo | Stream original |
| GET | `/api/hls/:upload_id/:quality/index.m3u8` | GetPlaylist | Quality playlist |
| GET | `/api/hls/:upload_id/:quality/:segment` | GetSegment | Video segment |

---

### 12. Livestream

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| POST | `/api/livestream` | Create | Tạo livestream |
| GET | `/api/livestream` | GetAll | Lấy livestreams |
| GET | `/api/livestream/:id` | GetByID | Chi tiết livestream |
| PUT | `/api/livestream/:id` | Update | Cập nhật livestream |
| DELETE | `/api/livestream/:id` | Delete | Xóa livestream |
| POST | `/api/livestream/:id/start` | Start | Bắt đầu livestream |
| POST | `/api/livestream/:id/end` | End | Kết thúc livestream |
| POST | `/api/livestream/:id/join` | Join | Tham gia livestream |
| POST | `/api/livestream/:id/leave` | Leave | Rời livestream |
| GET | `/api/livestream/:id/participants` | GetParticipants | Lấy participants |
| POST | `/api/livestream/:id/mute` | MuteParticipant | Mute participant |
| POST | `/api/livestream/:id/kick` | KickParticipant | Kick participant |
| POST | `/api/livestream/:id/lock-whiteboard` | LockWhiteboard | Lock whiteboard |
| POST | `/api/livestream/:id/unlock-whiteboard` | UnlockWhiteboard | Unlock whiteboard |
| POST | `/api/livestream/:id/screenshare/start` | StartScreenShare | Bắt đầu share màn hình |
| POST | `/api/livestream/:id/screenshare/stop` | StopScreenShare | Dừng share màn hình |

**Schedule fields:**
- `scheduled_at` - Giờ dự kiến bắt đầu
- `started_at` - Giờ thực tế bắt đầu
- `ended_at` - Giờ kết thúc

---

### 13. Chat & Whiteboard

#### 13.1 Chat

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| POST | `/api/chat/send` | Send | Gửi tin nhắn |
| GET | `/api/chat/:sessionId/messages` | GetMessages | Lấy tin nhắn |
| DELETE | `/api/chat/:id` | DeleteMessage | Xóa tin nhắn |
| POST | `/api/chat/:id/pin` | PinMessage | Ghim tin nhắn |
| POST | `/api/chat/:id/unpin` | UnPinMessage | Bỏ ghim tin nhắn |

#### 13.2 Whiteboard

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| GET | `/api/whiteboard/:sessionId/snapshot` | GetSnapshot | Lấy snapshot |
| POST | `/api/whiteboard/:sessionId/snapshot` | SaveSnapshot | Lưu snapshot |
| POST | `/api/whiteboard/:sessionId/event` | BroadcastEvent | Broadcast event |

---

### 14. Assignments & Submissions

#### 14.1 Assignments

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| POST | `/api/assignments` | Create | Tạo assignment |
| GET | `/api/assignments` | GetBySession | Lấy assignments theo session |
| GET | `/api/assignments/:id` | GetByID | Chi tiết assignment |
| GET | `/api/assignments/:id/sandbox` | GetSandbox | Lấy sandbox |
| PUT | `/api/assignments/:id` | Update | Cập nhật assignment |
| DELETE | `/api/assignments/:id` | Delete | Xóa assignment |
| POST | `/api/assignments/:id/publish` | Publish | Publish assignment |
| POST | `/api/assignments/:id/unpublish` | Unpublish | Unpublish assignment |
| GET | `/api/assignments/:id/testcases` | GetTestCases | Lấy test cases |
| POST | `/api/assignments/:id/testcases` | AddTestCase | Thêm test case |
| POST | `/api/assignments/:id/testcases/import` | ImportTestCases | Import test cases |
| DELETE | `/api/assignments/:id/testcases/:tcId` | DeleteTestCase | Xóa test case |

**Schedule fields:**
- `start_time` - Giờ mở assignment
- `end_time` - Deadline
- `duration_minutes` - Thời gian làm bài

#### 14.2 Submissions

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| POST | `/api/submissions` | Submit | Nộp bài |
| POST | `/api/submissions/run` | RunCode | Chạy code với test |
| POST | `/api/submissions/run-custom` | RunCustomCode | Chạy code với custom test |
| POST | `/api/submissions/execute` | ExecuteCode | Chạy code tự do (no auth) |
| GET | `/api/submissions/:id` | GetByID | Chi tiết submission |
| GET | `/api/submissions/assignment/:assignmentId` | GetByAssignment | Submissions của assignment |
| GET | `/api/submissions/my/:assignmentId` | GetMySubmissions | Submissions của tôi |
| GET | `/api/submissions/user/:userId` | GetByUser | Submissions của user |

---

### 15. Analytics

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| GET | `/api/analytics/livestream/:sessionId` | GetLivestreamAnalytics | Analytics livestream |
| GET | `/api/analytics/assignment/:assignmentId` | GetAssignmentAnalytics | Analytics assignment |
| GET | `/api/analytics/participants/:sessionId` | GetParticipantAnalytics | Analytics participants |

---

### 16. Gamification

#### 16.1 Achievements

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| GET | `/api/achievements` | ListAchievements | Lấy achievements (public) |
| GET | `/api/achievements/me` | GetMyAchievements | Achievements của tôi |
| POST | `/api/achievements/:id/unlock` | UnlockAchievement | Unlock achievement |

#### 16.2 Leaderboard

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| GET | `/api/leaderboard` | GetLeaderboard | Top users (public) |
| GET | `/api/leaderboard/me` | GetMyRank | Rank của tôi |

#### 16.3 Public Profile

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| GET | `/api/users/:id/public-profile` | GetPublicProfile | Profile công khai |

---

### 17. Wallet

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| GET | `/api/wallet/me` | GetWallet | Lấy ví student |
| GET | `/api/wallet/transactions` | GetTransactions | Lịch sử giao dịch |
| GET | `/api/wallet/teacher/me` | GetTeacherWallet | Lấy ví teacher |
| GET | `/api/wallet/teacher/transactions` | GetTeacherTransactions | Giao dịch teacher |
| PUT | `/api/wallet/teacher/bank-info` | UpdateBankInfo | Cập nhật bank info |

---

### 18. Parent Invitations

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| GET | `/api/invitations/validate/:token` | ValidateInvitationToken | Validate token (public) |
| POST | `/api/invitations/invite` | InviteParent | Mời parent |
| GET | `/api/invitations/pending` | GetPendingInvitations | Invitations chờ |
| GET | `/api/invitations/sent` | GetSentInvitations | Invitations đã gửi |
| POST | `/api/invitations/:id/respond` | RespondToInvitation | Phản hồi invitation |
| POST | `/api/invitations/:id/revoke` | RevokeInvitation | Thu hồi invitation |

---

### 19. Discussion Forum

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| GET | `/api/discussions` | ListPosts | Lấy posts (public) |
| GET | `/api/discussions/:slug` | GetPostBySlug | Post theo slug |
| POST | `/api/discussions` | CreatePost | Tạo post |
| POST | `/api/discussions/:slug/comments` | AddComment | Thêm comment |
| POST | `/api/discussions/:id/vote` | Vote | Vote post/comment |
| DELETE | `/api/discussions/:id/vote` | RemoveVote | Xóa vote |
| DELETE | `/api/discussions/:id` | DeletePost | Xóa post |

---

### 20. Notifications

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| GET | `/api/notifications` | ListNotifications | Lấy notifications |
| GET | `/api/notifications/unread-count` | GetUnreadCount | Số chưa đọc |
| GET | `/api/notifications/settings` | GetSettings | Cài đặt notifications |
| PUT | `/api/notifications/settings` | UpdateSettings | Cập nhật cài đặt |
| PATCH | `/api/notifications/read-all` | MarkAllAsRead | Đánh dấu tất cả đã đọc |
| PATCH | `/api/notifications/:id/read` | MarkAsRead | Đánh dấu đã đọc |
| DELETE | `/api/notifications/:id` | DeleteNotification | Xóa notification |
| POST | `/api/notifications/send` | SendNotification | Gửi notification |

---

### 21. User Preferences

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| GET | `/api/preferences/privacy` | GetPrivacySettings | Lấy cài đặt privacy |
| PUT | `/api/preferences/privacy` | UpdatePrivacySettings | Cập nhật privacy |

---

### 22. Health Check

| Method | Path | Handler | Mô tả |
|--------|------|---------|-------|
| GET | `/api/health` | Health | Server health |

---

## Phân tích tính năng còn thiếu

### 1. SCHEDULING & TIMETABLE (Lịch học)

#### ❌ Thiếu: Class Timetable / Recurring Schedule

**Vấn đề:** Hiện tại chỉ có `start_date`/`end_date` cho class, không có lịch học định kỳ.

**Cần thêm:**

```
Model: ClassSchedule
- id: UUID
- class_id: UUID (FK)
- day_of_week: int (0-6, 0=Sunday)
- start_time: TIME (VD: 14:00)
- end_time: TIME (VD: 15:30)
- room: string (optional)
- is_active: bool
- effective_from: DATE
- effective_until: DATE (nullable)
```

**APIs cần tạo:**

| Method | Path | Mô tả |
|--------|------|-------|
| POST | `/api/classes/:id/schedules` | Tạo lịch học định kỳ |
| GET | `/api/classes/:id/schedules` | Lấy lịch học của class |
| PUT | `/api/classes/:id/schedules/:scheduleId` | Cập nhật lịch |
| DELETE | `/api/classes/:id/schedules/:scheduleId` | Xóa lịch |
| GET | `/api/classes/:id/timetable` | Lấy timetable tuần/tháng |
| GET | `/api/me/timetable` | Lấy timetable cá nhân |

---

#### ❌ Thiếu: Calendar View API

**Cần thêm:**

| Method | Path | Mô tả |
|--------|------|-------|
| GET | `/api/calendar` | Lấy events trong khoảng thời gian |
| GET | `/api/calendar/availability` | Kiểm tra slot trống |
| GET | `/api/calendar/conflicts` | Kiểm tra conflicts |

**Query params:**
- `start_date`: RFC3339
- `end_date`: RFC3339
- `types`: livestream,assignment,class (filter)
- `class_id`: filter theo class

---

#### ❌ Thiếu: Reminder Settings API

**Model: ReminderSetting**
```
- id: UUID
- user_id: UUID (FK)
- event_type: string (livestream|assignment|class)
- remind_before_minutes: int[] (VD: [30, 60, 1440])
- channels: string[] (email, push, sms)
- is_enabled: bool
```

**APIs cần tạo:**

| Method | Path | Mô tả |
|--------|------|-------|
| GET | `/api/reminders/settings` | Lấy cài đặt reminder |
| PUT | `/api/reminders/settings` | Cập nhật cài đặt |
| GET | `/api/reminders/upcoming` | Lấy events sắp tới |

---

### 2. ATTENDANCE (Điểm danh)

#### ❌ Thiếu: Time-based Attendance

**Vấn đề:** Chỉ có `date`, không track giờ đến/về.

**Cần cập nhật Model:**
```
Attendance (update)
+ check_in_time: TIMESTAMP (giờ đến)
+ check_out_time: TIMESTAMP (giờ về)
+ expected_time: TIME (giờ dự kiến)
+ late_minutes: int (số phút đi muộn)
+ early_leave_minutes: int (số phút về sớm)
+ location: string (online/offline/room)
+ device_info: jsonb (thiết bị điểm danh)
+ verified_by: UUID (teacher xác nhận)
```

**APIs cần thêm:**

| Method | Path | Mô tả |
|--------|------|-------|
| POST | `/api/classes/:id/attendances/check-in` | Student tự check-in |
| POST | `/api/classes/:id/attendances/check-out` | Student tự check-out |
| GET | `/api/classes/:id/attendances/today` | Attendance hôm nay |
| GET | `/api/classes/:id/attendances/stats` | Thống kê điểm danh |
| GET | `/api/me/attendances` | Lịch sử điểm danh của tôi |
| POST | `/api/livestream/:id/auto-attendance` | Điểm danh tự động từ livestream |

---

### 3. ASSIGNMENT & SUBMISSION

#### ❌ Thiếu: Late Submission Handling

**Cần cập nhật Model:**
```
Assignment (update)
+ allow_late_submission: bool
+ late_penalty_percent: int (VD: 10 = -10% per day)
+ max_late_days: int
+ grace_period_minutes: int (VD: 15 phút không tính muộn)
```

**APIs cần thêm:**

| Method | Path | Mô tả |
|--------|------|-------|
| POST | `/api/assignments/:id/extension-request` | Xin gia hạn |
| GET | `/api/assignments/:id/extension-requests` | Lấy yêu cầu gia hạn |
| POST | `/api/assignments/:id/extension-requests/:reqId/approve` | Duyệt gia hạn |
| POST | `/api/assignments/:id/extension-requests/:reqId/reject` | Từ chối gia hạn |

---

#### ❌ Thiếu: Auto-submit on Timeout

**Cần thêm logic:**
- Backend queue task để auto-submit khi hết giờ
- WebSocket event thông báo sắp hết giờ

---

### 4. QUIZ

#### ❌ Thiếu: Quiz CRUD APIs

**Hiện tại:** Có model Quiz nhưng không thấy handler/router đầy đủ.

**APIs cần tạo:**

| Method | Path | Mô tả |
|--------|------|-------|
| POST | `/api/quizzes` | Tạo quiz |
| GET | `/api/quizzes` | Lấy quizzes |
| GET | `/api/quizzes/:id` | Chi tiết quiz |
| PUT | `/api/quizzes/:id` | Cập nhật quiz |
| DELETE | `/api/quizzes/:id` | Xóa quiz |
| POST | `/api/quizzes/:id/questions` | Thêm câu hỏi |
| PUT | `/api/quizzes/:id/questions/:qId` | Cập nhật câu hỏi |
| DELETE | `/api/quizzes/:id/questions/:qId` | Xóa câu hỏi |
| POST | `/api/quizzes/:id/start` | Bắt đầu làm quiz |
| POST | `/api/quizzes/:id/submit` | Nộp quiz |
| GET | `/api/quizzes/:id/results` | Kết quả quiz |
| GET | `/api/quizzes/:id/attempts` | Lịch sử làm quiz |

---

### 5. COURSE PROGRESS

#### ❌ Thiếu: Time Spent Tracking

**Cần cập nhật/tạo Model:**
```
LessonProgress (update hoặc tạo mới)
+ time_spent_seconds: int
+ last_position_seconds: int (video position)
+ views_count: int
+ completed_at: TIMESTAMP
```

**APIs cần thêm:**

| Method | Path | Mô tả |
|--------|------|-------|
| POST | `/api/lessons/:id/track-time` | Track thời gian học |
| GET | `/api/courses/:id/my-progress` | Tiến độ khóa học |
| GET | `/api/me/learning-stats` | Thống kê học tập |

---

### 6. REPORTS & ANALYTICS

#### ❌ Thiếu: Teacher/Admin Reports

| Method | Path | Mô tả |
|--------|------|-------|
| GET | `/api/reports/class/:id/attendance` | Báo cáo điểm danh class |
| GET | `/api/reports/class/:id/grades` | Báo cáo điểm class |
| GET | `/api/reports/class/:id/engagement` | Báo cáo tương tác |
| GET | `/api/reports/student/:id` | Báo cáo học sinh |
| GET | `/api/reports/course/:id/revenue` | Báo cáo doanh thu |
| GET | `/api/reports/organization/overview` | Tổng quan organization |
| POST | `/api/reports/export` | Xuất báo cáo (CSV/PDF) |

---

### 7. STUDENT GRADES

#### ❌ Thiếu: Grade Management

**Model: Grade**
```
- id: UUID
- student_id: UUID
- class_id: UUID
- assignment_id: UUID (nullable)
- quiz_id: UUID (nullable)
- grade_type: string (assignment|quiz|midterm|final|attendance|participation)
- score: float
- max_score: float
- weight: float (trọng số)
- graded_by: UUID
- graded_at: TIMESTAMP
- feedback: text
- is_final: bool
```

**APIs cần tạo:**

| Method | Path | Mô tả |
|--------|------|-------|
| POST | `/api/classes/:id/grades` | Nhập điểm |
| GET | `/api/classes/:id/grades` | Lấy bảng điểm class |
| PUT | `/api/grades/:id` | Cập nhật điểm |
| GET | `/api/classes/:id/grades/student/:studentId` | Điểm của student |
| GET | `/api/me/grades` | Điểm của tôi |
| POST | `/api/classes/:id/grades/calculate-final` | Tính điểm tổng kết |
| POST | `/api/classes/:id/grades/export` | Xuất bảng điểm |

---

### 8. TIMEZONE SUPPORT

#### ❌ Thiếu: Timezone Handling

**Cần thêm:**
- Field `timezone` trong User model
- Field `timezone` trong Organization model
- API để set timezone
- Logic convert timezone khi response

| Method | Path | Mô tả |
|--------|------|-------|
| PUT | `/api/me/timezone` | Cập nhật timezone |
| GET | `/api/timezones` | Lấy danh sách timezones |

---

### 9. BULK OPERATIONS

#### ❌ Thiếu: Bulk Schedule Operations

| Method | Path | Mô tả |
|--------|------|-------|
| POST | `/api/classes/:id/bulk-reschedule` | Dời lịch hàng loạt |
| POST | `/api/assignments/bulk-extend` | Gia hạn hàng loạt |
| POST | `/api/classes/:id/bulk-attendance` | Điểm danh hàng loạt |

---

### 10. CLASS SESSIONS (Buổi học cụ thể)

#### ❌ Thiếu: Individual Class Sessions

**Vấn đề:** Không có concept "buổi học" riêng lẻ, chỉ có livestream session.

**Model: ClassSession**
```
- id: UUID
- class_id: UUID
- schedule_id: UUID (từ recurring schedule)
- session_number: int (buổi thứ mấy)
- date: DATE
- start_time: TIME
- end_time: TIME
- status: scheduled|in_progress|completed|cancelled
- topic: string
- notes: text
- materials: jsonb (tài liệu buổi học)
- livestream_session_id: UUID (nullable, link tới livestream)
- created_at, updated_at
```

**APIs cần tạo:**

| Method | Path | Mô tả |
|--------|------|-------|
| GET | `/api/classes/:id/sessions` | Lấy danh sách buổi học |
| POST | `/api/classes/:id/sessions` | Tạo buổi học (ngoài recurring) |
| GET | `/api/classes/:id/sessions/:sessionId` | Chi tiết buổi học |
| PUT | `/api/classes/:id/sessions/:sessionId` | Cập nhật buổi học |
| DELETE | `/api/classes/:id/sessions/:sessionId` | Hủy buổi học |
| POST | `/api/classes/:id/sessions/generate` | Generate sessions từ recurring |

---

### 11. HOMEWORK/OFFLINE ASSIGNMENTS

#### ❌ Thiếu: Homework khác với Live Assignment

**Vấn đề:** Assignment hiện tại gắn với `session_id` (livestream), thiếu homework offline.

**Cần mở rộng:**
- Cho phép `session_id` nullable
- Thêm `class_id` cho homework
- Thêm `type`: live_coding | homework | project

---

### 12. TODO trong code

**File:** `internal/router/user_system_role_router.go`
- Line 23, 27, 42: Thiếu permission middleware cho admin routes

---

## Đề xuất API mới (Tổng hợp)

### Priority 1: Core Scheduling

```
# Class Timetable
POST   /api/classes/:id/schedules
GET    /api/classes/:id/schedules
PUT    /api/classes/:id/schedules/:scheduleId
DELETE /api/classes/:id/schedules/:scheduleId
GET    /api/classes/:id/timetable
GET    /api/me/timetable

# Class Sessions
GET    /api/classes/:id/sessions
POST   /api/classes/:id/sessions
GET    /api/classes/:id/sessions/:sessionId
PUT    /api/classes/:id/sessions/:sessionId
DELETE /api/classes/:id/sessions/:sessionId
POST   /api/classes/:id/sessions/generate

# Calendar
GET    /api/calendar
GET    /api/calendar/availability
GET    /api/calendar/conflicts
```

### Priority 2: Enhanced Attendance

```
POST   /api/classes/:id/attendances/check-in
POST   /api/classes/:id/attendances/check-out
GET    /api/classes/:id/attendances/today
GET    /api/classes/:id/attendances/stats
GET    /api/me/attendances
POST   /api/livestream/:id/auto-attendance
```

### Priority 3: Grade Management

```
POST   /api/classes/:id/grades
GET    /api/classes/:id/grades
PUT    /api/grades/:id
GET    /api/classes/:id/grades/student/:studentId
GET    /api/me/grades
POST   /api/classes/:id/grades/calculate-final
POST   /api/classes/:id/grades/export
```

### Priority 4: Quiz System

```
POST   /api/quizzes
GET    /api/quizzes
GET    /api/quizzes/:id
PUT    /api/quizzes/:id
DELETE /api/quizzes/:id
POST   /api/quizzes/:id/questions
PUT    /api/quizzes/:id/questions/:qId
DELETE /api/quizzes/:id/questions/:qId
POST   /api/quizzes/:id/start
POST   /api/quizzes/:id/submit
GET    /api/quizzes/:id/results
GET    /api/quizzes/:id/attempts
```

### Priority 5: Reports

```
GET    /api/reports/class/:id/attendance
GET    /api/reports/class/:id/grades
GET    /api/reports/class/:id/engagement
GET    /api/reports/student/:id
GET    /api/reports/course/:id/revenue
GET    /api/reports/organization/overview
POST   /api/reports/export
```

### Priority 6: Reminders & Extensions

```
GET    /api/reminders/settings
PUT    /api/reminders/settings
GET    /api/reminders/upcoming
POST   /api/assignments/:id/extension-request
GET    /api/assignments/:id/extension-requests
POST   /api/assignments/:id/extension-requests/:reqId/approve
POST   /api/assignments/:id/extension-requests/:reqId/reject
```

### Priority 7: Learning Progress

```
POST   /api/lessons/:id/track-time
GET    /api/courses/:id/my-progress
GET    /api/me/learning-stats
```

---

## Checklist Implementation

- [ ] Class Schedule (recurring timetable)
- [ ] Class Sessions (individual sessions)
- [ ] Calendar API
- [ ] Enhanced Attendance (check-in/out, time tracking)
- [ ] Grade Management
- [ ] Quiz CRUD + Attempt flow
- [ ] Reports & Analytics
- [ ] Reminder Settings
- [ ] Assignment Extensions
- [ ] Learning Progress Tracking
- [ ] Timezone Support
- [ ] Bulk Operations
- [ ] Permission middleware cho admin routes (TODO in code)

---

## Notes

1. **Timezone:** Nên dùng UTC trong DB, convert ở client hoặc response layer
2. **Scheduled tasks:** Dùng Asynq cho reminders, auto-submit, etc.
3. **WebSocket events:** Cần thêm events cho real-time updates (schedule changes, reminders)
4. **Caching:** Consider Redis cache cho timetable, calendar queries

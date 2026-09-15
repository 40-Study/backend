-- ============================================================================
-- Seed DEMO cho MÔI TRƯỜNG LOCAL — 40Study, 2026-09-15
-- ============================================================================
-- Lấp các vùng dữ liệu còn rỗng sau seed gốc 2026-09-11: quiz (+câu hỏi, đáp án,
-- lượt làm bài), đánh giá khoá học, giỏ hàng, và MỘT đơn hàng đã thanh toán.
-- Trước đó cả 14 đơn trong DB đều `cancelled`, nên không test được luồng mua
-- thành công, doanh thu, hay enroll-sau-thanh-toán.
--
-- KHÔNG dùng trên staging/production. Chỉ là dữ liệu demo.
--
-- Idempotent: mọi hàng dùng UUID CỐ ĐỊNH + ON CONFLICT DO NOTHING, nên chạy lại
-- nhiều lần không nhân bản. Chạy:
--   psql -h 127.0.0.1 -p 5432 -U <user> -d <db> -f scripts/seed-local-demo-260915.sql
--
-- UUID người dùng/khoá học/bài học lấy từ seed gốc. Nếu bạn tạo lại DB từ đầu thì
-- chúng đổi và script dừng ngay ở bước 0 kèm thông báo, thay vì chèn nửa vời rồi
-- fail ở khoá ngoại. Khi đó lấy lại ID bằng:
--   SELECT id, email FROM users;  SELECT id, slug FROM courses;
-- ============================================================================

BEGIN;

-- ---------------------------------------------------------------------------
-- 0. Kiểm tra tiền đề. Không có bước này thì một DB đã tạo lại sẽ fail ở giữa
--    script với lỗi khoá ngoại khó lần ra nguyên nhân.
-- ---------------------------------------------------------------------------
DO $checks$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM users WHERE id = 'e84f8be5-d5d2-4b58-a3ca-a1921252642f') THEN
    RAISE EXCEPTION 'Khong tim thay student1@demo.com (UUID cu). DB nay khong phai ban seed goc 11/09 — doc phan dau file.';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM lessons WHERE id = 'b1a3cb2b-d178-4bee-99c9-980e72d34b65') THEN
    RAISE EXCEPTION 'Khong tim thay lesson React dau tien (UUID cu). Lay lai ID theo huong dan dau file.';
  END IF;
END
$checks$;

-- ---------------------------------------------------------------------------
-- 1. QUIZZES — 3 quiz: 2 cho khoá React (trả phí), 1 cho khoá Git (miễn phí).
--    Đặt một quiz vào khoá Git vì CẢ student1 và student2 đều đã enroll khoá đó,
--    nên kiểm được quiz dưới 2 tài khoản khác nhau mà không phải mua gì.
-- ---------------------------------------------------------------------------
INSERT INTO quizzes (id, created_at, updated_at, lesson_id, course_id, title, description,
                     time_limit_minutes, pass_percentage, max_attempts, trigger_type,
                     shuffle_questions, shuffle_answers, show_correct_answers, is_ai_generated)
VALUES
  ('a1000000-0000-4000-8000-000000000001', now(), now(),
   'b1a3cb2b-d178-4bee-99c9-980e72d34b65', 'f1451024-589c-40fb-9588-3f3d583b2759',
   'Kiểm tra: Giới thiệu React', 'Ba câu hỏi ngắn về khái niệm cơ bản của React.',
   10, 70.00, 3, 'manual', true, true, true, false),
  ('a1000000-0000-4000-8000-000000000002', now(), now(),
   '7495ca80-7d5c-49da-b8e7-3b7b5d9fc33e', 'f1451024-589c-40fb-9588-3f3d583b2759',
   'Kiểm tra: Cài đặt môi trường', 'Xác nhận bạn đã dựng được môi trường phát triển.',
   10, 70.00, 3, 'manual', true, true, true, false),
  ('a1000000-0000-4000-8000-000000000003', now(), now(),
   'd3a6c3a0-2922-420d-bb67-2bd9b112c67a', '489f0c01-a40a-4476-a5e2-d4322866f342',
   'Kiểm tra: Cài đặt và cấu hình Git', 'Ba câu hỏi về cấu hình Git lần đầu.',
   15, 70.00, 5, 'manual', true, true, true, false)
ON CONFLICT (id) DO NOTHING;

-- ---------------------------------------------------------------------------
-- 2. QUESTIONS — 3 câu/quiz, phủ 3 loại: single_choice, true_false,
--    multiple_choice. question_type bị CHECK constraint giới hạn ở 5 giá trị
--    (xem internal/model/quiz.go), nên đừng thêm loại mới ở đây.
-- ---------------------------------------------------------------------------
INSERT INTO questions (id, created_at, updated_at, quiz_id, question_text, question_type,
                       explanation, points, display_order, is_ai_generated)
VALUES
  -- Quiz 1: Giới thiệu React
  ('b1000000-0000-4000-8000-000000000101', now(), now(), 'a1000000-0000-4000-8000-000000000001',
   'React là gì?', 'single_choice',
   'React là thư viện JavaScript để xây dựng giao diện người dùng, không phải framework đầy đủ.', 1.00, 1, false),
  ('b1000000-0000-4000-8000-000000000102', now(), now(), 'a1000000-0000-4000-8000-000000000001',
   'JSX là cú pháp bắt buộc phải dùng khi viết React.', 'true_false',
   'Sai. JSX chỉ là cú pháp tiện lợi; có thể gọi React.createElement trực tiếp.', 1.00, 2, false),
  ('b1000000-0000-4000-8000-000000000103', now(), now(), 'a1000000-0000-4000-8000-000000000001',
   'Đâu là hook có sẵn của React? (chọn nhiều)', 'multiple_choice',
   'useState và useEffect là hook của React. componentDidMount là lifecycle của class component, ngOnInit thuộc Angular.', 1.00, 3, false),

  -- Quiz 2: Cài đặt môi trường
  ('b1000000-0000-4000-8000-000000000201', now(), now(), 'a1000000-0000-4000-8000-000000000002',
   'Lệnh nào tạo mới một dự án Next.js?', 'single_choice',
   'npx create-next-app@latest là lệnh chính thức.', 1.00, 1, false),
  ('b1000000-0000-4000-8000-000000000202', now(), now(), 'a1000000-0000-4000-8000-000000000002',
   'Node.js là điều kiện bắt buộc để chạy Next.js ở môi trường phát triển.', 'true_false',
   'Đúng. Next.js dev server chạy trên Node.js.', 1.00, 2, false),
  ('b1000000-0000-4000-8000-000000000203', now(), now(), 'a1000000-0000-4000-8000-000000000002',
   'Những tệp nào thường có ở thư mục gốc của dự án Next.js? (chọn nhiều)', 'multiple_choice',
   'package.json và next.config.js nằm ở gốc. pom.xml thuộc Java, Gemfile thuộc Ruby.', 1.00, 3, false),

  -- Quiz 3: Git
  ('b1000000-0000-4000-8000-000000000301', now(), now(), 'a1000000-0000-4000-8000-000000000003',
   'Lệnh nào đặt tên người dùng Git ở phạm vi toàn máy?', 'single_choice',
   'git config --global user.name "Tên" — cờ --global ghi vào ~/.gitconfig.', 1.00, 1, false),
  ('b1000000-0000-4000-8000-000000000302', now(), now(), 'a1000000-0000-4000-8000-000000000003',
   'git init tạo một thư mục .git trong dự án.', 'true_false',
   'Đúng. Toàn bộ lịch sử repo nằm trong .git.', 1.00, 2, false),
  ('b1000000-0000-4000-8000-000000000303', now(), now(), 'a1000000-0000-4000-8000-000000000003',
   'Lệnh nào xem được lịch sử commit? (chọn nhiều)', 'multiple_choice',
   'git log và git reflog đều hiển thị lịch sử. git status chỉ xem trạng thái, git clone thì tải repo về.', 1.00, 3, false)
ON CONFLICT (id) DO NOTHING;

-- ---------------------------------------------------------------------------
-- 3. QUESTION_ANSWERS — đáp án từng câu; is_correct=true là đáp án đúng.
-- ---------------------------------------------------------------------------
INSERT INTO question_answers (id, created_at, question_id, answer_text, is_correct, display_order)
VALUES
  -- 1.1 single_choice
  ('c1000000-0000-4000-8000-000000011011', now(), 'b1000000-0000-4000-8000-000000000101', 'Một thư viện JavaScript để xây dựng giao diện người dùng', true,  1),
  ('c1000000-0000-4000-8000-000000011012', now(), 'b1000000-0000-4000-8000-000000000101', 'Một hệ quản trị cơ sở dữ liệu',                            false, 2),
  ('c1000000-0000-4000-8000-000000011013', now(), 'b1000000-0000-4000-8000-000000000101', 'Một ngôn ngữ lập trình biên dịch',                         false, 3),
  ('c1000000-0000-4000-8000-000000011014', now(), 'b1000000-0000-4000-8000-000000000101', 'Một máy chủ web',                                          false, 4),
  -- 1.2 true_false
  ('c1000000-0000-4000-8000-000000011021', now(), 'b1000000-0000-4000-8000-000000000102', 'Đúng', false, 1),
  ('c1000000-0000-4000-8000-000000011022', now(), 'b1000000-0000-4000-8000-000000000102', 'Sai',  true,  2),
  -- 1.3 multiple_choice
  ('c1000000-0000-4000-8000-000000011031', now(), 'b1000000-0000-4000-8000-000000000103', 'useState',          true,  1),
  ('c1000000-0000-4000-8000-000000011032', now(), 'b1000000-0000-4000-8000-000000000103', 'useEffect',         true,  2),
  ('c1000000-0000-4000-8000-000000011033', now(), 'b1000000-0000-4000-8000-000000000103', 'componentDidMount', false, 3),
  ('c1000000-0000-4000-8000-000000011034', now(), 'b1000000-0000-4000-8000-000000000103', 'ngOnInit',          false, 4),

  -- 2.1
  ('c1000000-0000-4000-8000-000000022011', now(), 'b1000000-0000-4000-8000-000000000201', 'npx create-next-app@latest', true,  1),
  ('c1000000-0000-4000-8000-000000022012', now(), 'b1000000-0000-4000-8000-000000000201', 'npm start next',             false, 2),
  ('c1000000-0000-4000-8000-000000022013', now(), 'b1000000-0000-4000-8000-000000000201', 'next new project',           false, 3),
  ('c1000000-0000-4000-8000-000000022014', now(), 'b1000000-0000-4000-8000-000000000201', 'node create next',           false, 4),
  -- 2.2
  ('c1000000-0000-4000-8000-000000022021', now(), 'b1000000-0000-4000-8000-000000000202', 'Đúng', true,  1),
  ('c1000000-0000-4000-8000-000000022022', now(), 'b1000000-0000-4000-8000-000000000202', 'Sai',  false, 2),
  -- 2.3
  ('c1000000-0000-4000-8000-000000022031', now(), 'b1000000-0000-4000-8000-000000000203', 'package.json',   true,  1),
  ('c1000000-0000-4000-8000-000000022032', now(), 'b1000000-0000-4000-8000-000000000203', 'next.config.js', true,  2),
  ('c1000000-0000-4000-8000-000000022033', now(), 'b1000000-0000-4000-8000-000000000203', 'pom.xml',        false, 3),
  ('c1000000-0000-4000-8000-000000022034', now(), 'b1000000-0000-4000-8000-000000000203', 'Gemfile',        false, 4),

  -- 3.1
  ('c1000000-0000-4000-8000-000000033011', now(), 'b1000000-0000-4000-8000-000000000301', 'git config --global user.name "Tên"', true,  1),
  ('c1000000-0000-4000-8000-000000033012', now(), 'b1000000-0000-4000-8000-000000000301', 'git set username "Tên"',              false, 2),
  ('c1000000-0000-4000-8000-000000033013', now(), 'b1000000-0000-4000-8000-000000000301', 'git user --name "Tên"',               false, 3),
  ('c1000000-0000-4000-8000-000000033014', now(), 'b1000000-0000-4000-8000-000000000301', 'git init --user "Tên"',               false, 4),
  -- 3.2
  ('c1000000-0000-4000-8000-000000033021', now(), 'b1000000-0000-4000-8000-000000000302', 'Đúng', true,  1),
  ('c1000000-0000-4000-8000-000000033022', now(), 'b1000000-0000-4000-8000-000000000302', 'Sai',  false, 2),
  -- 3.3
  ('c1000000-0000-4000-8000-000000033031', now(), 'b1000000-0000-4000-8000-000000000303', 'git log',    true,  1),
  ('c1000000-0000-4000-8000-000000033032', now(), 'b1000000-0000-4000-8000-000000000303', 'git reflog', true,  2),
  ('c1000000-0000-4000-8000-000000033033', now(), 'b1000000-0000-4000-8000-000000000303', 'git status', false, 3),
  ('c1000000-0000-4000-8000-000000033034', now(), 'b1000000-0000-4000-8000-000000000303', 'git clone',  false, 4)
ON CONFLICT (id) DO NOTHING;

-- ---------------------------------------------------------------------------
-- 4. QUIZ_ATTEMPTS — student1 làm quiz React 2 lần: trượt rồi đạt.
--    Hai lượt thay vì một, để trang lịch sử và thống kê có cả hai trạng thái
--    (pass_percentage=70 nên 2/3=66.67 là trượt, 3/3=100 là đạt).
-- ---------------------------------------------------------------------------
INSERT INTO quiz_attempts (id, created_at, user_id, quiz_id, score, total_points, percentage,
                           is_passed, time_spent_seconds, started_at, completed_at)
VALUES
  ('d1000000-0000-4000-8000-000000000001', now() - interval '2 days',
   'e84f8be5-d5d2-4b58-a3ca-a1921252642f', 'a1000000-0000-4000-8000-000000000001',
   2.00, 3.00, 66.67, false, 185,
   now() - interval '2 days', now() - interval '2 days' + interval '185 seconds'),
  ('d1000000-0000-4000-8000-000000000002', now() - interval '1 day',
   'e84f8be5-d5d2-4b58-a3ca-a1921252642f', 'a1000000-0000-4000-8000-000000000001',
   3.00, 3.00, 100.00, true, 142,
   now() - interval '1 day', now() - interval '1 day' + interval '142 seconds')
ON CONFLICT (id) DO NOTHING;

-- Lượt 1 (trượt): câu multiple_choice sai vì chỉ chọn 1 trong 2 đáp án đúng.
-- Lượt 2 (đạt): chọn đủ cả hai.
INSERT INTO quiz_attempt_answers (id, created_at, attempt_id, question_id, selected_answer_ids,
                                  text_answer, is_correct, points_earned)
VALUES
  ('e1000000-0000-4000-8000-000000000011', now() - interval '2 days', 'd1000000-0000-4000-8000-000000000001',
   'b1000000-0000-4000-8000-000000000101', ARRAY['c1000000-0000-4000-8000-000000011011']::uuid[], NULL, true,  1.00),
  ('e1000000-0000-4000-8000-000000000012', now() - interval '2 days', 'd1000000-0000-4000-8000-000000000001',
   'b1000000-0000-4000-8000-000000000102', ARRAY['c1000000-0000-4000-8000-000000011022']::uuid[], NULL, true,  1.00),
  ('e1000000-0000-4000-8000-000000000013', now() - interval '2 days', 'd1000000-0000-4000-8000-000000000001',
   'b1000000-0000-4000-8000-000000000103', ARRAY['c1000000-0000-4000-8000-000000011031']::uuid[], NULL, false, 0.00),
  ('e1000000-0000-4000-8000-000000000021', now() - interval '1 day', 'd1000000-0000-4000-8000-000000000002',
   'b1000000-0000-4000-8000-000000000101', ARRAY['c1000000-0000-4000-8000-000000011011']::uuid[], NULL, true,  1.00),
  ('e1000000-0000-4000-8000-000000000022', now() - interval '1 day', 'd1000000-0000-4000-8000-000000000002',
   'b1000000-0000-4000-8000-000000000102', ARRAY['c1000000-0000-4000-8000-000000011022']::uuid[], NULL, true,  1.00),
  ('e1000000-0000-4000-8000-000000000023', now() - interval '1 day', 'd1000000-0000-4000-8000-000000000002',
   'b1000000-0000-4000-8000-000000000103',
   ARRAY['c1000000-0000-4000-8000-000000011031','c1000000-0000-4000-8000-000000011032']::uuid[], NULL, true, 1.00)
ON CONFLICT (id) DO NOTHING;

-- ---------------------------------------------------------------------------
-- 5. REVIEWS — chỉ đánh giá khoá mà người đó ĐÃ enroll (theo bảng enrollments
--    của seed gốc); nếu không, dữ liệu tự mâu thuẫn với luật nghiệp vụ.
--    Unique index là PARTIAL (WHERE deleted_at IS NULL) nên ON CONFLICT phải
--    nhắc lại đúng điều kiện đó, không thì Postgres không khớp được index.
-- ---------------------------------------------------------------------------
INSERT INTO reviews (id, created_at, updated_at, user_id, course_id, rating, comment)
VALUES
  ('f1000000-0000-4000-8000-000000000001', now() - interval '5 days', now() - interval '5 days',
   'e84f8be5-d5d2-4b58-a3ca-a1921252642f', 'f1451024-589c-40fb-9588-3f3d583b2759', 5,
   'Giảng viên đi từ dễ đến khó rất mạch lạc. Phần Server Component giải thích rõ hơn hẳn tài liệu chính thức.'),
  ('f1000000-0000-4000-8000-000000000002', now() - interval '4 days', now() - interval '4 days',
   'e84f8be5-d5d2-4b58-a3ca-a1921252642f', '489f0c01-a40a-4476-a5e2-d4322866f342', 4,
   'Khoá miễn phí mà chất lượng tốt. Mong có thêm bài về rebase và cách xử lý xung đột.'),
  ('f1000000-0000-4000-8000-000000000003', now() - interval '3 days', now() - interval '3 days',
   'f411e408-1505-4cf1-8610-d14a3113d7ee', 'df67de09-e52a-4ec6-a4ce-915d12612d27', 5,
   'Bài tập pandas sát thực tế. Học xong tự làm được báo cáo cho công việc hiện tại.'),
  ('f1000000-0000-4000-8000-000000000004', now() - interval '2 days', now() - interval '2 days',
   'f411e408-1505-4cf1-8610-d14a3113d7ee', '489f0c01-a40a-4476-a5e2-d4322866f342', 3,
   'Nội dung ổn nhưng âm thanh vài bài bị rè. Phần pull request hơi nhanh.')
ON CONFLICT (user_id, course_id) WHERE deleted_at IS NULL DO NOTHING;

-- ---------------------------------------------------------------------------
-- 6. CART_ITEMS — chỉ khoá người đó CHƯA enroll và CHƯA mua, nếu không giỏ hàng
--    sẽ chứa thứ đã sở hữu. student1 chưa có Docker; student2 chưa có React.
-- ---------------------------------------------------------------------------
INSERT INTO cart_items (id, created_at, user_id, course_id)
VALUES
  ('a2000000-0000-4000-8000-000000000001', now() - interval '6 hours',
   'e84f8be5-d5d2-4b58-a3ca-a1921252642f', '7c965803-1aff-4a25-94f0-eea87c3f6a61'),
  ('a2000000-0000-4000-8000-000000000002', now() - interval '3 hours',
   'f411e408-1505-4cf1-8610-d14a3113d7ee', 'f1451024-589c-40fb-9588-3f3d583b2759')
ON CONFLICT (user_id, course_id) DO NOTHING;

-- ---------------------------------------------------------------------------
-- 7. MỘT ĐƠN ĐÃ THANH TOÁN + enrollment kèm theo.
--    student2 mua khoá Docker (799.000đ). Cả 14 đơn cũ đều `cancelled`, nên đây
--    là đơn hoàn tất đầu tiên trong DB: cần cho trang lịch sử đơn, doanh thu, và
--    để kiểm rằng enroll sau thanh toán hiển thị đúng.
--
--    Trạng thái là `completed`, KHÔNG phải `paid`: `paid` không nằm trong
--    model.OrderStatuses (SSOT) nên CHECK constraint chk_orders_status từ chối
--    thẳng. Thời điểm trả tiền nằm ở cột `paid_at`, không ở `status`.
--
--    order_status_histories ghi đủ 3 chặng pending -> processing -> completed,
--    khớp máy trạng thái trong order_service.go (allowed transitions: từ
--    "processing" mới sang được "completed"). Chèn mỗi hàng cuối sẽ tạo lịch sử
--    khuyết đầu và không phản ánh luồng thật.
-- ---------------------------------------------------------------------------
INSERT INTO orders (id, created_at, updated_at, user_id, order_number, subtotal, discount_amount,
                    tax_amount, total_amount, currency, status, payment_method, payment_gateway,
                    payment_transaction_id, paid_at, notes)
VALUES
  ('a3000000-0000-4000-8000-000000000001', now() - interval '8 days', now() - interval '8 days' + interval '11 minutes',
   'f411e408-1505-4cf1-8610-d14a3113d7ee', 'ORD-260907-0001',
   799000.00, 0.00, 0.00, 799000.00, 'VND', 'completed',
   'bank_transfer', 'sepay', 'DEMO-TXN-260907-0001',
   now() - interval '8 days' + interval '11 minutes',
   'Đơn demo seed local — luồng chuyển khoản thành công.')
ON CONFLICT (id) DO NOTHING;

INSERT INTO order_items (id, created_at, order_id, course_id, price, discount_amount, final_price)
VALUES
  ('a4000000-0000-4000-8000-000000000001', now() - interval '8 days',
   'a3000000-0000-4000-8000-000000000001', '7c965803-1aff-4a25-94f0-eea87c3f6a61',
   799000.00, 0.00, 799000.00)
ON CONFLICT (id) DO NOTHING;

INSERT INTO order_status_histories (id, created_at, order_id, from_status, to_status, reason, actor)
VALUES
  ('a5000000-0000-4000-8000-000000000001', now() - interval '8 days',
   'a3000000-0000-4000-8000-000000000001', NULL, 'pending', 'Khởi tạo đơn từ giỏ hàng', 'system'),
  ('a5000000-0000-4000-8000-000000000002', now() - interval '8 days' + interval '6 minutes',
   'a3000000-0000-4000-8000-000000000001', 'pending', 'processing', 'Nhận được biến động số dư, đang đối soát', 'system'),
  ('a5000000-0000-4000-8000-000000000003', now() - interval '8 days' + interval '11 minutes',
   'a3000000-0000-4000-8000-000000000001', 'processing', 'completed', 'Đối soát chuyển khoản khớp số tiền', 'system')
ON CONFLICT (id) DO NOTHING;

-- Enrollment sinh ra từ đơn đã thanh toán. Bảng này không có unique index trên
-- (user_id, course_id) nên dùng NOT EXISTS thay cho ON CONFLICT — nếu không,
-- chạy lại script sẽ tạo enrollment trùng.
INSERT INTO enrollments (id, created_at, updated_at, user_id, course_id, enrolled_at,
                         progress_percentage, last_accessed_at)
SELECT 'a6000000-0000-4000-8000-000000000001', now() - interval '8 days' + interval '11 minutes',
       now() - interval '8 days' + interval '11 minutes',
       'f411e408-1505-4cf1-8610-d14a3113d7ee', '7c965803-1aff-4a25-94f0-eea87c3f6a61',
       now() - interval '8 days' + interval '11 minutes', 0, NULL
WHERE NOT EXISTS (
  SELECT 1 FROM enrollments
  WHERE user_id = 'f411e408-1505-4cf1-8610-d14a3113d7ee'
    AND course_id = '7c965803-1aff-4a25-94f0-eea87c3f6a61'
    AND deleted_at IS NULL
);

COMMIT;

-- ---------------------------------------------------------------------------
-- Đối chiếu nhanh sau khi chạy.
-- ---------------------------------------------------------------------------
SELECT 'quizzes' AS bang, count(*) FROM quizzes
UNION ALL SELECT 'questions', count(*) FROM questions
UNION ALL SELECT 'question_answers', count(*) FROM question_answers
UNION ALL SELECT 'quiz_attempts', count(*) FROM quiz_attempts
UNION ALL SELECT 'quiz_attempt_answers', count(*) FROM quiz_attempt_answers
UNION ALL SELECT 'reviews', count(*) FROM reviews
UNION ALL SELECT 'cart_items', count(*) FROM cart_items
UNION ALL SELECT 'orders(completed)', count(*) FROM orders WHERE status = 'completed'
UNION ALL SELECT 'enrollments', count(*) FROM enrollments
ORDER BY 1;

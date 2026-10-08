-- ============================================================================
-- Seed DEMO cho MÔI TRƯỜNG LOCAL — 40Study, 2026-09-15
-- ============================================================================
-- Bổ sung trên nền seed Go (`go run ./cmd/seed`): 2 quiz React (+câu hỏi, đáp án,
-- lượt làm bài), đánh giá khoá học, giỏ hàng, và một đơn chuyển khoản đã hoàn
-- tất (student2 mua khoá Docker) kèm enrollment.
--
-- KHÔNG dùng trên staging/production. Chỉ là dữ liệu demo.
--
-- Idempotent: hàng do script tạo dùng UUID CỐ ĐỊNH + ON CONFLICT DO NOTHING (hoặc
-- NOT EXISTS), nên chạy lại nhiều lần không nhân bản. Chạy:
--   psql -h 127.0.0.1 -p 5432 -U <user> -d <db> -f scripts/seed-local-demo-260915.sql
--
-- ID người dùng/khoá học/bài học KHÔNG gắn cứng: bước 0 tra theo khoá tự nhiên
-- (email, slug khoá học, thứ tự bài học) rồi lưu vào biến psql bằng \gset, nên
-- script chạy được trên DB vừa tạo lại. Vì dùng \gset, script CHỈ chạy qua psql.
--
-- Quiz Git trước đây nằm ở file này nay do seed Go tạo (cùng tiêu đề, cùng bài),
-- nên đã bỏ khỏi đây để không sinh quiz trùng.
-- ============================================================================

\set ON_ERROR_STOP on

BEGIN;

-- ---------------------------------------------------------------------------
-- 0. Tra ID theo khoá tự nhiên. Thiếu thứ gì thì dừng NGAY với thông báo rõ,
--    thay vì để \gset gán chuỗi rỗng rồi fail ở ép kiểu uuid khó hiểu.
-- ---------------------------------------------------------------------------
CREATE TEMP TABLE seed_ref ON COMMIT DROP AS
WITH react_lessons AS (
  SELECT l.id, row_number() OVER (ORDER BY s.display_order, l.display_order) AS n
  FROM lessons l
  JOIN sections s ON s.id = l.section_id AND s.deleted_at IS NULL
  JOIN courses  c ON c.id = s.course_id
  WHERE c.slug = 'react-nextjs-tu-co-ban-den-nang-cao'
)
SELECT
  (SELECT id FROM users   WHERE email = 'student1@demo.com' AND deleted_at IS NULL) AS student1,
  (SELECT id FROM users   WHERE email = 'student2@demo.com' AND deleted_at IS NULL) AS student2,
  (SELECT id FROM courses WHERE slug = 'react-nextjs-tu-co-ban-den-nang-cao')       AS course_react,
  (SELECT id FROM courses WHERE slug = 'git-github-cho-nguoi-moi-bat-dau')          AS course_git,
  (SELECT id FROM courses WHERE slug = 'python-cho-khoa-hoc-du-lieu')               AS course_data,
  (SELECT id FROM courses WHERE slug = 'docker-kubernetes-thuc-chien')              AS course_docker,
  (SELECT id FROM react_lessons WHERE n = 1)                                        AS lesson_react_1,
  (SELECT id FROM react_lessons WHERE n = 2)                                        AS lesson_react_2;

DO $checks$
DECLARE r seed_ref;
BEGIN
  SELECT * INTO r FROM seed_ref;
  IF r.student1       IS NULL THEN RAISE EXCEPTION 'Thieu user student1@demo.com — chay go run ./cmd/seed truoc.'; END IF;
  IF r.student2       IS NULL THEN RAISE EXCEPTION 'Thieu user student2@demo.com — chay go run ./cmd/seed truoc.'; END IF;
  IF r.course_react   IS NULL THEN RAISE EXCEPTION 'Thieu khoa slug react-nextjs-tu-co-ban-den-nang-cao.'; END IF;
  IF r.course_git     IS NULL THEN RAISE EXCEPTION 'Thieu khoa slug git-github-cho-nguoi-moi-bat-dau.'; END IF;
  IF r.course_data    IS NULL THEN RAISE EXCEPTION 'Thieu khoa slug python-cho-khoa-hoc-du-lieu.'; END IF;
  IF r.course_docker  IS NULL THEN RAISE EXCEPTION 'Thieu khoa slug docker-kubernetes-thuc-chien.'; END IF;
  IF r.lesson_react_2 IS NULL THEN RAISE EXCEPTION 'Khoa React can it nhat 2 bai hoc.'; END IF;
END
$checks$;

SELECT * FROM seed_ref \gset

-- ---------------------------------------------------------------------------
-- 1. QUIZZES — 2 quiz cho khoá React (trả phí), gắn vào bài 1 và bài 2.
-- ---------------------------------------------------------------------------
INSERT INTO quizzes (id, created_at, updated_at, lesson_id, course_id, title, description,
                     time_limit_minutes, pass_percentage, max_attempts, trigger_type,
                     shuffle_questions, shuffle_answers, show_correct_answers, is_ai_generated)
VALUES
  ('a1000000-0000-4000-8000-000000000001', now(), now(),
   :'lesson_react_1', :'course_react',
   'Kiểm tra: Giới thiệu React', 'Ba câu hỏi ngắn về khái niệm cơ bản của React.',
   10, 70.00, 3, 'manual', true, true, true, false),
  ('a1000000-0000-4000-8000-000000000002', now(), now(),
   :'lesson_react_2', :'course_react',
   'Kiểm tra: Cài đặt môi trường', 'Xác nhận bạn đã dựng được môi trường phát triển.',
   10, 70.00, 3, 'manual', true, true, true, false)
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
   'package.json và next.config.js nằm ở gốc. pom.xml thuộc Java, Gemfile thuộc Ruby.', 1.00, 3, false)
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
  ('c1000000-0000-4000-8000-000000022034', now(), 'b1000000-0000-4000-8000-000000000203', 'Gemfile',        false, 4)
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
   :'student1', 'a1000000-0000-4000-8000-000000000001',
   2.00, 3.00, 66.67, false, 185,
   now() - interval '2 days', now() - interval '2 days' + interval '185 seconds'),
  ('d1000000-0000-4000-8000-000000000002', now() - interval '1 day',
   :'student1', 'a1000000-0000-4000-8000-000000000001',
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
-- 5. REVIEWS — chỉ chèn khi người đó ĐÃ enroll khoá (lọc bằng EXISTS), nếu
--    không dữ liệu tự mâu thuẫn với luật nghiệp vụ. Seed Go có thể đã tạo review
--    cho cùng cặp (user, course); khi đó ON CONFLICT bỏ qua, giữ bản của seed Go.
--    Unique index là PARTIAL (WHERE deleted_at IS NULL) nên ON CONFLICT phải
--    nhắc lại đúng điều kiện đó, không thì Postgres không khớp được index.
-- ---------------------------------------------------------------------------
INSERT INTO reviews (id, created_at, updated_at, user_id, course_id, rating, comment)
SELECT v.id::uuid, v.at, v.at, v.user_id::uuid, v.course_id::uuid, v.rating, v.comment
FROM (VALUES
  ('f1000000-0000-4000-8000-000000000001', now() - interval '5 days', :'student1', :'course_react', 5,
   'Giảng viên đi từ dễ đến khó rất mạch lạc. Phần Server Component giải thích rõ hơn hẳn tài liệu chính thức.'),
  ('f1000000-0000-4000-8000-000000000002', now() - interval '4 days', :'student1', :'course_git', 4,
   'Khoá miễn phí mà chất lượng tốt. Mong có thêm bài về rebase và cách xử lý xung đột.'),
  ('f1000000-0000-4000-8000-000000000003', now() - interval '3 days', :'student2', :'course_data', 5,
   'Bài tập pandas sát thực tế. Học xong tự làm được báo cáo cho công việc hiện tại.'),
  ('f1000000-0000-4000-8000-000000000004', now() - interval '2 days', :'student2', :'course_git', 3,
   'Nội dung ổn nhưng âm thanh vài bài bị rè. Phần pull request hơi nhanh.')
) AS v(id, at, user_id, course_id, rating, comment)
WHERE EXISTS (
  SELECT 1 FROM enrollments e
  WHERE e.user_id = v.user_id::uuid AND e.course_id = v.course_id::uuid AND e.deleted_at IS NULL
)
ON CONFLICT (user_id, course_id) WHERE deleted_at IS NULL DO NOTHING;

-- ---------------------------------------------------------------------------
-- 6. CART_ITEMS — chỉ khoá người đó CHƯA enroll và CHƯA mua, nếu không giỏ hàng
--    sẽ chứa thứ đã sở hữu. student1 chưa có Docker; student2 chưa có React.
-- ---------------------------------------------------------------------------
INSERT INTO cart_items (id, created_at, user_id, course_id)
VALUES
  ('a2000000-0000-4000-8000-000000000001', now() - interval '6 hours', :'student1', :'course_docker'),
  ('a2000000-0000-4000-8000-000000000002', now() - interval '3 hours', :'student2', :'course_react')
ON CONFLICT (user_id, course_id) DO NOTHING;

-- ---------------------------------------------------------------------------
-- 7. MỘT ĐƠN CHUYỂN KHOẢN ĐÃ HOÀN TẤT + enrollment kèm theo.
--    student2 mua khoá Docker (799.000đ) qua sepay — bổ sung luồng chuyển khoản
--    cho các đơn demo của seed Go.
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
   :'student2', 'ORD-260907-0001',
   799000.00, 0.00, 0.00, 799000.00, 'VND', 'completed',
   'bank_transfer', 'sepay', 'DEMO-TXN-260907-0001',
   now() - interval '8 days' + interval '11 minutes',
   'Đơn demo seed local — luồng chuyển khoản thành công.')
ON CONFLICT (id) DO NOTHING;

INSERT INTO order_items (id, created_at, order_id, course_id, price, discount_amount, final_price)
VALUES
  ('a4000000-0000-4000-8000-000000000001', now() - interval '8 days',
   'a3000000-0000-4000-8000-000000000001', :'course_docker',
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
       :'student2', :'course_docker',
       now() - interval '8 days' + interval '11 minutes', 0, NULL
WHERE NOT EXISTS (
  SELECT 1 FROM enrollments
  WHERE user_id = :'student2' AND course_id = :'course_docker' AND deleted_at IS NULL
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

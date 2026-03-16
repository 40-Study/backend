# Order/Payment Implementation Plan

Muc tieu: bo sung luong mua khoa hoc day du theo mo hinh order -> payment -> enroll.

## 1) Scope va ket qua can dat

- [ ] Co API tao order tu cart hoac buy-now.
- [ ] Co API lay chi tiet order, list orders cua user.
- [ ] Co API khoi tao payment (QR/transfer metadata) gan voi order.
- [ ] Co webhook/payment callback de xac nhan thanh toan.
- [ ] Co state machine trang thai order ro rang, idempotent.
- [ ] Khi thanh toan thanh cong, tu dong tao enrollment cho cac khoa hoc trong order.
- [ ] Co co che retry + dead-letter logic cho webhook xu ly loi tam thoi.
- [ ] Co test (unit + integration) cho business-critical paths.

## 2) Scope khong lam trong phase 1

- [ ] Khong lam refund phuc tap da cong no (chi cho phep manual mark/refund basic neu can).
- [ ] Khong lam split payout instructor real-time.
- [ ] Khong lam shopping cart coupon stack (chi 1 coupon/order).

## 3) Kien truc module (de xep code)

- [ ] Them repository cho order: create/get/update status/list by user.
- [ ] Them repository cho order_items.
- [ ] Them repository cho coupon/coupon_usage (neu dung coupon).
- [ ] Them service PaymentService (tao payment intent + verify webhook + transition).
- [ ] Them service OrderService (pricing, validate, place order, complete order).
- [ ] Them handler + router cho order/payment.
- [ ] Dang ky vao app/repositories.go, app/services.go, app/handlers.go, router/route.go.

## 4) Data model va rang buoc

### 4.1 Orders

- [ ] order_number unique, de tra cuu ngoai.
- [ ] status enum: pending, processing, completed, failed, refunded, cancelled.
- [ ] tong tien: subtotal, discount_amount, tax_amount, total_amount.
- [ ] payment metadata: payment_method, payment_gateway, payment_transaction_id, paid_at.
- [ ] optimistic lock field (version) hoac updated_at check de tranh race update.

### 4.2 Order items

- [ ] Luu snapshot gia tai thoi diem mua: price, discount_amount, final_price.
- [ ] Rang buoc unique (order_id, course_id).

### 4.3 Coupon

- [ ] Validate active window (starts_at/expires_at).
- [ ] Validate min purchase, usage limit, per-user limit.
- [ ] Ghi coupon_usage theo order khi thanh toan thanh cong (hoac reserve + confirm).

### 4.4 Cart/Wishlist

- [ ] Cart item unique theo (user_id, course_id).
- [ ] Sau khi completed order, remove cart items da mua.

### 4.5 Bang bo sung de kiem soat race condition

- [ ] Tao bang `order_status_histories`:
  - [ ] id, order_id, from_status, to_status, reason, actor, created_at.
  - [ ] index (order_id, created_at DESC).
- [ ] Tao bang `payment_events`:
  - [ ] id, provider, event_id, order_id, transaction_id, amount, currency, raw_payload, headers, processed_at, status.
  - [ ] unique (provider, event_id) de chong replay.
  - [ ] unique (provider, transaction_id) de chong duplicate complete.
- [ ] Tao bang `idempotency_keys`:
  - [ ] id, key, scope, request_hash, response_snapshot, status_code, expires_at, created_at.
  - [ ] unique (scope, key).
- [ ] Tao bang `order_locks` (optional neu can advisory lock app-level):
  - [ ] order_id unique, lock_owner, lock_until.

## 5) API design checklist

## 5.1 Cart/Wishlist (neu chua co)

- [ ] POST /api/v1/cart/items
- [ ] GET /api/v1/cart/items
- [ ] DELETE /api/v1/cart/items/:course_id
- [ ] POST /api/v1/wishlist/items
- [ ] GET /api/v1/wishlist/items
- [ ] DELETE /api/v1/wishlist/items/:course_id

## 5.2 Order

- [ ] POST /api/v1/orders
- Input:
  - [ ] source = cart | buy_now
  - [ ] course_ids (neu buy_now)
  - [ ] coupon_code (optional)
  - [ ] note (optional)
- Output:
  - [ ] order_id, order_number
  - [ ] amount breakdown
  - [ ] expires_at

- [ ] GET /api/v1/orders/:id
- [ ] GET /api/v1/orders/me?page=&limit=&status=
- [ ] POST /api/v1/orders/:id/cancel (chi pending/processing co dieu kien)

## 5.3 Payment

- [ ] POST /api/v1/orders/:id/payment-intent
- Output:
  - [ ] payment_code
  - [ ] qr_content / bank transfer content
  - [ ] amount
  - [ ] expired_at

- [ ] POST /api/v1/payment/webhook
- [ ] GET /api/v1/orders/:id/payment-status (polling fallback cho FE)

## 6) Business rules quan trong

- [ ] Khong cho mua khoa hoc da enrolled.
- [ ] Khong cho mua duplicate course trong cung order.
- [ ] Gia cuoi cung tinh o BE, khong tin gia FE gui len.
- [ ] Verify amount trong webhook phai khop expected total.
- [ ] Verify payment reference/pin map dung order.
- [ ] Xu ly webhook idempotent (same tx khong duoc complete lan 2).
- [ ] Atomic flow complete order + create enrollments trong 1 DB transaction.

## 7) State machine (order status)

- [ ] pending -> processing (khi tao payment intent)
- [ ] processing -> completed (webhook valid + amount matched)
- [ ] processing -> failed (webhook fail/expired)
- [ ] pending -> cancelled (nguoi dung huy, chua thanh toan)
- [ ] completed -> refunded (phase sau/manual)

Rule:
- [ ] Cam transition nguoc trai phep.
- [ ] Log lich su transition (audit table hoac app log co correlation_id).

## 8) Idempotency, locking, concurrency

- [ ] Idempotency-Key cho POST /orders va /payment-intent.
- [ ] DB unique constraint cho external transaction id.
- [ ] Row-level lock khi complete order (SELECT FOR UPDATE) neu can.
- [ ] Neu webhook den truoc payment-status poll, he thong van nhat quan.

## 9) Webhook verification

- [ ] Verify signature secret (HMAC neu provider ho tro).
- [ ] Verify source IP/range neu provider co publish.
- [ ] Validate timestamp drift de tranh replay attack.
- [ ] Luu raw payload + headers de forensic.
- [ ] Replay protection (nonce/event_id unique).

## 10) Enrollment after payment

- [ ] Sau completed, tao enrollment cho tung course.
- [ ] Neu course private/archived, define policy ro (skip hay fail order).
- [ ] Neu 1 enrollment loi: rollback all (phase 1 de dam bao all-or-nothing).
- [ ] Gui notification payment_success cho user.

## 11) Error model va response codes

- [ ] 400: payload invalid, coupon invalid, transition invalid.
- [ ] 401/403: khong co quyen thao tac order cua nguoi khac.
- [ ] 404: order/coupon/course not found.
- [ ] 409: duplicate request, invalid state transition, already enrolled.
- [ ] 422: amount mismatch business validation.
- [ ] 500: unexpected internal.

- [ ] Thong nhat response shape: code, message, details, request_id.

## 12) Logging, metrics, tracing

- [ ] Correlation ID/request ID xuyen suot order + payment + webhook.
- [ ] Structured log fields: order_id, order_number, user_id, tx_id, status_from, status_to.
- [ ] Metrics:
  - [ ] order_created_total
  - [ ] order_completed_total
  - [ ] payment_webhook_invalid_total
  - [ ] payment_amount_mismatch_total
  - [ ] order_complete_latency_ms

## 13) Security checklist

- [ ] Khong expose thong tin nhay cam payment provider.
- [ ] Validate UUID/path params chat che.
- [ ] Rate limit endpoint webhook + create order.
- [ ] Sanitize note/description de tranh log injection.

## 14) Test plan chi tiet

### 14.1 Unit tests

- [ ] Pricing calc (subtotal/discount/tax/total).
- [ ] Coupon validation matrix.
- [ ] State transition validator.
- [ ] Idempotency behavior.

### 14.2 Integration tests

- [ ] Tao order cart success.
- [ ] Buy-now success.
- [ ] Webhook complete success.
- [ ] Webhook duplicate event (idempotent).
- [ ] Amount mismatch -> khong complete.
- [ ] User da enrolled -> reject.
- [ ] Cancel pending order.

### 14.3 Failure tests

- [ ] DB transaction fail khi create enrollment -> rollback order complete.
- [ ] Redis down (neu dung cache/lock) -> he thong van safe.
- [ ] Payment service timeout -> retry policy hoat dong.

## 15) Postman/API contract

- [ ] Tao collection payment_orders.json gom full flow:
  - [ ] add cart
  - [ ] create order
  - [ ] create payment intent
  - [ ] simulate webhook
  - [ ] check order status
  - [ ] verify enrollment

## 16) Migration va deploy strategy

- [ ] Tao migration cho index/constraint con thieu (neu co).
- [ ] Backfill data neu can (thuong khong can phase 1).
- [ ] Deploy theo thu tu:
  - [ ] deploy schema truoc
  - [ ] deploy API sau
  - [ ] enable webhook cuoi
- [ ] Feature flag cho payment flow neu muon rollout an toan.

## 17) Definition of Done

- [ ] FE co the mua khoa hoc end-to-end qua order va webhook.
- [ ] Order completed thi user nhan enrollment ngay.
- [ ] Khong co duplicate complete khi webhook replay.
- [ ] Monitoring co metric va log du de debug production.
- [ ] Test quan trong pass trong CI.

## 18) Thu tu implement de xong nhanh

- [ ] Step 1: repository + migration/index + DTO.
- [ ] Step 2: OrderService create/get/list/cancel.
- [ ] Step 3: PaymentService payment-intent + webhook idempotent.
- [ ] Step 4: complete order + enrollment transaction.
- [ ] Step 5: handlers + routers + app wiring.
- [ ] Step 6: tests + postman collection.
- [ ] Step 7: logs/metrics + hardening.

## 19) Notes mapping voi code hien tai

- Da co model nen tan dung:
  - Order, OrderItem, Coupon, CouponUsage, InstructorPayout.
- Da co utility tao payment code va QR helper:
  - GeneratePaymentCode
  - GenerateQRPayment
- Chua co wiring cho order/payment trong app services/handlers/routes.

## 20) Prompt mau theo bang (de implement nhanh, tranh race condition)

### 20.1 Prompt cho bang orders

Su dung prompt sau khi code service/repository:

"Implement repository + service cho bang orders voi cac yeu cau:
1) Moi update status phai di qua ham transition co validate state machine.
2) Dung transaction + SELECT FOR UPDATE khi complete order.
3) Neu status da completed/cancelled thi reject transition voi error code `ERR_INVALID_STATE_TRANSITION`.
4) Tat ca loi DB map thanh domain error ro rang (not found, conflict, transient).
5) Ghi history vao order_status_histories trong cung transaction."

### 20.2 Prompt cho bang order_items

"Implement create/list order_items voi cac yeu cau:
1) Unique (order_id, course_id), duplicate phai tra `ERR_DUPLICATE_ORDER_ITEM`.
2) Luu snapshot gia (price, discount_amount, final_price), khong phu thuoc gia hien tai cua course.
3) Validate tong final_price cua items phai khop subtotal cua order trong cung transaction.
4) Loi validation tra 422, loi constraint tra 409."

### 20.3 Prompt cho bang payment_events

"Implement webhook processor voi bang payment_events:
1) Upsert event theo unique (provider, event_id) de idempotent.
2) Neu duplicate event thi return success som, khong xu ly lai complete order.
3) Verify amount/currency/reference truoc khi transition order.
4) Luu raw_payload + headers de audit.
5) Neu loi transient (DB timeout/network) thi danh dau retryable, neu loi business thi mark failed khong retry."

### 20.4 Prompt cho bang idempotency_keys

"Implement middleware idempotency cho POST /orders va POST /payment-intent:
1) Key scope theo endpoint + user_id.
2) request_hash khac ma cung key -> tra `ERR_IDEMPOTENCY_PAYLOAD_MISMATCH`.
3) Neu key da co response thanh cong -> replay dung response cu.
4) Co TTL cleanup job cho key het han.
5) Dam bao atomic insert key + execute business (transaction hoac lock phu hop)."

### 20.5 Prompt cho bang enrollments (sau payment)

"Implement complete-order transaction:
1) Lock order row truoc, verify status processing.
2) Tao enrollment cho tung course trong order (all-or-nothing).
3) Neu trung enrollment thi rollback va set order failed voi reason ro rang.
4) Neu thanh cong, update order completed + paid_at + payment_transaction_id.
5) Emit notification event sau commit (outbox pattern neu co)."

## 21) Checklist xu ly loi va retry (bat buoc)

- [ ] Error taxonomy thong nhat:
  - [ ] `ERR_NOT_FOUND`
  - [ ] `ERR_CONFLICT`
  - [ ] `ERR_INVALID_STATE_TRANSITION`
  - [ ] `ERR_AMOUNT_MISMATCH`
  - [ ] `ERR_IDEMPOTENCY_PAYLOAD_MISMATCH`
  - [ ] `ERR_PROVIDER_SIGNATURE_INVALID`
  - [ ] `ERR_TRANSIENT_DEPENDENCY`
- [ ] Mapping HTTP:
  - [ ] 404 -> not found
  - [ ] 409 -> conflict/idempotency/duplicate
  - [ ] 422 -> amount/state business invalid
  - [ ] 500/503 -> transient infra
- [ ] Retry policy webhook:
  - [ ] exponential backoff (vi du 1s, 5s, 30s, 2m, 10m)
  - [ ] max retry + dead-letter queue
  - [ ] idempotent xu ly tren moi lan retry
- [ ] Logging khi loi:
  - [ ] request_id, order_id, event_id, tx_id, error_code, retry_count
  - [ ] khong log secret/signature raw

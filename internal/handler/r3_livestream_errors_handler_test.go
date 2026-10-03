package handler

// R3 (QA hồi quy 03/10/2026, B-10/B-21): lỗi đầu vào của livestream là 400, sai trạng thái là 409,
// không tồn tại là 404 — trước đây mọi thứ ngoài 403 đều rơi về 500.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/service"
)

func TestR3_Livestream_PhanLoaiLoi(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		method string
		path   string
		body   string
		want   int
	}{
		{"end chưa live", fmt.Errorf("%w: session is not live", service.ErrLivestreamStateConflict), "POST", "/livestream/:id/end", `{}`, fiber.StatusConflict},
		{"start sai trạng thái", fmt.Errorf("%w: cannot start", service.ErrLivestreamStateConflict), "POST", "/livestream/:id/start", `{}`, fiber.StatusConflict},
		{"update giờ sai", fmt.Errorf("%w: scheduled_at", service.ErrLivestreamInvalidInput), "PUT", "/livestream/:id", `{"title":"abc"}`, fiber.StatusBadRequest},
		{"end không tồn tại", service.ErrSessionNotFound, "POST", "/livestream/:id/end", `{}`, fiber.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &stubLivestreamService{authzErr: tc.err}
			h := NewLivestreamHandler(svc, nil)
			handle := h.End
			switch {
			case strings.HasSuffix(tc.path, "/start"):
				handle = h.Start
			case tc.method == "PUT":
				handle = h.Update
			}
			app := mountWithCaller(tc.method, tc.path, uuid.New(), handle)
			if got := doJSON(t, app, tc.method, strings.Replace(tc.path, ":id", uuid.New().String(), 1), tc.body); got != tc.want {
				t.Fatalf("status = %d, muốn %d", got, tc.want)
			}
		})
	}
}

// Chuỗi rỗng ở scheduled_end_at/location là "xoá giá trị đã lưu" nên phải qua được validate của handler;
// title ngắn hơn 3 ký tự (tag min=3 trước đây không được kiểm ở PUT) bị 400 trước khi tới service.
func TestR3_LivestreamUpdate_Validate(t *testing.T) {
	svc := &stubLivestreamService{}
	h := NewLivestreamHandler(svc, nil)
	app := mountWithCaller("PUT", "/livestream/:id", uuid.New(), h.Update)
	id := uuid.New().String()

	if got := doJSON(t, app, "PUT", "/livestream/"+id, `{"title":"ab"}`); got != fiber.StatusBadRequest {
		t.Fatalf("title quá ngắn: status = %d, muốn 400", got)
	}
	if svc.gotActorID != uuid.Nil {
		t.Error("service bị gọi dù body không hợp lệ")
	}
	if got := doJSON(t, app, "PUT", "/livestream/"+id, `{"scheduled_end_at":"","location":""}`); got != fiber.StatusOK {
		t.Fatalf("xoá giờ kết thúc/phòng: status = %d, muốn 200", got)
	}
}

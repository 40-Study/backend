package handler

// Test cho V3-6/V3-7 (issue #58): moi handler livestream phai lay danh tinh NGUOI GOI tu access
// token (c.Locals("user_id")), khong duoc lay tu body/URL; va loi uy quyen tu service phai thanh
// 403 chu khong phai 500. Hai kich ban lap cho tung handler da doi: nguoi la -> 403, host -> 200.

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/service"
)

// mountWithCaller dung mot route gia lap AuthMiddleware: dat c.Locals("user_id") = caller roi moi
// chay handler that. Nho vay test do duoc dung hanh vi khi da xac thuc — lop 401 da co test rieng
// (TestLivestreamCreate_RequiresAuth).
func mountWithCaller(method, path string, caller uuid.UUID, h fiber.Handler) *fiber.App {
	app := fiber.New()
	app.Add(method, path, func(c *fiber.Ctx) error {
		c.Locals("user_id", caller)
		return c.Next()
	}, h)
	return app
}

func doJSON(t *testing.T, app *fiber.App, method, path, body string) int {
	t.Helper()
	var req = httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	return resp.StatusCode
}

// --- Join: mao danh bang user_id trong body ------------------------------------------------

// TestLivestreamJoin_MaoDanhBangUserID_BiBoQua (kich ban chinh cua V3-6): attacker gui
// `user_id` cua NGUOI KHAC trong body. Handler PHAI truyen danh tinh cua chinh nguoi goi xuong
// service — neu con doc tu body thi userID truyen xuong se trung voi gia tri gia mao.
func TestLivestreamJoin_MaoDanhBangUserID_BiBoQua(t *testing.T) {
	svc := &stubLivestreamService{}
	h := NewLivestreamHandler(svc, nil)

	realCaller := uuid.New()
	victimInBody := uuid.New()
	sessionID := uuid.New()

	app := mountWithCaller("POST", "/livestream/:id/join", realCaller, h.Join)
	body := `{"user_id":"` + victimInBody.String() + `","name":"Ke mao danh","role":"teacher"}`

	status := doJSON(t, app, "POST", "/livestream/"+sessionID.String()+"/join", body)

	if status != fiber.StatusOK {
		t.Fatalf("status = %d, muon 200 (nguoi goi hop le)", status)
	}
	if svc.gotJoinUserID != realCaller {
		t.Errorf("userID truyen xuong service = %s, muon nguoi dang dang nhap = %s (bi anh huong boi user_id trong body!)",
			svc.gotJoinUserID, realCaller)
	}
	if svc.gotJoinUserID == victimInBody {
		t.Error("userID truyen xuong TRUNG voi user_id gia mao trong body — day chinh la lo hong mao danh")
	}
}

// TestLivestreamJoin_BoRoleTuBody: `role` trong body khong con duong vao DTO, nen khong the tac
// dong toi vai tro — vai tro do service suy ra tu quan he that trong DB.
func TestLivestreamJoin_BoRoleTuBody(t *testing.T) {
	svc := &stubLivestreamService{}
	h := NewLivestreamHandler(svc, nil)
	caller := uuid.New()
	sessionID := uuid.New()

	app := mountWithCaller("POST", "/livestream/:id/join", caller, h.Join)
	body := `{"name":"Hoc sinh tu phong","role":"teacher"}`

	status := doJSON(t, app, "POST", "/livestream/"+sessionID.String()+"/join", body)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, muon 200", status)
	}
	// dto.JoinLivestreamDTO khong con field Role — neu con, gia tri nay se la "teacher".
	if svc.gotJoinReq.Name != "Hoc sinh tu phong" {
		t.Errorf("name truyen xuong = %q, muon %q", svc.gotJoinReq.Name, "Hoc sinh tu phong")
	}
}

// TestLivestreamJoin_NguoiLa_Bi403: nguoi khong co quan he voi phien -> service tra
// ErrNotSessionMember -> handler phai map 403 (khong phai 500).
func TestLivestreamJoin_NguoiLa_Bi403(t *testing.T) {
	svc := &stubLivestreamService{authzErr: service.ErrNotSessionMember}
	h := NewLivestreamHandler(svc, nil)
	app := mountWithCaller("POST", "/livestream/:id/join", uuid.New(), h.Join)

	status := doJSON(t, app, "POST", "/livestream/"+uuid.New().String()+"/join", `{"name":"Nguoi la"}`)
	if status != fiber.StatusForbidden {
		t.Fatalf("status = %d, muon 403 khi khong phai thanh vien phien", status)
	}
}

// TestLivestreamJoin_KhongDangNhap_Bi401: khong co user_id trong Locals -> 401 truoc khi cham
// service (route that con AuthMiddleware, nhung handler phai tu chan duoc khi mount truc tiep).
func TestLivestreamJoin_KhongDangNhap_Bi401(t *testing.T) {
	svc := &stubLivestreamService{}
	h := NewLivestreamHandler(svc, nil)

	app := fiber.New()
	app.Post("/livestream/:id/join", h.Join)

	status := doJSON(t, app, "POST", "/livestream/"+uuid.New().String()+"/join", `{"name":"An danh"}`)
	if status != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, muon 401 khi chua dang nhap", status)
	}
	if svc.gotJoinUserID != uuid.Nil {
		t.Error("service.Join bi goi du chua dang nhap")
	}
}

// --- Leave: da nguoi khac ra khoi phong -----------------------------------------------------

// TestLivestreamLeave_MaoDanhBangUserID_BiBoQua: truoc day body `{"user_id": <nan nhan>}` cho
// phep bat ky ai da NGUOI KHAC ra khoi phong. Sau fix, nguoi roi phong luon la nguoi goi.
func TestLivestreamLeave_MaoDanhBangUserID_BiBoQua(t *testing.T) {
	svc := &stubLivestreamService{}
	h := NewLivestreamHandler(svc, nil)

	realCaller := uuid.New()
	victimInBody := uuid.New()
	sessionID := uuid.New()

	app := mountWithCaller("POST", "/livestream/:id/leave", realCaller, h.Leave)
	body := `{"user_id":"` + victimInBody.String() + `"}`

	status := doJSON(t, app, "POST", "/livestream/"+sessionID.String()+"/leave", body)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, muon 200", status)
	}
	if svc.gotLeaveUserID != realCaller {
		t.Errorf("userID truyen xuong service = %s, muon nguoi dang dang nhap = %s", svc.gotLeaveUserID, realCaller)
	}
	if svc.gotLeaveUserID == victimInBody {
		t.Error("da NGUOI KHAC ra khoi phong bang user_id trong body — lo hong V3-6")
	}
}

// TestLivestreamLeave_KhongCanBody: body rong van phai hoat dong — DTO leave da bi xoa hoan toan
// (khong con truong nao), nen khong duoc doi hoi body.
func TestLivestreamLeave_KhongCanBody(t *testing.T) {
	svc := &stubLivestreamService{}
	h := NewLivestreamHandler(svc, nil)
	caller := uuid.New()

	app := mountWithCaller("POST", "/livestream/:id/leave", caller, h.Leave)
	status := doJSON(t, app, "POST", "/livestream/"+uuid.New().String()+"/leave", ``)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, muon 200 voi body rong", status)
	}
	if svc.gotLeaveUserID != caller {
		t.Errorf("userID = %s, muon %s", svc.gotLeaveUserID, caller)
	}
}

// --- Nhom handler quan tri: nguoi la -> 403, host -> 200 ------------------------------------

// manageCase mo ta mot handler quan tri can kiem: cach mount, body, va ham doc trang thai da ghi
// vao stub (de chung minh actorID truyen xuong la nguoi goi chu khong phai ai khac).
type manageCase struct {
	name   string
	method string
	path   string
	body   string
	handle func(*LivestreamHandler) fiber.Handler
	// gotActor doc actorID ma handler da truyen xuong service.
	gotActor func(*stubLivestreamService) uuid.UUID
}

func manageCases() []manageCase {
	return []manageCase{
		{
			name: "Update", method: "PUT", path: "/livestream/:id",
			body:   `{"title":"Doi ten phien"}`,
			handle: func(h *LivestreamHandler) fiber.Handler { return h.Update },
			gotActor: func(s *stubLivestreamService) uuid.UUID {
				return s.gotActorID
			},
		},
		{
			name: "Delete", method: "DELETE", path: "/livestream/:id",
			body:   ``,
			handle: func(h *LivestreamHandler) fiber.Handler { return h.Delete },
			gotActor: func(s *stubLivestreamService) uuid.UUID {
				return s.gotActorID
			},
		},
		{
			name: "Start", method: "POST", path: "/livestream/:id/start",
			body:   `{}`,
			handle: func(h *LivestreamHandler) fiber.Handler { return h.Start },
			gotActor: func(s *stubLivestreamService) uuid.UUID {
				return s.gotActorID
			},
		},
		{
			name: "End", method: "POST", path: "/livestream/:id/end",
			body:   `{}`,
			handle: func(h *LivestreamHandler) fiber.Handler { return h.End },
			gotActor: func(s *stubLivestreamService) uuid.UUID {
				return s.gotActorID
			},
		},
		{
			name: "LockWhiteboard", method: "POST", path: "/livestream/:id/whiteboard/lock",
			body:   `{"locked":true}`,
			handle: func(h *LivestreamHandler) fiber.Handler { return h.LockWhiteboard },
			gotActor: func(s *stubLivestreamService) uuid.UUID {
				return s.gotActorID
			},
		},
		{
			name: "UnlockWhiteboard", method: "POST", path: "/livestream/:id/whiteboard/unlock",
			body:   `{"locked":false}`,
			handle: func(h *LivestreamHandler) fiber.Handler { return h.UnlockWhiteboard },
			gotActor: func(s *stubLivestreamService) uuid.UUID {
				return s.gotActorID
			},
		},
		{
			name: "StartScreenShare", method: "POST", path: "/livestream/:id/screen-share/start",
			body:   `{"action":"start"}`,
			handle: func(h *LivestreamHandler) fiber.Handler { return h.StartScreenShare },
			gotActor: func(s *stubLivestreamService) uuid.UUID {
				return s.gotActorID
			},
		},
		{
			name: "StopScreenShare", method: "POST", path: "/livestream/:id/screen-share/stop",
			body:   `{"action":"stop"}`,
			handle: func(h *LivestreamHandler) fiber.Handler { return h.StopScreenShare },
			gotActor: func(s *stubLivestreamService) uuid.UUID {
				return s.gotActorID
			},
		},
		{
			name: "MuteParticipant", method: "POST", path: "/livestream/:id/mute",
			body:   `{"user_id":"` + uuid.Nil.String() + `","action":"mute"}`,
			handle: func(h *LivestreamHandler) fiber.Handler { return h.MuteParticipant },
			gotActor: func(s *stubLivestreamService) uuid.UUID {
				return s.gotActorID
			},
		},
		{
			name: "KickParticipant", method: "POST", path: "/livestream/:id/kick",
			body:   `{"user_id":"` + uuid.Nil.String() + `","action":"kick"}`,
			handle: func(h *LivestreamHandler) fiber.Handler { return h.KickParticipant },
			gotActor: func(s *stubLivestreamService) uuid.UUID {
				return s.gotActorID
			},
		},
	}
}

// TestLivestreamManage_NguoiLa_Bi403: voi MOI handler quan tri, service tra ErrNotClassTeacher
// (nguoi goi khong phai host/GV lop/instructor khoa) -> phai la 403, khong duoc la 500.
func TestLivestreamManage_NguoiLa_Bi403(t *testing.T) {
	for _, tc := range manageCases() {
		t.Run(tc.name, func(t *testing.T) {
			svc := &stubLivestreamService{authzErr: service.ErrNotClassTeacher}
			h := NewLivestreamHandler(svc, nil)
			app := mountWithCaller(tc.method, tc.path, uuid.New(), tc.handle(h))

			status := doJSON(t, app, tc.method, strings.Replace(tc.path, ":id", uuid.New().String(), 1), tc.body)
			if status != fiber.StatusForbidden {
				t.Fatalf("status = %d, muon 403 khi khong co quyen quan tri phien", status)
			}
		})
	}
}

// TestLivestreamManage_Host_DuocPhep: cung bo handler do, khi service cho qua (host hop le) phai
// tra 2xx — chot chan rang viec them kiem quyen khong vo tinh chan luon ca nguoi co quyen.
func TestLivestreamManage_Host_DuocPhep(t *testing.T) {
	for _, tc := range manageCases() {
		t.Run(tc.name, func(t *testing.T) {
			svc := &stubLivestreamService{}
			h := NewLivestreamHandler(svc, nil)
			hostID := uuid.New()
			app := mountWithCaller(tc.method, tc.path, hostID, tc.handle(h))

			status := doJSON(t, app, tc.method, strings.Replace(tc.path, ":id", uuid.New().String(), 1), tc.body)
			if status < 200 || status > 299 {
				t.Fatalf("status = %d, muon 2xx khi host hop le", status)
			}
			if got := tc.gotActor(svc); got != hostID {
				t.Errorf("actorID truyen xuong = %s, muon nguoi dang dang nhap = %s", got, hostID)
			}
		})
	}
}

// TestLivestreamManage_KhongDangNhap_Bi401: chua xac thuc -> 401 truoc khi cham service, voi MOI
// handler quan tri.
func TestLivestreamManage_KhongDangNhap_Bi401(t *testing.T) {
	for _, tc := range manageCases() {
		t.Run(tc.name, func(t *testing.T) {
			svc := &stubLivestreamService{}
			h := NewLivestreamHandler(svc, nil)
			app := fiber.New()
			app.Add(tc.method, tc.path, tc.handle(h))

			status := doJSON(t, app, tc.method, strings.Replace(tc.path, ":id", uuid.New().String(), 1), tc.body)
			if status != fiber.StatusUnauthorized {
				t.Fatalf("status = %d, muon 401 khi chua dang nhap", status)
			}
			if got := tc.gotActor(svc); got != uuid.Nil {
				t.Error("service bi goi du chua dang nhap")
			}
		})
	}
}

// TestLivestreamKick_ChanKickHost_Bi403 (V3-7): service tra ErrCannotKickHost -> 403. Day la
// truong hop rieng cua nhom tren nhung duoc tach ra vi day la loi ve DOI TUONG bi tac dong chu
// khong phai ve nguoi goi — mot GV lop (co quyen quan tri) van khong duoc da host ra.
func TestLivestreamKick_ChanKickHost_Bi403(t *testing.T) {
	svc := &stubLivestreamService{authzErr: service.ErrCannotKickHost}
	h := NewLivestreamHandler(svc, nil)
	app := mountWithCaller("POST", "/livestream/:id/kick", uuid.New(), h.KickParticipant)

	body := `{"user_id":"` + uuid.New().String() + `","action":"kick"}`
	status := doJSON(t, app, "POST", "/livestream/"+uuid.New().String()+"/kick", body)
	if status != fiber.StatusForbidden {
		t.Fatalf("status = %d, muon 403 khi khong duoc da host ra khoi phong", status)
	}
}

// TestLivestreamKick_DoiTuongLayTuBody: `user_id` trong body la DOI TUONG bi kick (khac voi actor
// lay tu token) — phai duoc truyen xuong dung, neu khong thi moderation khong con tac dung.
func TestLivestreamKick_DoiTuongLayTuBody(t *testing.T) {
	svc := &stubLivestreamService{}
	h := NewLivestreamHandler(svc, nil)
	hostID, targetID := uuid.New(), uuid.New()
	app := mountWithCaller("POST", "/livestream/:id/kick", hostID, h.KickParticipant)

	body := `{"user_id":"` + targetID.String() + `","action":"kick"}`
	status := doJSON(t, app, "POST", "/livestream/"+uuid.New().String()+"/kick", body)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, muon 200", status)
	}
	if svc.gotActorID != hostID {
		t.Errorf("actorID = %s, muon nguoi goi %s", svc.gotActorID, hostID)
	}
	if svc.gotTargetID != targetID {
		t.Errorf("targetID = %s, muon doi tuong trong body %s", svc.gotTargetID, targetID)
	}
}

// TestLivestreamScreenShare_ActorLuonLayTuToken_TargetLayTuBody (D3, review vong 2 260915): sau D3,
// `user_id` trong body la MUC TIEU hop le (host duyet chia se man hinh CHO mot hoc sinh cu the) —
// day KHONG con la lo hong mao danh nhu Phase 1 (khi StartScreenShare chua co khai niem target).
// Diem UY QUYEN van dam bao o tang service (getManageableSession trong StartScreenShare that —
// stub o day chi xac nhan handler truyen dung 2 gia tri: actorID tu TOKEN, targetID tu BODY.
func TestLivestreamScreenShare_ActorLuonLayTuToken_TargetLayTuBody(t *testing.T) {
	svc := &stubLivestreamService{}
	h := NewLivestreamHandler(svc, nil)
	caller, victim := uuid.New(), uuid.New()
	app := mountWithCaller("POST", "/livestream/:id/screen-share/start", caller, h.StartScreenShare)

	body := `{"user_id":"` + victim.String() + `","action":"start"}`
	status := doJSON(t, app, "POST", "/livestream/"+uuid.New().String()+"/screen-share/start", body)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, muon 200", status)
	}
	if svc.gotActorID != caller {
		t.Errorf("actorID = %s, muon nguoi goi (tu token) %s — actorID KHONG duoc lay tu body", svc.gotActorID, caller)
	}
	if svc.gotTargetID != victim {
		t.Errorf("targetID = %s, muon doi tuong trong body %s (D3: host duyet cho mot hoc sinh cu the)", svc.gotTargetID, victim)
	}
}

// TestLivestreamScreenShare_KhongTruyenUserID_TargetLaChinhActor (D3): khi body khong co user_id
// (truong hop hoc sinh/giao vien tu chia se man hinh cua chinh minh), target phai mac dinh la actor.
func TestLivestreamScreenShare_KhongTruyenUserID_TargetLaChinhActor(t *testing.T) {
	svc := &stubLivestreamService{}
	h := NewLivestreamHandler(svc, nil)
	caller := uuid.New()
	app := mountWithCaller("POST", "/livestream/:id/screen-share/start", caller, h.StartScreenShare)

	status := doJSON(t, app, "POST", "/livestream/"+uuid.New().String()+"/screen-share/start", `{"action":"start"}`)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, muon 200", status)
	}
	if svc.gotTargetID != caller {
		t.Errorf("targetID = %s, muon mac dinh la actor %s khi body khong truyen user_id", svc.gotTargetID, caller)
	}
}

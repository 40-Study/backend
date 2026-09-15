package handler

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/service"
)

// stubLivestreamService: chi Create duoc dung trong pham vi test nay — cac method khac cua
// LivestreamServiceInterface panic voi nil pointer neu bi goi, dung y muon de lo ngay khi mot
// test cham vao thu ngoai pham vi thay vi im lang tra zero value.
type stubLivestreamService struct {
	gotHostID uuid.UUID
	gotReq    dto.CreateLivestreamDTO
	called    int
	// createErr (N1, review vong 2 260915): khi khac nil, Create tra loi nay thay vi thanh cong —
	// dung de gia lap ErrNotClassTeacher tu tang service ma khong can dung service that.
	createErr error
	// gotLessonContentIDFilter (N10, review vong 2): GetAll ghi lai tham so nay de test kiem
	// tra handler co parse dung query ?lesson_content_id= khong.
	gotLessonContentIDFilter *uuid.UUID
	getAllCalled             int

	// V3-6/V3-7 (issue #58): cac truong duoi day ghi lai danh tinh nguoi goi / doi tuong ma
	// handler truyen xuong, de test chung minh handler KHONG con lay chung tu body.
	gotActorID       uuid.UUID
	gotSessionID     uuid.UUID
	gotTargetID      uuid.UUID
	gotJoinUserID    uuid.UUID
	gotLeaveUserID   uuid.UUID
	gotJoinReq       dto.JoinLivestreamDTO
	gotScreenShareID uuid.UUID
	gotLocked        bool
	// authzErr: khi khac nil, MOI method quan tri tra loi nay — dung de gia lap 403 tu service.
	authzErr error
}

func (s *stubLivestreamService) Create(ctx context.Context, hostID uuid.UUID, req dto.CreateLivestreamDTO) (*model.LivestreamSession, error) {
	s.called++
	s.gotHostID = hostID
	s.gotReq = req
	if s.createErr != nil {
		return nil, s.createErr
	}
	return &model.LivestreamSession{
		BaseModel: model.BaseModel{ID: uuid.New()},
		HostID:    hostID,
	}, nil
}
func (s *stubLivestreamService) GetByID(ctx context.Context, userID uuid.UUID, isAdmin bool, id uuid.UUID) (*dto.LivestreamDetailDTO, error) {
	return nil, errors.New("not used in these tests")
}
func (s *stubLivestreamService) GetAll(ctx context.Context, userID uuid.UUID, isAdmin bool, page, pageSize int, status string, hostID *uuid.UUID, lessonContentID *uuid.UUID) (*dto.LivestreamListDTO, error) {
	s.getAllCalled++
	s.gotLessonContentIDFilter = lessonContentID
	return &dto.LivestreamListDTO{Data: []dto.LivestreamResponseDTO{}, Total: 0, Page: page, PageSize: pageSize}, nil
}
func (s *stubLivestreamService) Update(ctx context.Context, userID uuid.UUID, isAdmin bool, id uuid.UUID, req dto.UpdateLivestreamDTO) (*model.LivestreamSession, error) {
	s.gotActorID, s.gotSessionID = userID, id
	if s.authzErr != nil {
		return nil, s.authzErr
	}
	return &model.LivestreamSession{BaseModel: model.BaseModel{ID: id}, HostID: userID}, nil
}
func (s *stubLivestreamService) Delete(ctx context.Context, userID uuid.UUID, isAdmin bool, id uuid.UUID) error {
	s.gotActorID, s.gotSessionID = userID, id
	return s.authzErr
}
func (s *stubLivestreamService) Start(ctx context.Context, userID uuid.UUID, isAdmin bool, id uuid.UUID) (*model.LivestreamSession, error) {
	s.gotActorID, s.gotSessionID = userID, id
	if s.authzErr != nil {
		return nil, s.authzErr
	}
	return &model.LivestreamSession{BaseModel: model.BaseModel{ID: id}}, nil
}
func (s *stubLivestreamService) End(ctx context.Context, userID uuid.UUID, isAdmin bool, id uuid.UUID) (*model.LivestreamSession, error) {
	s.gotActorID, s.gotSessionID = userID, id
	if s.authzErr != nil {
		return nil, s.authzErr
	}
	return &model.LivestreamSession{BaseModel: model.BaseModel{ID: id}}, nil
}
func (s *stubLivestreamService) Join(ctx context.Context, sessionID, userID uuid.UUID, isAdmin bool, req dto.JoinLivestreamDTO) (*dto.ParticipantResponseDTO, error) {
	s.gotSessionID, s.gotJoinUserID, s.gotJoinReq = sessionID, userID, req
	if s.authzErr != nil {
		return nil, s.authzErr
	}
	return &dto.ParticipantResponseDTO{
		ID:       uuid.New(),
		UserID:   userID,
		Role:     string(model.ParticipantRoleStudent),
		Token:    "fake-token",
		JoinedAt: "2026-01-01T00:00:00Z",
	}, nil
}
func (s *stubLivestreamService) Leave(ctx context.Context, sessionID, userID uuid.UUID) error {
	s.gotSessionID, s.gotLeaveUserID = sessionID, userID
	return s.authzErr
}
func (s *stubLivestreamService) GetParticipants(ctx context.Context, userID uuid.UUID, isAdmin bool, sessionID uuid.UUID, page, pageSize int) ([]model.Participant, int64, error) {
	return nil, 0, errors.New("not used in these tests")
}
func (s *stubLivestreamService) MuteParticipant(ctx context.Context, actorID uuid.UUID, isAdmin bool, sessionID, targetID uuid.UUID) error {
	s.gotActorID, s.gotSessionID, s.gotTargetID = actorID, sessionID, targetID
	return s.authzErr
}
func (s *stubLivestreamService) KickParticipant(ctx context.Context, actorID uuid.UUID, isAdmin bool, sessionID, targetID uuid.UUID) error {
	s.gotActorID, s.gotSessionID, s.gotTargetID = actorID, sessionID, targetID
	return s.authzErr
}
func (s *stubLivestreamService) LockWhiteboard(ctx context.Context, actorID uuid.UUID, isAdmin bool, sessionID uuid.UUID, locked bool) error {
	s.gotActorID, s.gotSessionID, s.gotLocked = actorID, sessionID, locked
	return s.authzErr
}
func (s *stubLivestreamService) StartScreenShare(ctx context.Context, actorID uuid.UUID, isAdmin bool, sessionID, targetID uuid.UUID) error {
	s.gotActorID, s.gotScreenShareID, s.gotTargetID = actorID, sessionID, targetID
	return s.authzErr
}
func (s *stubLivestreamService) StopScreenShare(ctx context.Context, actorID uuid.UUID, isAdmin bool, sessionID, targetID uuid.UUID) error {
	s.gotActorID, s.gotScreenShareID, s.gotTargetID = actorID, sessionID, targetID
	return s.authzErr
}
func (s *stubLivestreamService) EnsureSessionMember(ctx context.Context, sessionID, userID uuid.UUID) error {
	s.gotActorID, s.gotSessionID = userID, sessionID
	return s.authzErr
}
func (s *stubLivestreamService) EnsureSessionManage(ctx context.Context, sessionID, userID uuid.UUID) error {
	s.gotActorID, s.gotSessionID = userID, sessionID
	return s.authzErr
}

// TestLivestreamCreate_BoQuaHostIDTuBody (review 260915, tu PR web #16): client gui them
// "host_id" trong body (kich ban tan cong: doan/lay UUID cua mot user KHAC, vd giao vien, roi tu
// nhan la host cua phien). CreateLivestreamDTO khong con field host_id nen BodyParser lang le bo
// qua no — session PHAI mang HostID cua nguoi dang dang nhap (Locals user_id), khong phai gia tri
// trong body.
func TestLivestreamCreate_BoQuaHostIDTuBody(t *testing.T) {
	svc := &stubLivestreamService{}
	h := NewLivestreamHandler(svc, nil)

	realCaller := uuid.New()
	foreignHostIDInBody := uuid.New() // "nan nhan" ma attacker muon mao danh

	app := fiber.New()
	app.Post("/livestream", func(c *fiber.Ctx) error {
		c.Locals("user_id", realCaller)
		return c.Next()
	}, h.Create)

	body := `{"title":"Buoi hoc Toan","class_id":"` + uuid.New().String() +
		`","host_id":"` + foreignHostIDInBody.String() + `"}`
	req := httptest.NewRequest("POST", "/livestream", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("status = %d, muon 201", resp.StatusCode)
	}
	if svc.called != 1 {
		t.Fatalf("service.Create duoc goi %d lan, muon 1", svc.called)
	}
	if svc.gotHostID != realCaller {
		t.Errorf("hostID truyen xuong service = %s, muon nguoi dang dang nhap = %s (bi anh huong boi host_id trong body!)",
			svc.gotHostID, realCaller)
	}
	if svc.gotHostID == foreignHostIDInBody {
		t.Error("hostID truyen xuong service TRUNG voi host_id gia mao trong body — day chinh la lo hong mao danh")
	}
}

// TestLivestreamCreate_RequiresAuth: khong co user_id trong Locals (chua dang nhap) phai bi 401
// truoc khi cham service — extractUserID la lop bao ve duy nhat kien tra dieu nay khi mount truc
// tiep (route that con co them AuthMiddleware o tang router).
func TestLivestreamCreate_RequiresAuth(t *testing.T) {
	svc := &stubLivestreamService{}
	h := NewLivestreamHandler(svc, nil)

	app := fiber.New()
	app.Post("/livestream", h.Create)

	body := `{"title":"Buoi hoc","class_id":"` + uuid.New().String() + `"}`
	req := httptest.NewRequest("POST", "/livestream", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, muon 401", resp.StatusCode)
	}
	if svc.called != 0 {
		t.Errorf("service.Create bi goi %d lan du chua dang nhap", svc.called)
	}
}

// TestLivestreamCreate_ValidatesRequiredFields (LOW tu finding review 260915: handler truoc day
// khong goi ValidateStruct — title rong/class_id sai dinh dang van toi thang service). Sau fix,
// thieu title/class_id phai bi chan 400 truoc khi cham service.
func TestLivestreamCreate_ValidatesRequiredFields(t *testing.T) {
	svc := &stubLivestreamService{}
	h := NewLivestreamHandler(svc, nil)

	app := fiber.New()
	app.Post("/livestream", func(c *fiber.Ctx) error {
		c.Locals("user_id", uuid.New())
		return c.Next()
	}, h.Create)

	// Thieu ca title lan class_id.
	req := httptest.NewRequest("POST", "/livestream", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, muon 400 khi thieu title/class_id", resp.StatusCode)
	}
	if svc.called != 0 {
		t.Errorf("service.Create bi goi %d lan du request khong hop le", svc.called)
	}
}

// TestLivestreamCreate_ForbiddenKhiKhongPhaiGiaoVienLop (N1, review vong 2 260915): service tra
// ve service.ErrNotClassTeacher -> handler phai map ve 403, khong phai 500 mac dinh — day la loi
// UY QUYEN, khong phai loi ha tang.
func TestLivestreamCreate_ForbiddenKhiKhongPhaiGiaoVienLop(t *testing.T) {
	svc := &stubLivestreamService{createErr: service.ErrNotClassTeacher}
	h := NewLivestreamHandler(svc, nil)

	app := fiber.New()
	app.Post("/livestream", func(c *fiber.Ctx) error {
		c.Locals("user_id", uuid.New())
		return c.Next()
	}, h.Create)

	body := `{"title":"Buoi hoc gia mao","class_id":"` + uuid.New().String() + `"}`
	req := httptest.NewRequest("POST", "/livestream", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("status = %d, muon 403 khi service tra ErrNotClassTeacher", resp.StatusCode)
	}
}

// TestLivestreamCreate_ScheduledAtSaiDinhDang_BiTuChoi400 (N9, review vong 2 260915): truoc day
// livestream_service.go nuot loi parse RFC3339 cua scheduled_at (`if err == nil { ... }`) — client
// go sai dinh dang van nhan 201, phien duoc tao KHONG lich va KHONG enqueue reminder ma khong biet.
// Sau khi them validate tag vao DTO, handler phai chan 400 TRUOC KHI cham service.
func TestLivestreamCreate_ScheduledAtSaiDinhDang_BiTuChoi400(t *testing.T) {
	svc := &stubLivestreamService{}
	h := NewLivestreamHandler(svc, nil)

	app := fiber.New()
	app.Post("/livestream", func(c *fiber.Ctx) error {
		c.Locals("user_id", uuid.New())
		return c.Next()
	}, h.Create)

	body := `{"title":"Buoi hoc Toan","class_id":"` + uuid.New().String() +
		`","scheduled_at":"khong-phai-RFC3339"}`
	req := httptest.NewRequest("POST", "/livestream", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, muon 400 (scheduled_at sai dinh dang phai bi chan truoc khi cham service)", resp.StatusCode)
	}
	if svc.called != 0 {
		t.Errorf("service.Create bi goi %d lan du scheduled_at khong hop le — phien co the da duoc tao ma khong co lich/reminder", svc.called)
	}
}

// TestLivestreamCreate_ScheduledAtHopLeRFC3339_DuocChapNhan: chot chan hoi quy — mot gia tri
// RFC3339 hop le van phai qua duoc validate va toi service, khong bi tag moi chan oan.
func TestLivestreamCreate_ScheduledAtHopLeRFC3339_DuocChapNhan(t *testing.T) {
	svc := &stubLivestreamService{}
	h := NewLivestreamHandler(svc, nil)

	app := fiber.New()
	app.Post("/livestream", func(c *fiber.Ctx) error {
		c.Locals("user_id", uuid.New())
		return c.Next()
	}, h.Create)

	body := `{"title":"Buoi hoc Toan","class_id":"` + uuid.New().String() +
		`","scheduled_at":"2026-12-01T10:00:00Z"}`
	req := httptest.NewRequest("POST", "/livestream", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("status = %d, muon 201 (scheduled_at hop le RFC3339)", resp.StatusCode)
	}
	if svc.called != 1 {
		t.Errorf("service.Create duoc goi %d lan, muon 1", svc.called)
	}
}

// TestLivestreamGetAll_LocTheoLessonContentID (N10, review vong 2 — "neu re"): query
// ?lesson_content_id=<uuid> phai duoc parse va truyen xuong service.GetAll dung uuid.
func TestLivestreamGetAll_LocTheoLessonContentID(t *testing.T) {
	svc := &stubLivestreamService{}
	h := NewLivestreamHandler(svc, nil)

	// F-1 (issue #58 review vong 2): GetAll gio doi hoi dang nhap (callerOrUnauthorized) truoc
	// khi cham service — truoc day handler nay khong doc user_id nen mount thang khong Locals
	// van qua duoc; nay phai gia lap AuthMiddleware nhu cac test khac.
	app := fiber.New()
	app.Get("/livestream", func(c *fiber.Ctx) error {
		c.Locals("user_id", uuid.New())
		return c.Next()
	}, h.GetAll)

	lessonContentID := uuid.New()
	req := httptest.NewRequest("GET", "/livestream?lesson_content_id="+lessonContentID.String(), nil)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, muon 200", resp.StatusCode)
	}
	if svc.getAllCalled != 1 {
		t.Fatalf("service.GetAll duoc goi %d lan, muon 1", svc.getAllCalled)
	}
	if svc.gotLessonContentIDFilter == nil {
		t.Fatal("lessonContentID truyen xuong service la nil du query co gan ?lesson_content_id=")
	}
	if *svc.gotLessonContentIDFilter != lessonContentID {
		t.Errorf("lessonContentID = %s, muon %s", *svc.gotLessonContentIDFilter, lessonContentID)
	}
}

// TestLivestreamGetAll_KhongCoLessonContentID_KhongLoc: khong dat query -> filter phai la nil,
// khong duoc suy dien ra mot gia tri rac (vd uuid.Nil).
func TestLivestreamGetAll_KhongCoLessonContentID_KhongLoc(t *testing.T) {
	svc := &stubLivestreamService{}
	h := NewLivestreamHandler(svc, nil)

	// F-1: xem ghi chu trong TestLivestreamGetAll_LocTheoLessonContentID o tren.
	app := fiber.New()
	app.Get("/livestream", func(c *fiber.Ctx) error {
		c.Locals("user_id", uuid.New())
		return c.Next()
	}, h.GetAll)

	req := httptest.NewRequest("GET", "/livestream", nil)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test loi: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, muon 200", resp.StatusCode)
	}
	if svc.gotLessonContentIDFilter != nil {
		t.Errorf("lessonContentID = %v, muon nil khi khong co query", *svc.gotLessonContentIDFilter)
	}
}

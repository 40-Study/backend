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
)

// stubLivestreamService: chi Create duoc dung trong pham vi test nay — cac method khac cua
// LivestreamServiceInterface panic voi nil pointer neu bi goi, dung y muon de lo ngay khi mot
// test cham vao thu ngoai pham vi thay vi im lang tra zero value.
type stubLivestreamService struct {
	gotHostID uuid.UUID
	gotReq    dto.CreateLivestreamDTO
	called    int
}

func (s *stubLivestreamService) Create(ctx context.Context, hostID uuid.UUID, req dto.CreateLivestreamDTO) (*model.LivestreamSession, error) {
	s.called++
	s.gotHostID = hostID
	s.gotReq = req
	return &model.LivestreamSession{
		BaseModel: model.BaseModel{ID: uuid.New()},
		HostID:    hostID,
	}, nil
}
func (s *stubLivestreamService) GetByID(ctx context.Context, id uuid.UUID) (*dto.LivestreamDetailDTO, error) {
	return nil, errors.New("not used in these tests")
}
func (s *stubLivestreamService) GetAll(ctx context.Context, page, pageSize int, status string, hostID *uuid.UUID) (*dto.LivestreamListDTO, error) {
	return nil, errors.New("not used in these tests")
}
func (s *stubLivestreamService) Update(ctx context.Context, id uuid.UUID, req dto.UpdateLivestreamDTO) (*model.LivestreamSession, error) {
	return nil, errors.New("not used in these tests")
}
func (s *stubLivestreamService) Delete(ctx context.Context, id uuid.UUID) error {
	return errors.New("not used in these tests")
}
func (s *stubLivestreamService) Start(ctx context.Context, id uuid.UUID) (*model.LivestreamSession, error) {
	return nil, errors.New("not used in these tests")
}
func (s *stubLivestreamService) End(ctx context.Context, id uuid.UUID) (*model.LivestreamSession, error) {
	return nil, errors.New("not used in these tests")
}
func (s *stubLivestreamService) Join(ctx context.Context, sessionID uuid.UUID, req dto.JoinLivestreamDTO) (*dto.ParticipantResponseDTO, error) {
	return nil, errors.New("not used in these tests")
}
func (s *stubLivestreamService) Leave(ctx context.Context, sessionID uuid.UUID, req dto.LeaveLivestreamDTO) error {
	return errors.New("not used in these tests")
}
func (s *stubLivestreamService) GetParticipants(ctx context.Context, sessionID uuid.UUID, page, pageSize int) ([]model.Participant, int64, error) {
	return nil, 0, errors.New("not used in these tests")
}
func (s *stubLivestreamService) MuteParticipant(ctx context.Context, sessionID, userID uuid.UUID) error {
	return errors.New("not used in these tests")
}
func (s *stubLivestreamService) KickParticipant(ctx context.Context, sessionID, userID uuid.UUID) error {
	return errors.New("not used in these tests")
}
func (s *stubLivestreamService) LockWhiteboard(ctx context.Context, sessionID uuid.UUID, locked bool) error {
	return errors.New("not used in these tests")
}
func (s *stubLivestreamService) StartScreenShare(ctx context.Context, sessionID, userID uuid.UUID) error {
	return errors.New("not used in these tests")
}
func (s *stubLivestreamService) StopScreenShare(ctx context.Context, sessionID, userID uuid.UUID) error {
	return errors.New("not used in these tests")
}

// TestLivestreamCreate_BoQuaHostIDTuBody (review 260915, tu PR web #16): client gui them
// "host_id" trong body (kich ban tan cong: doan/lay UUID cua mot user KHAC, vd giao vien, roi tu
// nhan la host cua phien). CreateLivestreamDTO khong con field host_id nen BodyParser lang le bo
// qua no — session PHAI mang HostID cua nguoi dang dang nhap (Locals user_id), khong phai gia tri
// trong body.
func TestLivestreamCreate_BoQuaHostIDTuBody(t *testing.T) {
	svc := &stubLivestreamService{}
	h := NewLivestreamHandler(svc)

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
	h := NewLivestreamHandler(svc)

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
	h := NewLivestreamHandler(svc)

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

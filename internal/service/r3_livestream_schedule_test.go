package service

// R3 (QA hồi quy 03/10/2026): B-09 (sửa lịch/phòng của buổi livestream), B-10 và B-21 (giờ sai/quá khứ
// là 400, thao tác sai trạng thái là 409 thay vì 500). Không cần DB: dùng fake repo như
// livestream_service_test.go. Bỏ validateLivestreamWindow hoặc nhánh cập nhật lịch trong Update thì ĐỎ.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
)

// r3LivestreamRepo giữ một phiên trong bộ nhớ; Update ghi lại bản cuối để test đọc.
type r3LivestreamRepo struct {
	fakeLivestreamRepoHostTest
	session *model.LivestreamSession
	updated *model.LivestreamSession
}

func (r *r3LivestreamRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.LivestreamSession, error) {
	return r.session, nil
}

func (r *r3LivestreamRepo) Update(ctx context.Context, s *model.LivestreamSession) error {
	r.updated = s
	return nil
}

func newR3LivestreamSvc(repo *r3LivestreamRepo) *LivestreamService {
	return NewLivestreamService(repo, nil, &fakeAnalyticsRepoHostTest{}, &fakeClassRepoAllowAll{}, nil, nil, nil, nil, nil, nil)
}

func rfc(t time.Time) string { return t.Format(time.RFC3339) }

func TestR3_LivestreamCreate_GioSai(t *testing.T) {
	host := uuid.New()
	base := dto.CreateLivestreamDTO{Title: "Buoi hoc", ClassID: uuid.NewString()}
	future := time.Now().Add(48 * time.Hour)

	cases := []struct {
		name  string
		start string
		end   string
		want  error // nil = tạo được
	}{
		{"quá khứ", rfc(time.Now().Add(-2 * time.Hour)), "", ErrLivestreamInvalidInput},
		{"kết thúc trước bắt đầu", rfc(future), rfc(future.Add(-time.Hour)), ErrLivestreamInvalidInput},
		{"kết thúc bằng bắt đầu", rfc(future), rfc(future), ErrLivestreamInvalidInput},
		{"kết thúc mà không có bắt đầu", "", rfc(future), ErrLivestreamInvalidInput},
		{"định dạng sai", "ngày mai", "", ErrLivestreamInvalidInput},
		{"hợp lệ", rfc(future), rfc(future.Add(90 * time.Minute)), nil},
		{"chỉ bắt đầu", rfc(future), "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &r3LivestreamRepo{}
			svc := newR3LivestreamSvc(repo)
			req := base
			req.ScheduledAt, req.ScheduledEndAt, req.Location = tc.start, tc.end, "Phong A101"
			got, err := svc.Create(context.Background(), host, req)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v, muốn %v", err, tc.want)
			}
			if tc.want != nil {
				if repo.created != nil {
					t.Error("giờ sai nhưng vẫn ghi buổi xuống DB")
				}
				return
			}
			if got.Location == nil || *got.Location != "Phong A101" {
				t.Errorf("phòng không được lưu: %v", got.Location)
			}
			if tc.end != "" && (got.ScheduledEndAt == nil || got.ScheduledEndAt.Sub(*got.ScheduledAt) != 90*time.Minute) {
				t.Errorf("giờ kết thúc không được lưu đúng: %v", got.ScheduledEndAt)
			}
		})
	}
}

func TestR3_LivestreamUpdate_DoiLichVaPhong(t *testing.T) {
	host := uuid.New()
	newSession := func(status model.LivestreamSessionStatus) *r3LivestreamRepo {
		start := time.Now().Add(24 * time.Hour)
		return &r3LivestreamRepo{session: &model.LivestreamSession{
			BaseModel: model.BaseModel{ID: uuid.New()}, HostID: host, ClassID: uuid.New(),
			Status: status, ScheduledAt: &start,
		}}
	}
	ctx := context.Background()
	str := func(s string) *string { return &s }
	newStart := time.Now().Add(72 * time.Hour)

	t.Run("đổi giờ bắt đầu, kết thúc và phòng", func(t *testing.T) {
		repo := newSession(model.LivestreamStatusScheduled)
		svc := newR3LivestreamSvc(repo)
		_, err := svc.Update(ctx, host, false, repo.session.ID, dto.UpdateLivestreamDTO{
			ScheduledAt: str(rfc(newStart)), ScheduledEndAt: str(rfc(newStart.Add(2 * time.Hour))), Location: str("Phong B2"),
		})
		if err != nil {
			t.Fatal(err)
		}
		u := repo.updated
		if u == nil || u.ScheduledAt.Unix() != newStart.Unix() || u.ScheduledEndAt == nil || u.ScheduledEndAt.Sub(*u.ScheduledAt) != 2*time.Hour {
			t.Fatalf("lịch mới không được lưu: %+v", u)
		}
		if u.Location == nil || *u.Location != "Phong B2" {
			t.Errorf("phòng mới không được lưu: %v", u.Location)
		}
	})

	t.Run("chuỗi rỗng xoá giờ kết thúc và phòng", func(t *testing.T) {
		repo := newSession(model.LivestreamStatusScheduled)
		end := repo.session.ScheduledAt.Add(time.Hour)
		repo.session.ScheduledEndAt, repo.session.Location = &end, str("cũ")
		svc := newR3LivestreamSvc(repo)
		if _, err := svc.Update(ctx, host, false, repo.session.ID, dto.UpdateLivestreamDTO{ScheduledEndAt: str(""), Location: str("")}); err != nil {
			t.Fatal(err)
		}
		if repo.updated.ScheduledEndAt != nil || repo.updated.Location != nil {
			t.Errorf("chưa xoá: end=%v location=%v", repo.updated.ScheduledEndAt, repo.updated.Location)
		}
	})

	t.Run("dời sang quá khứ là 400", func(t *testing.T) {
		repo := newSession(model.LivestreamStatusScheduled)
		_, err := newR3LivestreamSvc(repo).Update(ctx, host, false, repo.session.ID, dto.UpdateLivestreamDTO{ScheduledAt: str(rfc(time.Now().Add(-time.Hour)))})
		if !errors.Is(err, ErrLivestreamInvalidInput) || repo.updated != nil {
			t.Fatalf("err=%v updated=%v, muốn ErrLivestreamInvalidInput và không ghi", err, repo.updated)
		}
	})

	t.Run("kết thúc trước bắt đầu hiện có là 400", func(t *testing.T) {
		repo := newSession(model.LivestreamStatusScheduled)
		_, err := newR3LivestreamSvc(repo).Update(ctx, host, false, repo.session.ID, dto.UpdateLivestreamDTO{ScheduledEndAt: str(rfc(repo.session.ScheduledAt.Add(-time.Minute)))})
		if !errors.Is(err, ErrLivestreamInvalidInput) {
			t.Fatalf("err=%v, muốn ErrLivestreamInvalidInput", err)
		}
	})

	t.Run("buổi đang live không dời giờ được (409) nhưng sửa phòng/tiêu đề được", func(t *testing.T) {
		repo := newSession(model.LivestreamStatusLive)
		svc := newR3LivestreamSvc(repo)
		if _, err := svc.Update(ctx, host, false, repo.session.ID, dto.UpdateLivestreamDTO{ScheduledAt: str(rfc(newStart))}); !errors.Is(err, ErrLivestreamStateConflict) {
			t.Fatalf("dời giờ buổi live: err=%v, muốn ErrLivestreamStateConflict", err)
		}
		title := "Tiêu đề mới"
		if _, err := svc.Update(ctx, host, false, repo.session.ID, dto.UpdateLivestreamDTO{Title: &title, Location: str("Phong C3")}); err != nil {
			t.Fatalf("sửa tiêu đề/phòng buổi live: %v", err)
		}
	})
}

func TestR3_LivestreamEnd_ChuaLive_La409(t *testing.T) {
	host := uuid.New()
	for _, status := range []model.LivestreamSessionStatus{model.LivestreamStatusScheduled, model.LivestreamStatusEnded} {
		repo := &r3LivestreamRepo{session: &model.LivestreamSession{
			BaseModel: model.BaseModel{ID: uuid.New()}, HostID: host, ClassID: uuid.New(), Status: status,
		}}
		_, err := newR3LivestreamSvc(repo).End(context.Background(), host, false, repo.session.ID)
		if !errors.Is(err, ErrLivestreamStateConflict) {
			t.Errorf("end buổi %s: err=%v, muốn ErrLivestreamStateConflict (không phải lỗi 500 chung)", status, err)
		}
	}
}

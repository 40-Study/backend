package handler

// Test cho F1 (review 260917), tang handler: danh tinh truyen xuong service phai la nguoi goi
// (tu access token), va service.ErrUploadNotOwned phai thanh 403 — khong phai 500 lan voi loi ha
// tang, vi 500 giong het phan hoi cua chinh chu khi MinIO loi (chinh la dau hieu IDOR luc tai hien).
//
// mountWithCaller/doJSON dung lai tu livestream_authz_handler_test.go (cung package).

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/service"
)

// stubOwnerUploadService: moi method khong cai dat thuoc interface nhung (nil) se panic neu handler
// goi nham.
type stubOwnerUploadService struct {
	service.VideoUploadServiceInterface
	err     error
	gotUser uuid.UUID
}

func (s *stubOwnerUploadService) CompleteVideoUpload(ctx context.Context, req *dto.CompleteVideoUploadRequest, userID uuid.UUID) (*dto.CompleteVideoUploadResponse, error) {
	s.gotUser = userID
	if s.err != nil {
		return nil, s.err
	}
	return &dto.CompleteVideoUploadResponse{Success: true}, nil
}

func (s *stubOwnerUploadService) GetUploadStatus(ctx context.Context, uploadID, userID uuid.UUID) (*dto.GetUploadStatusResponse, error) {
	s.gotUser = userID
	if s.err != nil {
		return nil, s.err
	}
	return &dto.GetUploadStatusResponse{}, nil
}

func (s *stubOwnerUploadService) ReprocessVideo(ctx context.Context, uploadID, userID uuid.UUID) error {
	s.gotUser = userID
	return s.err
}

func TestUploadComplete_NguoiLa_403VaTruyenDungDanhTinh(t *testing.T) {
	caller := uuid.New()
	svc := &stubOwnerUploadService{err: service.ErrUploadNotOwned}
	app := mountWithCaller("POST", "/complete", caller, NewVideoUploadHandler(svc).CompleteVideoUpload)

	code := doJSON(t, app, "POST", "/complete", `{"upload_id":"`+uuid.NewString()+`"}`)
	if code != 403 {
		t.Fatalf("nguoi khong so huu phai nhan 403, nhan %d", code)
	}
	if svc.gotUser != caller {
		t.Fatalf("handler phai truyen user_id cua nguoi goi xuong service, truyen %s", svc.gotUser)
	}
}

func TestUploadComplete_ChinhChu_200(t *testing.T) {
	caller := uuid.New()
	svc := &stubOwnerUploadService{}
	app := mountWithCaller("POST", "/complete", caller, NewVideoUploadHandler(svc).CompleteVideoUpload)
	if code := doJSON(t, app, "POST", "/complete", `{"upload_id":"`+uuid.NewString()+`"}`); code != 200 {
		t.Fatalf("chinh chu phai nhan 200, nhan %d", code)
	}
}

func TestUploadStatus_NguoiLa_403(t *testing.T) {
	svc := &stubOwnerUploadService{err: service.ErrUploadNotOwned}
	app := mountWithCaller("GET", "/:upload_id/status", uuid.New(), NewVideoUploadHandler(svc).GetUploadStatus)
	if code := doJSON(t, app, "GET", "/"+uuid.NewString()+"/status", ""); code != 403 {
		t.Fatalf("status cua upload nguoi khac phai la 403, nhan %d", code)
	}
}

func TestUploadReprocess_NguoiLa_403(t *testing.T) {
	caller := uuid.New()
	svc := &stubOwnerUploadService{err: service.ErrUploadNotOwned}
	app := mountWithCaller("POST", "/:upload_id/reprocess", caller, NewVideoUploadHandler(svc).ReprocessVideo)
	if code := doJSON(t, app, "POST", "/"+uuid.NewString()+"/reprocess", ""); code != 403 {
		t.Fatalf("reprocess upload nguoi khac phai la 403, nhan %d", code)
	}
	if svc.gotUser != caller {
		t.Fatalf("reprocess phai truyen user_id cua nguoi goi, truyen %s", svc.gotUser)
	}
}

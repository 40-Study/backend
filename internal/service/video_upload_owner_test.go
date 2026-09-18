package service

// Test cho F1 (review 260917, da tai hien tren server that): moi endpoint thao tac tren mot
// upload_id phai tu choi nguoi KHONG so huu bang ErrUploadNotOwned, TRUOC khi cham Redis, bang
// parts hay MinIO. Service o day duoc dung voi storage/redis/queue = nil: neu kiem chu so huu bi
// go bo, luong code se di tiep toi cac dependency nil va test do (panic hoac sai loi), khong the
// xanh gia.

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

// fakeOwnerUploadRepo chi cai dat GetUploadByID; moi method khac cua interface nhung vao la nil
// va se panic neu bi goi — tuc la bat ky truy cap nao ngoai viec doc ban ghi deu lam test do.
type fakeOwnerUploadRepo struct {
	repository.VideoUploadRepositoryInterface
	upload *model.VideoUpload
}

func (f *fakeOwnerUploadRepo) GetUploadByID(ctx context.Context, uploadID uuid.UUID) (*model.VideoUpload, error) {
	if f.upload == nil || f.upload.ID != uploadID {
		return nil, errors.New("record not found")
	}
	return f.upload, nil
}

func newOwnerTestService(owner uuid.UUID) (*VideoUploadService, uuid.UUID) {
	uploadID := uuid.New()
	up := &model.VideoUpload{UserID: owner, TotalChunks: 3}
	up.ID = uploadID
	return NewVideoUploadService(&fakeOwnerUploadRepo{upload: up}, nil, nil, nil, nil), uploadID
}

func TestVideoUpload_NguoiLa_BiChanOMoiEndpoint(t *testing.T) {
	owner, stranger := uuid.New(), uuid.New()
	svc, uploadID := newOwnerTestService(owner)
	ctx := context.Background()

	cases := map[string]func() error{
		"GetPresignedURLs": func() error {
			_, err := svc.GetPresignedURLs(ctx, &dto.GetPresignedURLsRequest{UploadID: uploadID, ChunkNumbers: []int{1}}, stranger)
			return err
		},
		"CompleteChunkUpload": func() error {
			_, err := svc.CompleteChunkUpload(ctx, &dto.CompleteChunkUploadRequest{UploadID: uploadID, ChunkNumber: 1, ETag: "e", Size: 1}, stranger)
			return err
		},
		"CompleteVideoUpload": func() error {
			_, err := svc.CompleteVideoUpload(ctx, &dto.CompleteVideoUploadRequest{UploadID: uploadID}, stranger)
			return err
		},
		"GetUploadStatus": func() error {
			_, err := svc.GetUploadStatus(ctx, uploadID, stranger)
			return err
		},
		"GetResumeInfo": func() error {
			_, err := svc.GetResumeInfo(ctx, uploadID, stranger)
			return err
		},
		"AbortUpload": func() error {
			_, err := svc.AbortUpload(ctx, &dto.AbortUploadRequest{}, uploadID, stranger)
			return err
		},
		"ReprocessVideo": func() error {
			return svc.ReprocessVideo(ctx, uploadID, stranger)
		},
	}
	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, ErrUploadNotOwned) {
				t.Fatalf("%s: nguoi la phai nhan ErrUploadNotOwned, nhan %v", name, err)
			}
		})
	}
}

func TestGetOwnedUpload_ChinhChu_DuocTraVe(t *testing.T) {
	owner := uuid.New()
	svc, uploadID := newOwnerTestService(owner)
	up, err := svc.getOwnedUpload(context.Background(), uploadID, owner)
	if err != nil {
		t.Fatalf("chinh chu khong duoc bi chan: %v", err)
	}
	if up.UserID != owner {
		t.Fatalf("tra sai ban ghi: user_id=%s", up.UserID)
	}
}

func TestGetOwnedUpload_KhongTonTai_KhongPhaiLoiQuyen(t *testing.T) {
	svc, _ := newOwnerTestService(uuid.New())
	_, err := svc.getOwnedUpload(context.Background(), uuid.New(), uuid.New())
	if err == nil || errors.Is(err, ErrUploadNotOwned) {
		t.Fatalf("upload khong ton tai phai la loi not-found, khong phai 403: %v", err)
	}
}

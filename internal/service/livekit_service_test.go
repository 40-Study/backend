package service

// R3-1 (issue #58 review vòng 3): bản vá R2-1 (CanSubscribe mặc định true trong
// buildUpdateParticipantRequest) không có test nào bảo vệ — mutation M-9 (xoá dòng
// "CanSubscribe: true") phục hồi nguyên vẹn lỗi CHẶN MERGE nặng nhất của PR (khoá/mở bảng trắng
// làm cả lớp mất khả năng nghe/nhìn) mà suite vẫn xanh. Pin lại bằng cách gọi thẳng hàm BUILD
// request (buildUpdateParticipantRequest, tách khỏi phần gọi mạng thật trong livekit_service.go)
// — không cần fake/mock gRPC client nào.

import (
	"testing"

	"study.com/v1/internal/dto"
)

func TestBuildUpdateParticipantRequest_ChiTruyenCanPublish_GiuCanSubscribeVaPublishData(t *testing.T) {
	req := dto.UpdateParticipantDTO{CanPublish: ptrBool(false)}

	got := buildUpdateParticipantRequest("room-1", "user-1", req)

	if got.Permission == nil {
		t.Fatal("Permission bi de nil du co truyen CanPublish")
	}
	if !got.Permission.CanSubscribe {
		t.Error("CanSubscribe = false, muon true (R2-1: mac dinh giu subscribe khi khong truyen tuong minh)")
	}
	if !got.Permission.CanPublishData {
		t.Error("CanPublishData = false, muon true (mac dinh khi khong truyen tuong minh)")
	}
	if got.Permission.CanPublish {
		t.Error("CanPublish phai la false dung nhu tham so da truyen")
	}
}

func TestBuildUpdateParticipantRequest_TruyenCanSubscribeFalse_DuocTonTrong(t *testing.T) {
	// Hoi quy: mac dinh true KHONG duoc de ghi de mot gia tri false CALLER da chu dich truyen —
	// hien tai khong co call site nao trong livestream_service.go can dieu nay, nhung dam bao
	// hop dong cua ham build dung nhu comment da ghi ("khong suy doan").
	req := dto.UpdateParticipantDTO{CanSubscribe: ptrBool(false)}

	got := buildUpdateParticipantRequest("room-1", "user-1", req)

	if got.Permission == nil {
		t.Fatal("Permission bi de nil du co truyen CanSubscribe")
	}
	if got.Permission.CanSubscribe {
		t.Error("CanSubscribe phai la false dung nhu tham so da truyen tuong minh, khong duoc mac dinh de ghi de")
	}
}

func TestBuildUpdateParticipantRequest_KhongTruyenGiCa_KhongDungPermission(t *testing.T) {
	// Hoi quy: khi khong truyen field nao (chi doi Metadata), Permission phai la nil — khong
	// duoc vo tinh gui mot Permission "mac dinh" thay the toan bo quyen hien tai cua participant.
	got := buildUpdateParticipantRequest("room-1", "user-1", dto.UpdateParticipantDTO{Metadata: "abc"})

	if got.Permission != nil {
		t.Errorf("Permission = %+v, muon nil khi khong truyen field quyen nao", got.Permission)
	}
	if got.Metadata != "abc" {
		t.Errorf("Metadata = %q, muon %q", got.Metadata, "abc")
	}
}

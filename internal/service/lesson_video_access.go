package service

import (
	"log"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/hlsauth"
)

// videoViewer mô tả người sẽ nhận URL video ký trong response nội dung bài học.
//
// userID: gắn vào chữ ký để truy vết (uuid.Nil với khách xem thử). original: true CHỈ với chủ khoá
// học / admin — những role duy nhất được phép lấy URL file video GỐC (trang quản lý cần xem lại
// file vừa upload khi HLS chưa xử lý xong). Học viên và khách KHÔNG BAO GIỜ nhận URL video gốc.
type videoViewer struct {
	userID   uuid.UUID
	original bool
	// withheld: đường gọi CHƯA có bước kiểm quyền xem (vd GetContentByID không nhận user) — không
	// cấp URL video nào, kể cả HLS. Chỉ nội dung video ngoài hệ thống được giữ nguyên.
	withheld bool
}

// guestVideoViewer: người xem thử công khai, không đăng nhập.
var guestVideoViewer = videoViewer{}

// withheldVideoViewer: không cấp URL video nội bộ.
var withheldVideoViewer = videoViewer{withheld: true}

// signSubtitleURL: bucket video là private nên URL phụ đề .vtt đã lưu (URL MinIO trực tiếp) không
// còn tải được. Thay bằng URL ký của /api/hls/object cho người xem đã qua kiểm quyền. URL không
// trỏ vào bucket video (vd URL ngoài) được giữ nguyên. Không ký được -> bỏ URL, không lộ URL thô.
func signSubtitleURL(stored *string, v videoViewer) *string {
	if stored == nil || *stored == "" {
		return stored
	}
	key, ok := hlsauth.ObjectKey(*stored)
	if !ok {
		return stored
	}
	if v.withheld {
		return nil
	}
	tok, err := hlsauth.SignResource(hlsauth.ScopeObject, key, v.userID, time.Now())
	if err != nil {
		log.Printf("[ERROR] Khong ky duoc URL phu de %s: %v", key, err)
		return nil
	}
	signed := hlsauth.ObjectURL(key, tok)
	return &signed
}

// applyVideoAccess điền các field video của một content: thay URL HLS/gốc đã lưu bằng URL KÝ.
//
// Chỉ gọi ở đường ĐÃ kiểm quyền (ghi danh / chủ khoá / admin / route xem thử đã kiểm published+
// preview). Đây là điểm DUY NHẤT sinh URL HLS trả về client — các mapper dùng chung để không có
// nhánh nào quên ký. URL video ngoài hệ thống (không có /hls/{uuid}, vd video mẫu của seed) giữ
// nguyên vì không phải tài nguyên của ta.
func applyVideoAccess(resp *dto.LessonContentResponseDTO, storedURL *string, v videoViewer) {
	resp.SubtitleURL = signSubtitleURL(resp.SubtitleURL, v)
	resp.VideoURL = storedURL
	if storedURL == nil || *storedURL == "" {
		return
	}
	uploadID, ok := hlsauth.ExtractUploadID(*storedURL)
	if !ok {
		return
	}

	// Không ký được (chưa Configure secret) là lỗi lập trình/cấu hình — InitResources đã chặn
	// khởi động. Không bao giờ rơi về URL không ký: xoá URL và log rõ để không âm thầm lộ video.
	resp.VideoURL = nil
	if v.withheld {
		return
	}
	now := time.Now()
	stream, err := hlsauth.Sign(hlsauth.ScopeStream, uploadID, v.userID, now)
	if err != nil {
		log.Printf("[ERROR] Khong ky duoc URL HLS cho upload %s: %v", uploadID, err)
		return
	}
	id := uploadID.String()
	hlsURL := hlsauth.MasterURL(uploadID, stream)
	resp.VideoUploadID = &id
	resp.VideoHLSURL = &hlsURL

	if v.original {
		orig, err := hlsauth.Sign(hlsauth.ScopeOriginal, uploadID, v.userID, now)
		if err != nil {
			log.Printf("[ERROR] Khong ky duoc URL video goc cho upload %s: %v", uploadID, err)
			return
		}
		origURL := hlsauth.OriginalURL(uploadID, orig)
		resp.VideoURL = &origURL
	}
}

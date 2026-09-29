// Package hlsauth ký và kiểm tra URL ngắn hạn cho /api/hls/*.
//
// Vì sao cần: token đăng nhập của hệ thống nằm trong header/body, KHÔNG nằm trong cookie, nên thẻ
// <video> và hls.js không tự gửi được auth. Trước đây /api/hls/* vì thế để công khai — ai biết
// upload_id là tải được cả video. Nay endpoint nội dung bài học (đã kiểm ghi danh / chủ khoá /
// admin, hoặc route xem thử công khai) trả về URL kèm chữ ký HMAC; handler HLS chỉ phục vụ khi
// chữ ký khớp và chưa hết hạn.
//
// Chữ ký phủ: scope + upload_id + thời điểm hết hạn + user_id (rỗng với khách). Scope tách quyền
// xem HLS (mọi người được cấp) khỏi quyền xem FILE GỐC (chỉ chủ khoá / admin).
package hlsauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	// ScopeStream: playlist + segment HLS — cấp cho học viên đã ghi danh, khách xem thử, chủ khoá, admin.
	ScopeStream = "hls"
	// ScopeOriginal: file video gốc (video.mp4) — CHỈ chủ khoá / admin.
	ScopeOriginal = "src"

	// TTL là thời hạn URL ký. Player gặp 403 giữa chừng thì xin lại URL mới một lần (web lo).
	TTL = 2 * time.Hour

	// MinSecretLength: HMAC-SHA256 với secret ngắn dễ bị dò; ép tối thiểu 32 ký tự.
	MinSecretLength = 32

	secretEnvName = "HLS_SIGNING_SECRET"
)

var (
	ErrNotConfigured = errors.New("hlsauth: chưa cấu hình secret ký URL HLS")
	ErrInvalid       = errors.New("chữ ký video không hợp lệ")
	ErrExpired       = errors.New("liên kết video đã hết hạn")
)

var (
	mu     sync.RWMutex
	secret []byte
)

// ValidateSecret là hàm thuần để LoadConfig fail-fast lúc khởi động — cùng kiểu validateJWTSecret.
// Cố ý KHÔNG có giá trị mặc định: thiếu secret mà chạy tiếp nghĩa là ký bằng chuỗi công khai.
func ValidateSecret(s, configFileHint string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return fmt.Errorf("%s là bắt buộc nhưng chưa được đặt — đặt trong %s hoặc biến môi trường (chuỗi ngẫu nhiên >= %d ký tự)", secretEnvName, configFileHint, MinSecretLength)
	}
	if strings.HasPrefix(strings.ToLower(s), "changeme") {
		return fmt.Errorf("%s vẫn là giá trị mẫu từ .env.example — đặt secret thật trong %s hoặc biến môi trường", secretEnvName, configFileHint)
	}
	if len(s) < MinSecretLength {
		return fmt.Errorf("%s quá ngắn (%d ký tự) — cần >= %d ký tự ngẫu nhiên", secretEnvName, len(s), MinSecretLength)
	}
	return nil
}

// Configure nạp secret cho toàn process. Trả lỗi (không panic) nếu secret không đạt ValidateSecret.
func Configure(s string) error {
	if err := ValidateSecret(s, "cấu hình"); err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	secret = []byte(strings.TrimSpace(s))
	return nil
}

func currentSecret() ([]byte, error) {
	mu.RLock()
	defer mu.RUnlock()
	if len(secret) == 0 {
		return nil, ErrNotConfigured
	}
	return secret, nil
}

// Token là bộ tham số nằm trong query của URL ký.
type Token struct {
	Exp int64     // unix giây
	UID uuid.UUID // uuid.Nil = khách / không gắn user
	Sig string    // hex(HMAC-SHA256)
}

// Query dựng chuỗi query "exp=..&uid=..&sig=..". Chỉ gồm ký tự an toàn (số, hex, uuid) nên không cần escape.
func (t Token) Query() string {
	q := "exp=" + strconv.FormatInt(t.Exp, 10)
	if t.UID != uuid.Nil {
		q += "&uid=" + t.UID.String()
	}
	return q + "&sig=" + t.Sig
}

func mac(sec []byte, scope, uploadID string, exp int64, uid uuid.UUID) string {
	h := hmac.New(sha256.New, sec)
	// Phân tách bằng "\n": không trường nào chứa xuống dòng nên không thể ghép nhập nhằng.
	fmt.Fprintf(h, "v1\n%s\n%s\n%d\n%s", scope, uploadID, exp, uid.String())
	return hex.EncodeToString(h.Sum(nil))
}

// Sign cấp token cho (scope, upload). Lỗi khi chưa Configure — không bao giờ trả URL không ký.
func Sign(scope string, uploadID uuid.UUID, uid uuid.UUID, now time.Time) (Token, error) {
	sec, err := currentSecret()
	if err != nil {
		return Token{}, err
	}
	exp := now.Add(TTL).Unix()
	return Token{Exp: exp, UID: uid, Sig: mac(sec, scope, uploadID.String(), exp, uid)}, nil
}

// Verify kiểm chữ ký từ các tham số query thô (exp, uid, sig). Trả Token đã kiểm để handler dựng
// lại query cho playlist con từ giá trị ĐÃ PARSE, không bao giờ nhúng nguyên chuỗi client gửi.
func Verify(scope string, uploadID uuid.UUID, expRaw, uidRaw, sigRaw string, now time.Time) (Token, error) {
	sec, err := currentSecret()
	if err != nil {
		return Token{}, err
	}
	exp, err := strconv.ParseInt(expRaw, 10, 64)
	if err != nil || sigRaw == "" {
		return Token{}, ErrInvalid
	}
	uid := uuid.Nil
	if uidRaw != "" {
		if uid, err = uuid.Parse(uidRaw); err != nil {
			return Token{}, ErrInvalid
		}
	}
	want := mac(sec, scope, uploadID.String(), exp, uid)
	// So sánh hằng thời gian; sai chữ ký được báo TRƯỚC hết hạn để không lộ token nào từng hợp lệ.
	if !hmac.Equal([]byte(want), []byte(sigRaw)) {
		return Token{}, ErrInvalid
	}
	if now.Unix() > exp {
		return Token{}, ErrExpired
	}
	return Token{Exp: exp, UID: uid, Sig: want}, nil
}

// URLs dựng đường dẫn tương đối cho một upload.
func MasterURL(uploadID uuid.UUID, t Token) string {
	return "/api/hls/" + uploadID.String() + "/master.m3u8?" + t.Query()
}

func OriginalURL(uploadID uuid.UUID, t Token) string {
	return "/api/hls/" + uploadID.String() + "/video.mp4?" + t.Query()
}

// ExtractUploadID lấy upload id từ một URL video đã lưu (dạng .../hls/{uuid}/...). Trả false nếu
// URL không phải video nội bộ (vd video mẫu ngoài hệ thống) — khi đó không ký gì cả.
func ExtractUploadID(raw string) (uuid.UUID, bool) {
	i := strings.Index(raw, "/hls/")
	if i < 0 {
		return uuid.Nil, false
	}
	rest := raw[i+len("/hls/"):]
	if len(rest) < 36 {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(rest[:36])
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

// RewritePlaylist gắn query ký vào MỌI URI tương đối trong playlist m3u8 để hls.js — vốn không
// chuyển query của playlist xuống URL con — gọi playlist/segment kế tiếp kèm chữ ký. URI vẫn
// tương đối nên chạy đúng dù đi qua proxy Next hay gọi thẳng backend.
// Chỉ đổi dòng URI thường (không bắt đầu bằng '#'). Playlist do ffmpeg sinh ra chỉ có URI tương đối
// dạng này (không EXT-X-KEY/EXT-X-MAP); URI tuyệt đối được giữ nguyên vì không phải của ta.
func RewritePlaylist(body []byte, t Token) []byte {
	q := t.Query()
	lines := strings.Split(string(body), "\n")
	for i, line := range lines {
		trimmed := strings.TrimRight(line, "\r")
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.Contains(trimmed, "://") || strings.HasPrefix(trimmed, "/") {
			continue
		}
		sep := "?"
		if strings.Contains(trimmed, "?") {
			sep = "&"
		}
		lines[i] = trimmed + sep + q
	}
	return []byte(strings.Join(lines, "\n"))
}

// QueryOf trích chuỗi query (không có '?') từ một URL ký — dùng cho test.
func QueryOf(rawURL string) url.Values {
	u, err := url.Parse(rawURL)
	if err != nil {
		return url.Values{}
	}
	return u.Query()
}

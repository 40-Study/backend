package service

import "study.com/v1/internal/hlsauth"

// Mọi mapper nội dung bài học nay ký URL HLS nên test cần secret. Giá trị này chỉ dùng trong test;
// app thật lấy từ HLS_SIGNING_SECRET và fail-fast lúc khởi động khi thiếu (app.InitResources).
const testHLSSecret = "test-only-hls-signing-secret-0123456789abcdef"

func init() {
	if err := hlsauth.Configure(testHLSSecret); err != nil {
		panic(err)
	}
}

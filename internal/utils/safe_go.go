package utils

import "log"

// SafeGo chạy fn trong một goroutine mới, có recover() để panic bên trong fn không làm sập cả
// process (Go không tự recover hộ goroutine con — panic trong 1 goroutine không recover sẽ crash
// toàn bộ chương trình, kể cả các request khác đang chạy).
//
// M-05 (audit 260909): các goroutine gửi email OTP / xử lý submission bất đồng bộ trước đây
// không có recover() nào — một lỗi runtime (vd nil pointer khi cấu hình SMTP thiếu) là sập server.
func SafeGo(fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[ERROR] recovered panic in background goroutine: %v", r)
			}
		}()
		fn()
	}()
}

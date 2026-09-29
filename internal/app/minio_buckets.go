package app

import (
	"context"
	"errors"
	"fmt"
)

// bucketEnsurer là phần của storage.MinioClient mà bước khởi động dùng — tách interface để test được
// mà không cần MinIO thật.
type bucketEnsurer interface {
	EnsureBuckets(ctx context.Context) error
}

// ensureVideoBucketsPrivate chạy EnsureBuckets và biến MỌI lỗi thành lỗi khởi động (review S1 F1).
//
// Bucket video "private" chỉ có nghĩa khi bước gỡ policy public-read THÀNH CÔNG: nếu nó lỗi (cấu hình
// MINIO_BUCKET_NAME trùng bucket ảnh, credential thiếu quyền, MinIO không với tới được lúc boot) mà
// backend vẫn chạy thì video tải được ẩn danh qua CDN và mọi chữ ký URL vô nghĩa — chỉ có một dòng
// warning không ai đọc. Nên caller phải dừng tiến trình (log.Fatalf), không được chạy tiếp.
func ensureVideoBucketsPrivate(ctx context.Context, m bucketEnsurer) error {
	if m == nil {
		return errors.New("MinIO client is not available: cannot make the video bucket private")
	}
	if err := m.EnsureBuckets(ctx); err != nil {
		return fmt.Errorf("MinIO: cannot make the video bucket private (need MINIO_BUCKET_NAME different from MINIO_BUCKET_IMAGES, "+
			"MinIO reachable, and credentials allowed s3:PutBucketPolicy and s3:DeleteBucketPolicy): %w", err)
	}
	return nil
}

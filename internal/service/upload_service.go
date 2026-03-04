// Package service chứa các business logic của ứng dụng
// File này xử lý việc upload file (hình ảnh và video) lên MinIO object storage
package service

import (
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"study.com/v1/internal/config"
)

// UploadResult chứa thông tin kết quả sau khi upload file thành công
// Struct này được trả về cho client sau khi upload hoàn tất
type UploadResult struct {
	URL      string `json:"url"`      // URL đầy đủ để truy cập file (http://host:port/bucket/object)
	Type     string `json:"type"`     // Loại file: "image" hoặc "video" - dùng để client biết cách xử lý
	Bucket   string `json:"bucket"`   // Tên bucket trên MinIO nơi file được lưu
	Filename string `json:"filename"` // Tên file gốc từ client upload lên
	Size     int64  `json:"size"`     // Kích thước file (bytes) - để hiển thị hoặc validate
	Object   string `json:"object"`   // ObjectName đầy đủ trên MinIO (folder/2006/01/02/uuid.ext) - dùng để debug hoặc xóa file sau này
}

// Định nghĩa các error constants để sử dụng xuyên suốt service
// Dùng var thay vì const vì errors.New trả về pointer, không thể dùng const
// Mục đích: Giúp code dễ đọc, dễ test, và có thể so sánh error bằng ==
var (
	ErrFileRequired       = errors.New("file is required")      // File không được gửi lên hoặc nil
	ErrFileTypeNotAllowed = errors.New("file type not allowed") // MIME type không nằm trong danh sách cho phép
	ErrCannotOpenFile     = errors.New("cannot open file")      // Không thể mở file để đọc nội dung (lỗi I/O)
	ErrUploadFailed       = errors.New("upload failed")         // Lỗi khi upload lên MinIO (network, permission, bucket không tồn tại, etc.)
)

// allowedImageTypes là whitelist các MIME type hình ảnh được phép upload
// Dùng map[string]bool thay vì slice để tra cứu O(1) thay vì O(n)
// Lý do cần whitelist: Bảo mật - tránh upload file độc hại, shell script giả mạo image
var allowedImageTypes = map[string]bool{
	"image/jpeg":    true, // JPEG - format phổ biến nhất cho ảnh
	"image/jpg":     true, // Alias của JPEG (một số browser gửi jpg thay vì jpeg)
	"image/png":     true, // PNG - hỗ trợ transparency, phổ biến cho logo, icon
	"image/gif":     true, // GIF - hỗ trợ animation
	"image/webp":    true, // WebP - format mới, nén tốt hơn JPEG/PNG
	"image/svg+xml": true, // SVG - vector graphics, scale không mất chất lượng
}

// allowedVideoTypes là whitelist các MIME type video được phép upload
// Tương tự image, dùng map để tra cứu nhanh và đảm bảo bảo mật
var allowedVideoTypes = map[string]bool{
	"video/mp4":        true, // MP4 - format phổ biến nhất, hỗ trợ rộng rãi
	"video/mpeg":       true, // MPEG - format cũ nhưng vẫn được dùng
	"video/quicktime":  true, // MOV - format của Apple QuickTime
	"video/x-msvideo":  true, // AVI - format của Microsoft
	"video/webm":       true, // WebM - format mở, tối ưu cho web
	"video/x-matroska": true, // MKV - container linh hoạt, chất lượng cao
}

// UploadServiceInterface định nghĩa contract của upload service
// Lý do dùng interface: Dễ mock khi test, dễ swap implementation, follow SOLID principles
type UploadServiceInterface interface {
	// Upload tự động detect loại file (image/video) và upload vào bucket tương ứng
	Upload(ctx context.Context, file *multipart.FileHeader, folder string) (*UploadResult, error)
	// UploadImage chỉ cho phép upload image, reject nếu không phải image
	UploadImage(ctx context.Context, file *multipart.FileHeader, folder string) (*UploadResult, error)
	// UploadVideo chỉ cho phép upload video, reject nếu không phải video
	UploadVideo(ctx context.Context, file *multipart.FileHeader, folder string) (*UploadResult, error)
	// DeleteByURL xóa file trên MinIO dựa vào URL đã được tạo trước đó
	DeleteByURL(ctx context.Context, fileURL string) error
}

// UploadService implement UploadServiceInterface
// Chứa dependencies cần thiết để upload file
type UploadService struct {
	objStorage *minio.Client  // Client để tương tác với MinIO storage
	cfg        *config.Config // Config chứa thông tin MinIO (host, port, bucket names, SSL)
}

// NewUploadService là constructor function tạo instance của UploadService
// Dùng dependency injection: nhận sẵn objStorage và cfg từ bên ngoài
// Lý do: Dễ test (mock dependencies), loose coupling, follow DI pattern
func NewUploadService(objStorage *minio.Client, cfg *config.Config) *UploadService {
	return &UploadService{objStorage: objStorage, cfg: cfg}
}

// constructFileURL tạo URL đầy đủ để truy cập file trên MinIO
// Format: protocol://host:port/bucket/objectName
// Lý do cần method này: MinIO client không tự động tạo URL, phải tự build
func (s *UploadService) constructFileURL(bucket, objectName string) string {
	// Mặc định dùng HTTP, chuyển sang HTTPS nếu config bật SSL
	// Lý do check SSL: Production thường dùng HTTPS, development dùng HTTP
	protocol := "http"
	if s.cfg.MinioUseSSL {
		protocol = "https"
	}
	// Tạo URL theo format: protocol://host:port/bucket/objectName
	// VD: http://localhost:9000/images/products/2024/12/20/abc-123.jpg
	return fmt.Sprintf("%s://%s:%s/%s/%s",
		protocol, s.cfg.MinioHost, s.cfg.MinioPort, bucket, objectName)
}

// validateAndGetBucket validate MIME type và trả về loại file + bucket tương ứng
// Mục đích: Tách biệt images và videos vào 2 bucket khác nhau để dễ quản lý
// Return: fileType ("image"|"video"), bucket name, error nếu không hợp lệ
func (s *UploadService) validateAndGetBucket(contentType string) (fileType string, bucket string, err error) {
	// Chuẩn hóa content type: lowercase + trim space
	// Lý do: "Image/JPEG" vs "image/jpeg" vs " image/jpeg " đều là 1
	ct := strings.ToLower(strings.TrimSpace(contentType))

	// Check xem có phải image không, nếu đúng trả về bucket images
	if allowedImageTypes[ct] {
		return "image", s.cfg.MinioBucketImages, nil
	}
	// Check xem có phải video không, nếu đúng trả về bucket videos
	if allowedVideoTypes[ct] {
		return "video", s.cfg.MinioBucketVideos, nil
	}
	// Nếu không phải image cũng không phải video -> reject
	return "", "", ErrFileTypeNotAllowed
}

// uploadToMinio thực hiện việc upload file lên MinIO storage
// Đây là helper method chứa logic upload thực sự, được gọi bởi các method public
// Return: objectName (path trên MinIO), URL (để client truy cập), error nếu có
func (s *UploadService) uploadToMinio(ctx context.Context, file *multipart.FileHeader, bucket, folder string) (objectName string, url string, err error) {
	// Mở file để đọc content
	// file.Open() trả về multipart.File (implement io.Reader)
	src, err := file.Open()
	if err != nil {
		return "", "", fmt.Errorf("cannot open file: %w", err)
	}
	// defer Close() đảm bảo file được đóng sau khi hàm return, tránh memory leak
	defer src.Close()

	// Lấy extension từ tên file gốc (.jpg, .png, .mp4, etc.)
	ext := filepath.Ext(file.Filename)
	// Tạo objectName theo pattern: folder/YYYY/MM/DD/uuid.ext
	// VD: products/2024/12/20/abc-123-def-456.jpg
	// Lý do: - folder để phân loại (products, avatars, etc.)
	//        - YYYY/MM/DD để dễ quản lý theo thời gian, tránh quá nhiều file 1 folder
	//        - uuid để tên file unique, tránh trùng lặp
	//        - giữ extension gốc để browser/client biết file type
	objectName = fmt.Sprintf("%s/%s/%s%s",
		folder,
		time.Now().Format("2006/01/02"), // Go time format: 2006=year, 01=month, 02=day
		uuid.NewString(),                // UUID v4 random
		ext,
	)

	// Lấy và chuẩn hóa content type từ header của file
	contentType := strings.ToLower(strings.TrimSpace(file.Header.Get("Content-Type")))

	// Upload file lên MinIO bằng PutObject API
	// ctx: để handle timeout/cancellation
	// bucket: nơi lưu file (images hoặc videos)
	// objectName: path/tên file trên MinIO
	// src: io.Reader chứa data của file
	// file.Size: kích thước file (bytes) - MinIO cần để validate
	// PutObjectOptions: metadata, trong đó ContentType giúp browser hiển thị đúng
	_, err = s.objStorage.PutObject(
		ctx,
		bucket,
		objectName,
		src,
		file.Size,
		minio.PutObjectOptions{ContentType: contentType},
	)
	if err != nil {
		// Lỗi có thể do: network, bucket không tồn tại, permission, disk full, etc.
		return "", "", fmt.Errorf("minio upload failed (bucket=%s, object=%s): %w", bucket, objectName, err)
	}

	// Upload thành công, tạo URL và return
	return objectName, s.constructFileURL(bucket, objectName), nil
}

// Upload là method chính để upload file, tự động detect image hoặc video
// Sử dụng khi không quan tâm file là image hay video, chỉ cần upload được
// Params: ctx (timeout control), file (từ multipart form), folder (phân loại: products, avatars, etc.)
func (s *UploadService) Upload(ctx context.Context, file *multipart.FileHeader, folder string) (*UploadResult, error) {
	// Validate file không được nil
	// Trường hợp nil: client không gửi file hoặc form field name sai
	if file == nil {
		return nil, ErrFileRequired
	}

	// Validate MIME type và xác định bucket phù hợp (images hoặc videos)
	// Nếu MIME type không hợp lệ, trả về ErrFileTypeNotAllowed
	fileType, bucket, err := s.validateAndGetBucket(file.Header.Get("Content-Type"))
	if err != nil {
		return nil, err
	}

	// Thực hiện upload file lên MinIO
	// Nhận về objectName (path trên MinIO) và URL (để truy cập)
	objectName, url, err := s.uploadToMinio(ctx, file, bucket, folder)
	if err != nil {
		return nil, err
	}

	// Upload thành công, tạo và trả về UploadResult chứa đầy đủ thông tin
	// Client sẽ dùng URL để hiển thị ảnh/video, hoặc lưu vào database
	return &UploadResult{
		URL:      url,           // URL để truy cập file
		Type:     fileType,      // "image" hoặc "video"
		Bucket:   bucket,        // Bucket name trên MinIO
		Filename: file.Filename, // Tên file gốc từ client
		Size:     file.Size,     // Kích thước file (bytes)
		Object:   objectName,    // Path đầy đủ trên MinIO (để debug hoặc delete)
	}, nil
}

// UploadImage chỉ cho phép upload image, reject nếu không phải image
// Khác với Upload(): method này strict hơn, chỉ accept image
// Use case: Khi endpoint chỉ cho phép upload avatar, product image, etc.
func (s *UploadService) UploadImage(ctx context.Context, file *multipart.FileHeader, folder string) (*UploadResult, error) {
	// Validate file không được nil
	if file == nil {
		return nil, ErrFileRequired
	}
	// Lấy và chuẩn hóa content type
	ct := strings.ToLower(strings.TrimSpace(file.Header.Get("Content-Type")))
	// Check strict: chỉ chấp nhận image, reject mọi thứ khác (kể cả video)
	// Lý do cần check riêng: Một số endpoint chỉ cho phép image (avatar, logo, etc.)
	if !allowedImageTypes[ct] {
		return nil, ErrFileTypeNotAllowed
	}

	// Upload vào bucket images (không cần validate nữa vì đã check ở trên)
	objectName, url, err := s.uploadToMinio(ctx, file, s.cfg.MinioBucketImages, folder)
	if err != nil {
		return nil, err
	}
	// Tạo result với Type cố định là "image"
	return &UploadResult{
		URL:      url,
		Type:     "image", // Hardcode vì method này chỉ upload image
		Bucket:   s.cfg.MinioBucketImages,
		Filename: file.Filename,
		Size:     file.Size,
		Object:   objectName,
	}, nil
}

// UploadVideo chỉ cho phép upload video, reject nếu không phải video
// Tương tự UploadImage nhưng dành cho video
// Use case: Khi endpoint chỉ cho phép upload video (product demo, tutorials, etc.)
func (s *UploadService) UploadVideo(ctx context.Context, file *multipart.FileHeader, folder string) (*UploadResult, error) {
	// Validate file không được nil
	if file == nil {
		return nil, ErrFileRequired
	}
	// Lấy và chuẩn hóa content type
	ct := strings.ToLower(strings.TrimSpace(file.Header.Get("Content-Type")))
	// Check strict: chỉ chấp nhận video, reject mọi thứ khác (kể cả image)
	// Lý do: Endpoint video có thể cần xử lý riêng (transcoding, thumbnail generation, etc.)
	if !allowedVideoTypes[ct] {
		return nil, ErrFileTypeNotAllowed
	}

	// Upload vào bucket videos (không cần validate nữa vì đã check ở trên)
	objectName, url, err := s.uploadToMinio(ctx, file, s.cfg.MinioBucketVideos, folder)
	if err != nil {
		return nil, err
	}
	// Tạo result với Type cố định là "video"
	return &UploadResult{
		URL:      url,
		Type:     "video", // Hardcode vì method này chỉ upload video
		Bucket:   s.cfg.MinioBucketVideos,
		Filename: file.Filename,
		Size:     file.Size,
		Object:   objectName,
	}, nil
}

// DeleteByURL xóa file trên MinIO dựa vào URL đã được tạo trước đó
// Use case: Xóa ảnh cũ khi user upload ảnh mới, xóa product bị xóa, etc.
// URL format: protocol://host:port/bucket/folder/YYYY/MM/DD/uuid.ext
func (s *UploadService) DeleteByURL(ctx context.Context, fileURL string) error {
	// Parse URL để lấy bucket và objectName
	// VD: http://localhost:9000/images/products/2024/12/20/abc.jpg
	// parts = ["http:", "", "localhost:9000", "images", "products", "2024", "12", "20", "abc.jpg"]
	parts := strings.Split(fileURL, "/")
	// Validate URL phải có ít nhất 5 parts: protocol, empty, host:port, bucket, object
	// Nếu ít hơn 5 parts -> URL không đúng format
	if len(parts) < 5 {
		return fmt.Errorf("invalid URL format")
	}
	// parts[3] là bucket name ("images" hoặc "videos")
	bucket := parts[3]
	// parts[4:] là các phần của objectName ("products", "2024", "12", "20", "abc.jpg")
	// Join lại với "/" để được objectName đầy đủ: "products/2024/12/20/abc.jpg"
	objectName := strings.Join(parts[4:], "/")
	// Gọi MinIO API để xóa object
	// RemoveObject sẽ xóa file vĩnh viễn, không có recycle bin
	return s.objStorage.RemoveObject(ctx, bucket, objectName, minio.RemoveObjectOptions{})
}

// Package storage cung cấp các chức năng tương tác với MinIO object storage
package storage

import (
	"context"       // Quản lý context cho các request, timeout và cancellation
	"fmt"           // Format string và xử lý lỗi
	"io"            // Interface cho các thao tác input/output
	"log"           // Logging
	"net/url"       // URL parsing and encoding
	"os"            // Thao tác với file system
	"path/filepath" // Xử lý đường dẫn file
	"strings"       // String manipulation cho ValidateContentType
	"sync"          // Đồng bộ goroutines cho concurrent upload
	"time"          // Xử lý thời gian cho presigned URL expiry

	"github.com/minio/minio-go/v7"                 // SDK chính của MinIO
	"github.com/minio/minio-go/v7/pkg/credentials" // Xử lý credentials cho MinIO authentication
	"study.com/v1/internal/config"                 // Cấu hình ứng dụng (host, port, credentials, bucket name, ...)
)

// MinioClientInterface định nghĩa interface cho các thao tác với MinIO
// Interface này cho phép mock và test dễ dàng, đồng thời hỗ trợ dependency injection
type MinioClientInterface interface {
	// === MULTIPART UPLOAD OPERATIONS ===
	// Các thao tác upload file lớn theo từng phần để tránh timeout và tiết kiệm băng thông

	// InitMultipartUpload khởi tạo một phiên upload multipart mới
	// Dùng để bắt đầu upload file lớn, trả về uploadID để track quá trình upload
	InitMultipartUpload(ctx context.Context, bucket, objectKey, contentType string) (uploadID string, err error)

	// GetPresignedUploadURL tạo URL có chữ ký số để client upload trực tiếp lên MinIO
	// Client sẽ dùng URL này để upload từng part mà không cần gửi qua backend server
	GetPresignedUploadURL(ctx context.Context, bucket, objectKey, uploadID string, partNumber int) (presignedURL string, err error)

	// GetMultiplePresignedUploadURLs tạo nhiều presigned URLs cùng lúc cho tất cả parts
	// Dùng khi biết trước số parts, tối ưu hóa bằng cách presign 1 lần với expiry 30 phút
	GetMultiplePresignedUploadURLs(ctx context.Context, bucket, objectKey, uploadID string, totalParts int) ([]string, error)

	// RefreshPresignedUploadURLs làm mới presigned URLs khi sắp hết hạn hoặc timeout
	// Dùng để tiếp tục upload sau khi URLs cũ hết hạn
	RefreshPresignedUploadURLs(ctx context.Context, bucket, objectKey, uploadID string, partNumbers []int) (map[int]string, error)

	// CompleteMultipartUpload hoàn tất quá trình upload multipart
	// Ghép tất cả các part đã upload thành 1 file hoàn chỉnh trên MinIO
	CompleteMultipartUpload(ctx context.Context, bucket, objectKey, uploadID string, parts []CompletePart) (etag string, err error)

	// AbortMultipartUpload hủy bỏ quá trình upload multipart
	// Xóa tất cả các part đã upload để giải phóng storage
	AbortMultipartUpload(ctx context.Context, bucket, objectKey, uploadID string) error

	// === OBJECT OPERATIONS ===
	// Các thao tác cơ bản với object trong MinIO

	// StatObject lấy metadata của object (size, last modified, content type, ...)
	// Dùng để kiểm tra object có tồn tại và lấy thông tin mà không cần download
	StatObject(ctx context.Context, bucket, objectKey string) (minio.ObjectInfo, error)

	// GetObject tải object từ MinIO về dạng stream
	// Trả về ReadCloser để đọc nội dung object, tiết kiệm memory cho file lớn
	GetObject(ctx context.Context, bucket, objectKey string) (io.ReadCloser, error)

	// PutObject upload object lên MinIO từ stream
	// Dùng cho upload trực tiếp từ memory hoặc stream data
	PutObject(ctx context.Context, bucket, objectKey string, reader io.Reader, size int64, contentType string) error

	// PutObjectFromFile upload file từ local filesystem lên MinIO
	// Đơn giản hóa việc upload file có sẵn trên server
	PutObjectFromFile(ctx context.Context, bucket, objectKey, filePath, contentType string) error

	// GetPresignedDownloadURL tạo URL có chữ ký số để download trực tiếp từ MinIO
	// Client dùng URL này để download mà không cần qua backend, giảm tải server
	GetPresignedDownloadURL(ctx context.Context, bucket, objectKey string, expires time.Duration) (string, error)

	// GetDefaultBucket trả về bucket mặc định từ config
	GetDefaultBucket() string

	// GeneratePresignedURL tạo presigned URL với method tùy chỉnh (GET/PUT)
	GeneratePresignedURL(ctx context.Context, bucket, objectKey string, expires time.Duration, method string) (string, error)

	// ValidateContentType kiểm tra content type có hợp lệ cho video upload
	ValidateContentType(contentType string) bool

	// GetPresignedDownloadURLSimple helper với signature đơn giản, dùng default bucket
	GetPresignedDownloadURLSimple(objectKey string, expirySeconds int) (string, error)

	// ListObjectsWithPrefix liệt kê objects theo prefix, dùng default bucket
	ListObjectsWithPrefix(prefix string) ([]minio.ObjectInfo, error)

	// UploadHLSDirectory upload toàn bộ thư mục HLS lên MinIO (concurrent, xóa local sau upload)
	UploadHLSDirectory(ctx context.Context, localDir, videoID, bucket string) error
}

// CompletePart chứa thông tin của 1 part đã upload thành công
// Struct này dùng để xác nhận và hoàn tất multipart upload
type CompletePart struct {
	PartNumber int    // Số thứ tự của part (bắt đầu từ 1)
	ETag       string // ETag (hash) của part để verify tính toàn vẹn dữ liệu
}

// MinioClient là wrapper cho minio-go SDK
// Cung cấp các phương thức tiện lợi và tối ưu hóa việc tương tác với MinIO
type MinioClient struct {
	client *minio.Client  // Client chuẩn của MinIO SDK cho các thao tác thông thường
	core   *minio.Core    // Core client cho các thao tác low-level như multipart upload
	cfg    *config.Config // Configuration của ứng dụng (host, port, credentials, ...)
}

// NewMinioClient khởi tạo một MinioClient instance mới
// Factory function này tạo và cấu hình cả client thông thường và core client
func NewMinioClient(cfg *config.Config) (*MinioClient, error) {
	// Khởi tạo minio client chuẩn cho các thao tác CRUD object thông thường
	client, err := minio.New(
		cfg.MinioHost+":"+cfg.MinioPort, // Endpoint của MinIO server (host:port)
		&minio.Options{
			// Tạo credentials tĩnh với access key và secret key từ config
			Creds: credentials.NewStaticV4(cfg.MinioAccessKey, cfg.MinioSecretKey, ""),
			// Secure = true nghĩa là dùng HTTPS, false nghĩa là dùng HTTP
			Secure: cfg.MinioUseSSL,
		})
	// Nếu không thể kết nối hoặc credentials sai, trả về lỗi
	if err != nil {
		return nil, fmt.Errorf("failed to create minio client: %w", err)
	}

	// Khởi tạo core client cho các thao tác low-level như multipart upload
	// Core client cung cấp API chi tiết hơn để kiểm soát từng bước upload
	core, err := minio.NewCore(
		cfg.MinioHost+":"+cfg.MinioPort, // Endpoint giống như client thông thường
		&minio.Options{
			// Dùng cùng credentials với client thông thường
			Creds: credentials.NewStaticV4(cfg.MinioAccessKey, cfg.MinioSecretKey, ""),
			// Dùng cùng cấu hình SSL với client thông thường
			Secure: cfg.MinioUseSSL,
		})
	// Nếu không thể tạo core client, trả về lỗi
	if err != nil {
		return nil, fmt.Errorf("failed to create minio core client: %w", err)
	}

	// Trả về MinioClient đã được khởi tạo với cả 2 client và config
	return &MinioClient{
		client: client, // Client thông thường
		core:   core,   // Core client cho multipart
		cfg:    cfg,    // Config để dùng cho các method khác
	}, nil
}

// InitMultipartUpload bắt đầu một phiên upload multipart mới
// Dùng khi cần upload file lớn (>100MB) để tránh timeout và có thể resume nếu lỗi
func (m *MinioClient) InitMultipartUpload(ctx context.Context, bucket, objectKey, contentType string) (string, error) {
	// Gọi API của MinIO để tạo multipart upload session mới
	uploadID, err := m.core.NewMultipartUpload(
		ctx,       // Context để cancel operation nếu cần
		bucket,    // Tên bucket chứa object
		objectKey, // Đường dẫn/tên của object trên MinIO
		minio.PutObjectOptions{
			ContentType: contentType, // MIME type của file (video/mp4, image/jpeg, ...)
		})
	// Nếu có lỗi khi khởi tạo, trả về lỗi với context
	if err != nil {
		return "", fmt.Errorf("failed to initiate multipart upload: %w", err)
	}
	// Trả về uploadID để client dùng cho các lần upload part tiếp theo
	return uploadID, nil
}

// GetPresignedUploadURL tạo URL có chữ ký để client upload part trực tiếp lên MinIO
// Client sẽ PUT data trực tiếp lên URL này mà không cần qua backend server
func (m *MinioClient) GetPresignedUploadURL(ctx context.Context, bucket, objectKey, uploadID string, partNumber int) (string, error) {
	// For multipart upload, we need to use PresignedGetObject/PresignedPutObject with proper query params
	// The uploadId and partNumber MUST be part of the signature

	// Create a request with the proper query parameters BEFORE signing
	reqParams := make(url.Values)
	reqParams.Set("uploadId", uploadID)
	reqParams.Set("partNumber", fmt.Sprintf("%d", partNumber))

	// Generate presigned URL with query params included in signature
	// Expiry 30 phút để có đủ thời gian upload các parts lớn
	presignedURL, err := m.client.Presign(
		ctx,
		"PUT",          // HTTP method
		bucket,         // Bucket name
		objectKey,      // Object key
		10*time.Minute, // Expiration time - 30 phút cho part size 16-32MB
		reqParams,      // Query parameters to include in signature
	)

	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	// Return the URL as string
	return presignedURL.String(), nil
}

// CompleteMultipartUpload hoàn tất quá trình multipart upload
// MinIO sẽ ghép tất cả các part thành 1 file hoàn chỉnh theo đúng thứ tự
func (m *MinioClient) CompleteMultipartUpload(ctx context.Context, bucket, objectKey, uploadID string, parts []CompletePart) (string, error) {
	// Tạo slice để chứa thông tin các part theo format của MinIO SDK
	completeParts := make([]minio.CompletePart, len(parts))
	// Convert từ CompletePart của chúng ta sang minio.CompletePart
	for i, part := range parts {
		completeParts[i] = minio.CompletePart{
			PartNumber: part.PartNumber, // Số thứ tự part (1, 2, 3, ...)
			ETag:       part.ETag,       // ETag để verify data integrity
		}
	}

	// Gọi API CompleteMultipartUpload để hoàn tất
	// MinIO sẽ kiểm tra và ghép các part theo thứ tự PartNumber
	uploadInfo, err := m.core.CompleteMultipartUpload(
		ctx,                      // Context
		bucket,                   // Tên bucket
		objectKey,                // Tên object
		uploadID,                 // ID của upload session
		completeParts,            // Danh sách các part đã upload
		minio.PutObjectOptions{}) // Options rỗng
	if err != nil {
		return "", fmt.Errorf("failed to complete multipart upload: %w", err)
	}

	// Upload hoàn tất thành công, trả về ETag
	return uploadInfo.ETag, nil
}

// AbortMultipartUpload hủy bỏ một multipart upload đang dở dang
// Xóa tất cả các part đã upload để giải phóng storage space trên MinIO
func (m *MinioClient) AbortMultipartUpload(ctx context.Context, bucket, objectKey, uploadID string) error {
	// Gọi API của MinIO để abort upload session và xóa các part đã upload
	err := m.core.AbortMultipartUpload(
		ctx,       // Context
		bucket,    // Tên bucket
		objectKey, // Tên object
		uploadID)  // ID của upload session cần hủy
	// Nếu có lỗi khi abort (upload không tồn tại, ...), trả về lỗi
	if err != nil {
		return fmt.Errorf("failed to abort multipart upload: %w", err)
	}
	// Abort thành công, các part đã bị xóa
	return nil
}

// StatObject lấy thông tin metadata của object mà không cần download nội dung
// Dùng để kiểm tra object có tồn tại, lấy size, last modified time, content type, ...
func (m *MinioClient) StatObject(ctx context.Context, bucket, objectKey string) (minio.ObjectInfo, error) {
	// Gọi API StatObject của MinIO để lấy metadata
	info, err := m.client.StatObject(
		ctx,                       // Context
		bucket,                    // Tên bucket
		objectKey,                 // Tên object cần lấy info
		minio.StatObjectOptions{}) // Options rỗng
	// Nếu object không tồn tại hoặc không có quyền truy cập, trả về lỗi
	if err != nil {
		return minio.ObjectInfo{}, fmt.Errorf("failed to stat object: %w", err)
	}
	// Trả về ObjectInfo chứa Size, LastModified, ContentType, ETag, ...
	return info, nil
}

// GetObject tải object từ MinIO về dạng stream (không load hết vào memory)
// Trả về ReadCloser để caller có thể đọc data và phải nhớ Close() sau khi dùng xong
func (m *MinioClient) GetObject(ctx context.Context, bucket, objectKey string) (io.ReadCloser, error) {
	// Gọi API GetObject để mở stream đọc data từ MinIO
	obj, err := m.client.GetObject(
		ctx,                      // Context để cancel nếu cần
		bucket,                   // Tên bucket
		objectKey,                // Tên object cần download
		minio.GetObjectOptions{}) // Options rỗng (có thể thêm range để đọc từng phần)
	if err != nil {
		return nil, fmt.Errorf("failed to get object: %w", err)
	}
	// Trả về stream để caller đọc data (nhớ Close() sau khi xong)
	return obj, nil
}

// PutObject upload data lên MinIO từ một Reader (stream)
// Dùng khi có data trong memory hoặc từ stream khác mà không cần lưu vào file tạm
func (m *MinioClient) PutObject(ctx context.Context, bucket, objectKey string, reader io.Reader, size int64, contentType string) error {
	// Gọi API PutObject để upload stream lên MinIO
	_, err := m.client.PutObject(
		ctx,       // Context
		bucket,    // Tên bucket đích
		objectKey, // Tên object đích
		reader,    // Stream chứa data cần upload
		size,      // Kích thước data (bytes) - bắt buộc phải biết trước
		minio.PutObjectOptions{
			ContentType: contentType, // MIME type (video/mp4, image/png, ...)
		})
	// Nếu upload thất bại (network error, permission denied, ...), trả về lỗi
	if err != nil {
		return fmt.Errorf("failed to put object: %w", err)
	}
	// Upload thành công
	return nil
}

// GetPresignedDownloadURL tạo URL có chữ ký để client download trực tiếp từ MinIO
// URL này có thời gian hết hạn, sau đó sẽ không thể download được nữa (bảo mật)
func (m *MinioClient) GetPresignedDownloadURL(ctx context.Context, bucket, objectKey string, expires time.Duration) (string, error) {
	// Gọi API để tạo presigned GET URL với thời gian hết hạn tùy chỉnh
	presignedURL, err := m.client.PresignedGetObject(
		ctx,       // Context
		bucket,    // Tên bucket chứa object
		objectKey, // Tên object cần download
		expires,   // Thời gian URL có hiệu lực (ví dụ: 1 giờ, 1 ngày)
		nil)       // Request parameters bổ sung (có thể nil)
	// Nếu không thể tạo URL (object không tồn tại, ...), trả về lỗi
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}
	// Trả về URL string để client dùng cho download
	return presignedURL.String(), nil
}

// GetPresignedURL là wrapper function đơn giản hóa việc tạo presigned download URL
// Tự động dùng bucket mặc định từ config và convert expirySeconds sang Duration
func (m *MinioClient) GetPresignedURL(objectKey string, expirySeconds int) (string, error) {
	// Tạo background context (không có timeout)
	ctx := context.Background()
	// Convert số giây sang time.Duration
	duration := time.Duration(expirySeconds) * time.Second
	// Gọi method GetPresignedDownloadURL với bucket mặc định từ config
	return m.GetPresignedDownloadURL(ctx, m.cfg.MinIOBucketName, objectKey, duration)
}

// GetPresignedDownloadURLSimple là alias của GetPresignedURL, dùng default bucket
func (m *MinioClient) GetPresignedDownloadURLSimple(objectKey string, expirySeconds int) (string, error) {
	return m.GetPresignedURL(objectKey, expirySeconds)
}

// ListObjectsWithPrefix là alias của ListObjects, dùng default bucket
func (m *MinioClient) ListObjectsWithPrefix(prefix string) ([]minio.ObjectInfo, error) {
	return m.ListObjects(prefix)
}

// ListObjects liệt kê tất cả objects trong bucket có prefix matching
// Ví dụ: prefix="videos/" sẽ list tất cả file trong thư mục videos
func (m *MinioClient) ListObjects(prefix string) ([]minio.ObjectInfo, error) {
	// Tạo background context
	ctx := context.Background()
	// Slice để chứa kết quả
	var objects []minio.ObjectInfo

	// ListObjects trả về channel, iterate qua channel để lấy từng object
	for object := range m.client.ListObjects(
		ctx,                   // Context
		m.cfg.MinIOBucketName, // Bucket mặc định từ config
		minio.ListObjectsOptions{
			Prefix:    prefix, // Chỉ list objects có key bắt đầu bằng prefix này
			Recursive: true,   // true = tìm kiếm đệ quy trong tất cả "thư mục" con
		}) {
		// Kiểm tra xem có lỗi khi list không (permission denied, ...)
		if object.Err != nil {
			return nil, fmt.Errorf("failed to list objects: %w", object.Err)
		}
		// Thêm object vào kết quả
		objects = append(objects, object)
	}

	// Trả về danh sách tất cả objects tìm được
	return objects, nil
}

// PutObjectFromFile upload một file từ local filesystem lên MinIO
// Đơn giản hóa việc upload file có sẵn trên server thay vì phải tự mở file và đọc
func (m *MinioClient) PutObjectFromFile(ctx context.Context, bucket, objectKey, filePath, contentType string) error {
	// Mở file từ filesystem để đọc
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	// Đảm bảo đóng file sau khi function kết thúc (tránh leak file handle)
	defer file.Close()

	// Lấy thông tin file để biết size (cần thiết cho PutObject)
	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file stats: %w", err)
	}

	// Upload file lên MinIO bằng cách đọc stream từ file
	_, err = m.client.PutObject(
		ctx,         // Context
		bucket,      // Bucket đích
		objectKey,   // Key/tên object trên MinIO
		file,        // File handle làm Reader để đọc data
		stat.Size(), // Kích thước file (bytes)
		minio.PutObjectOptions{
			ContentType: contentType, // MIME type của file
		})
	// Nếu upload thất bại, trả về lỗi
	if err != nil {
		return fmt.Errorf("failed to upload file: %w", err)
	}
	// Upload thành công
	return nil
}

// GetDefaultBucket trả về tên bucket mặc định từ config
func (m *MinioClient) GetDefaultBucket() string {
	return m.cfg.MinIOBucketName
}

// EnsureBuckets tạo các bucket cần thiết nếu chưa tồn tại
// Gọi 1 lần khi khởi động app
func (m *MinioClient) EnsureBuckets(ctx context.Context) error {
	buckets := []string{
		m.cfg.MinioBucketVideos,
		m.cfg.MinioBucketImages,
		m.cfg.MinIOBucketName,
	}

	for _, bucket := range buckets {
		if bucket == "" {
			continue
		}
		exists, err := m.client.BucketExists(ctx, bucket)
		if err != nil {
			return fmt.Errorf("failed to check bucket %s: %w", bucket, err)
		}
		if !exists {
			if err := m.client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
				return fmt.Errorf("failed to create bucket %s: %w", bucket, err)
			}
			log.Printf("[MinIO] Created bucket: %s", bucket)
		} else {
			log.Printf("[MinIO] Bucket already exists: %s", bucket)
		}

		// Set bucket policy to allow public read (for images)
		if err := m.SetBucketPublicRead(ctx, bucket); err != nil {
			log.Printf("[MinIO] Warning: Failed to set public policy for bucket %s: %v", bucket, err)
		}
	}
	return nil
}

// SetBucketPublicRead sets bucket policy to allow anonymous read access
// This allows images to be displayed directly in browsers without authentication
func (m *MinioClient) SetBucketPublicRead(ctx context.Context, bucket string) error {
	// Policy JSON cho phép anonymous read tất cả objects trong bucket
	policy := fmt.Sprintf(`{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Effect": "Allow",
				"Principal": {"AWS": ["*"]},
				"Action": ["s3:GetObject"],
				"Resource": ["arn:aws:s3:::%s/*"]
			}
		]
	}`, bucket)

	err := m.client.SetBucketPolicy(ctx, bucket, policy)
	if err != nil {
		return fmt.Errorf("failed to set bucket policy: %w", err)
	}
	log.Printf("[MinIO] Set public read policy for bucket: %s", bucket)
	return nil
}

// GetMultiplePresignedUploadURLs tạo nhiều presigned URLs cùng lúc cho tất cả parts
// Thiết kế mới: presign toàn bộ parts 1 lần với expiry 30 phút
// Part size khuyến nghị: 16MB-32MB, phù hợp với 30 phút expiry
func (m *MinioClient) GetMultiplePresignedUploadURLs(ctx context.Context, bucket, objectKey, uploadID string, totalParts int) ([]string, error) {
	urls := make([]string, totalParts)

	// Tạo presigned URL cho từng part
	for i := 0; i < totalParts; i++ {
		partNumber := i + 1
		url, err := m.GetPresignedUploadURL(ctx, bucket, objectKey, uploadID, partNumber)
		if err != nil {
			return nil, fmt.Errorf("failed to generate URL for part %d: %w", partNumber, err)
		}
		urls[i] = url
	}

	return urls, nil
}

func (m *MinioClient) RefreshPresignedUploadURLs(ctx context.Context, bucket, objectKey, uploadID string, partNumbers []int) (map[int]string, error) {
	refreshedURLs := make(map[int]string)

	// Tạo lại presigned URL cho từng part number được yêu cầu
	for _, partNumber := range partNumbers {
		url, err := m.GetPresignedUploadURL(ctx, bucket, objectKey, uploadID, partNumber)
		if err != nil {
			return nil, fmt.Errorf("failed to refresh URL for part %d: %w", partNumber, err)
		}
		refreshedURLs[partNumber] = url
	}

	return refreshedURLs, nil
}

// UploadHLSDirectory upload toàn bộ thư mục HLS lên MinIO với concurrent worker pool
// Xóa từng local file ngay sau khi upload xong, xóa cả thư mục nếu tất cả thành công
func (m *MinioClient) UploadHLSDirectory(ctx context.Context, localDir, videoID, bucket string) error {
	type uploadJob struct {
		localPath   string
		objectKey   string
		contentType string
	}

	var jobs []uploadJob
	err := filepath.Walk(localDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		relPath, err := filepath.Rel(localDir, path)
		if err != nil {
			return err
		}
		ext := strings.ToLower(filepath.Ext(path))
		contentType := getHLSContentType(ext)
		jobs = append(jobs, uploadJob{
			localPath:   path,
			objectKey:   filepath.ToSlash(filepath.Join("videos", videoID, relPath)),
			contentType: contentType,
		})
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to walk HLS dir: %w", err)
	}

	const maxWorkers = 10
	jobCh := make(chan uploadJob, len(jobs))
	errCh := make(chan error, len(jobs))
	var wg sync.WaitGroup

	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobCh {
				if uploadErr := m.PutObjectFromFile(ctx, bucket, job.objectKey, job.localPath, job.contentType); uploadErr != nil {
					errCh <- fmt.Errorf("upload %s: %w", job.objectKey, uploadErr)
					continue
				}
				os.Remove(job.localPath)
			}
		}()
	}

	for _, job := range jobs {
		jobCh <- job
	}
	close(jobCh)
	wg.Wait()
	close(errCh)

	var errs []string
	for e := range errCh {
		errs = append(errs, e.Error())
	}
	if len(errs) > 0 {
		return fmt.Errorf("HLS upload errors: %s", strings.Join(errs, "; "))
	}

	// Xóa thư mục local chỉ khi tất cả upload thành công
	os.RemoveAll(localDir)
	return nil
}

func getHLSContentType(ext string) string {
	switch ext {
	case ".m3u8":
		return "application/vnd.apple.mpegurl"
	case ".ts":
		return "video/MP2T"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	default:
		return "application/octet-stream"
	}
}

// ValidateContentType kiểm tra xem content type có được hỗ trợ cho video upload không
func (m *MinioClient) ValidateContentType(contentType string) bool {
	supportedTypes := []string{
		"video/mp4",
		"video/avi",
		"video/mov",
		"video/wmv",
		"video/flv",
		"video/webm",
		"video/quicktime",
		"video/x-msvideo",
	}

	contentTypeLower := strings.ToLower(contentType)
	for _, supportedType := range supportedTypes {
		if strings.EqualFold(contentTypeLower, supportedType) {
			return true
		}
	}
	return false
}

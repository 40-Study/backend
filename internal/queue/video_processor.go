package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/utils"

	"github.com/google/uuid"
)

type VideoMetadata struct {
	Duration   float64 `json:"duration"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	Bitrate    int64   `json:"bitrate"`
	Format     string  `json:"format"`
	VideoCodec string  `json:"video_codec"`
	AudioCodec string  `json:"audio_codec"`
	FrameRate  float64 `json:"frame_rate"`
	FileSize   int64   `json:"file_size"`
}

// VideoStorageInterface defines methods needed for video processing
type VideoStorageInterface interface {
	GetObject(ctx context.Context, bucket, objectKey string) (io.ReadCloser, error)
	PutObjectFromFile(ctx context.Context, bucket, objectKey, filePath, contentType string) error
	GetPresignedDownloadURL(ctx context.Context, bucket, objectKey string, expires time.Duration) (string, error)
	UploadHLSDirectory(ctx context.Context, localDir, videoID, bucket string) error
}

type VideoProcessor struct {
	uploadRepo    repository.VideoUploadRepositoryInterface
	storage       VideoStorageInterface
	ffmpeg        *utils.FFmpeg // dùng cho EncodeVideoToHLS (pipe:0, adaptive ladder)
	ffmpegPath    string        // dùng cho các method cũ: thumbnail, preview, encode profiles
	tempDir       string
	clamavEnabled bool //
	maxRetries    int
}

func NewVideoProcessor(
	uploadRepo repository.VideoUploadRepositoryInterface,
	storage VideoStorageInterface,
	ffmpegPath, tempDir string,
	clamavEnabled bool, // là
	maxRetries int,
) *VideoProcessor {
	// Create temp directory if it doesn't exist
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		log.Printf("Warning: Failed to create temp directory %s: %v", tempDir, err)
	}

	return &VideoProcessor{
		uploadRepo:    uploadRepo,
		storage:       storage,
		ffmpeg:        &utils.FFmpeg{FFmpegPath: ffmpegPath, FFprobePath: "ffprobe"},
		ffmpegPath:    ffmpegPath,
		tempDir:       tempDir,
		clamavEnabled: clamavEnabled,
		maxRetries:    maxRetries,
	}
}

func (vp *VideoProcessor) ProcessVideo(ctx context.Context, message VideoProcessingMessage) error {
	id := message.UploadID.String()
	log.Printf("[VideoProcessor] [%s] ▶ START processing", id)

	// [1] Cập nhật DB: status = processing
	log.Printf("[VideoProcessor] [%s] [1/8] Updating DB status → processing...", id)
	now := time.Now()
	err := vp.updateUploadProcessingStatus(ctx, message.UploadID, &now, nil)
	if err != nil {
		log.Printf("[VideoProcessor] [%s] [1/8] FAILED: update processing status: %v", id, err)
		return err
	}
	log.Printf("[VideoProcessor] [%s] [1/8] ✓ DB status = processing", id)

	// [2] Download file gốc từ MinIO về local
	log.Printf("[VideoProcessor] [%s] [2/8] Downloading video from MinIO: bucket=%s key=%s", id, message.Bucket, message.ObjectKey)
	t := time.Now()
	videoPath, err := vp.downloadVideoFile(ctx, message.Bucket, message.ObjectKey)
	if err != nil {
		log.Printf("[VideoProcessor] [%s] [2/8] FAILED: download: %v", id, err)
		errorMsg := err.Error()
		vp.updateUploadStatus(ctx, message.UploadID, model.VideoUploadStatusFailed, &errorMsg)
		return fmt.Errorf("failed to download video file: %w", err)
	}
	defer vp.cleanupLocalFile(videoPath)
	log.Printf("[VideoProcessor] [%s] [2/8] ✓ Downloaded in %v → %s", id, time.Since(t), videoPath)

	// [3] Extract metadata bằng ffprobe
	log.Printf("[VideoProcessor] [%s] [3/8] Extracting metadata (ffprobe)...", id)
	t = time.Now()
	metadata, err := vp.ExtractVideoMetadata(videoPath)
	if err != nil {
		log.Printf("[VideoProcessor] [%s] [3/8] FAILED: extract metadata: %v", id, err)
		errorMsg := fmt.Sprintf("Failed to extract video metadata: %v", err)
		vp.updateUploadStatus(ctx, message.UploadID, model.VideoUploadStatusFailed, &errorMsg)
		return err
	}
	log.Printf("[VideoProcessor] [%s] [3/8] ✓ Metadata in %v: duration=%.2fs resolution=%dx%d bitrate=%d",
		id, time.Since(t), metadata.Duration, metadata.Width, metadata.Height, metadata.Bitrate)

	outputs := &VideoProcessingOutputs{
		Duration: &metadata.Duration,
		Bitrate:  &metadata.Bitrate,
	}

	resolution := fmt.Sprintf("%dx%d", metadata.Width, metadata.Height)
	outputs.Resolution = &resolution

	// [4] Virus scan
	if message.ProcessingOptions.ScanForViruses {
		log.Printf("[VideoProcessor] [%s] [4/8] Scanning for viruses (clamav)...", id)
		t = time.Now()
		isClean, err := vp.ScanForViruses(videoPath)
		if err != nil {
			log.Printf("[VideoProcessor] [%s] [4/8] WARNING: virus scan error (skipped): %v", id, err)
		} else {
			outputs.VirusScanOk = &isClean
			if !isClean {
				log.Printf("[VideoProcessor] [%s] [4/8] ✗ VIRUS DETECTED — aborting", id)
				errorMsg := "Virus detected in uploaded file"
				vp.updateUploadStatus(ctx, message.UploadID, model.VideoUploadStatusFailed, &errorMsg)
				return fmt.Errorf("virus detected")
			}
			log.Printf("[VideoProcessor] [%s] [4/8] ✓ Virus scan clean in %v", id, time.Since(t))
		}
	} else {
		log.Printf("[VideoProcessor] [%s] [4/8] SKIP virus scan (disabled)", id)
	}

	// [5] Tạo thumbnail
	if message.ProcessingOptions.GenerateThumbnail {
		log.Printf("[VideoProcessor] [%s] [5/8] Generating thumbnail...", id)
		t = time.Now()
		thumbnailURL, err := vp.generateAndUploadThumbnail(ctx, videoPath, message)
		if err != nil {
			log.Printf("[VideoProcessor] [%s] [5/8] WARNING: thumbnail failed (skipped): %v", id, err)
		} else {
			outputs.ThumbnailURL = thumbnailURL
			log.Printf("[VideoProcessor] [%s] [5/8] ✓ Thumbnail done in %v", id, time.Since(t))
		}
	} else {
		log.Printf("[VideoProcessor] [%s] [5/8] SKIP thumbnail (disabled)", id)
	}

	// [6] Tạo preview clip
	if message.ProcessingOptions.GeneratePreview {
		log.Printf("[VideoProcessor] [%s] [6/8] Generating preview clip...", id)
		t = time.Now()
		previewURL, err := vp.generateAndUploadPreview(ctx, videoPath, message, metadata.Duration)
		if err != nil {
			log.Printf("[VideoProcessor] [%s] [6/8] WARNING: preview failed (skipped): %v", id, err)
		} else {
			outputs.PreviewURL = previewURL
			log.Printf("[VideoProcessor] [%s] [6/8] ✓ Preview clip done in %v", id, time.Since(t))
		}
	} else {
		log.Printf("[VideoProcessor] [%s] [6/8] SKIP preview (disabled)", id)
	}

	// [7] Encode các profile (480p/720p/1080p)
	if len(message.ProcessingOptions.EncodeProfiles) > 0 {
		log.Printf("[VideoProcessor] [%s] [7/8] Encoding profiles: %v...", id, message.ProcessingOptions.EncodeProfiles)
		t = time.Now()
		encodedURLs, err := vp.encodeVideoProfiles(ctx, videoPath, message)
		if err != nil {
			log.Printf("[VideoProcessor] [%s] [7/8] WARNING: encode profiles failed (skipped): %v", id, err)
		} else {
			outputs.EncodedFiles = encodedURLs
			log.Printf("[VideoProcessor] [%s] [7/8] ✓ Profiles encoded in %v", id, time.Since(t))
		}
	} else {
		log.Printf("[VideoProcessor] [%s] [7/8] SKIP encode profiles (none configured)", id)
	}

	// [8] Encode HLS + upload toàn bộ lên MinIO
	log.Printf("[VideoProcessor] [%s] [8/8] Encoding HLS (ffmpeg) and uploading to MinIO...", id)
	t = time.Now()
	hlsMasterURL, err := vp.encodeAndUploadHLS(ctx, videoPath, message)
	if err != nil {
		log.Printf("[VideoProcessor] [%s] [8/8] FAILED: HLS encode/upload: %v", id, err)
	} else {
		outputs.HLSMasterURL = hlsMasterURL
		log.Printf("[VideoProcessor] [%s] [8/8] ✓ HLS done in %v → %s", id, time.Since(t), *hlsMasterURL)
	}

	log.Printf("[VideoProcessor] [%s] Saving results to DB...", id)
	completedAt := time.Now()
	err = vp.updateUploadWithResults(ctx, message.UploadID, outputs, &completedAt)
	if err != nil {
		log.Printf("[VideoProcessor] [%s] FAILED: update results: %v", id, err)
		return err
	}

	err = vp.updateUploadStatus(ctx, message.UploadID, model.VideoUploadStatusReady, nil)
	if err != nil {
		log.Printf("[VideoProcessor] [%s] FAILED: update final status: %v", id, err)
		return err
	}

	log.Printf("[VideoProcessor] [%s] ✅ ALL DONE — total: %v | status=ready", id, time.Since(now))
	return nil
}

// ExtractVideoMetadata extracts metadata using FFprobe
func (vp *VideoProcessor) ExtractVideoMetadata(filePath string) (*VideoMetadata, error) {
	cmd := exec.Command("ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		filePath,
	)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}

	var probeResult struct {
		Format struct {
			Duration string `json:"duration"`
			Size     string `json:"size"`
			Format   string `json:"format_name"`
		} `json:"format"`
		Streams []struct {
			CodecType  string `json:"codec_type"`
			CodecName  string `json:"codec_name"`
			Width      int    `json:"width,omitempty"`
			Height     int    `json:"height,omitempty"`
			BitRate    string `json:"bit_rate,omitempty"`
			RFrameRate string `json:"r_frame_rate,omitempty"`
		} `json:"streams"`
	}

	err = json.Unmarshal(output, &probeResult)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ffprobe output: %w", err)
	}

	// Parse duration
	duration, _ := strconv.ParseFloat(probeResult.Format.Duration, 64)
	fileSize, _ := strconv.ParseInt(probeResult.Format.Size, 10, 64)

	metadata := &VideoMetadata{
		Duration: duration,
		Format:   probeResult.Format.Format,
		FileSize: fileSize,
	}

	// Extract video and audio stream info
	for _, stream := range probeResult.Streams {
		switch stream.CodecType {
		case "video":
			metadata.Width = stream.Width
			metadata.Height = stream.Height
			metadata.VideoCodec = stream.CodecName
			if stream.BitRate != "" {
				bitrate, _ := strconv.ParseInt(stream.BitRate, 10, 64)
				metadata.Bitrate = bitrate
			}
			if stream.RFrameRate != "" {
				// Parse frame rate (e.g., "30/1" -> 30.0)
				parts := strings.Split(stream.RFrameRate, "/")
				if len(parts) == 2 {
					num, _ := strconv.ParseFloat(parts[0], 64)
					den, _ := strconv.ParseFloat(parts[1], 64)
					if den != 0 {
						metadata.FrameRate = num / den
					}
				}
			}
		case "audio":
			metadata.AudioCodec = stream.CodecName
		}
	}

	return metadata, nil
}

// GenerateThumbnail creates a thumbnail image from video
func (vp *VideoProcessor) GenerateThumbnail(inputPath, outputPath string) error {
	cmd := exec.Command(vp.ffmpegPath,
		"-i", inputPath,
		"-ss", "00:00:10", // Extract frame at 10 seconds
		"-vframes", "1",
		"-vf", "scale=320:240", // Resize to 320x240
		"-y", // Overwrite output file
		outputPath,
	)

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("thumbnail generation failed: %w", err)
	}

	return nil
}

// GenerateVideoPreview creates a short preview clip
func (vp *VideoProcessor) GenerateVideoPreview(inputPath, outputPath string, duration float64) error {
	// Generate 30-second preview from the middle of the video
	startTime := duration / 2.0
	if startTime > 30 {
		startTime = 30 // Start at 30 seconds if video is long enough
	}

	cmd := exec.Command(vp.ffmpegPath,
		"-i", inputPath,
		"-ss", fmt.Sprintf("%.2f", startTime),
		"-t", "30", // 30 seconds duration
		"-vf", "scale=640:480", // Lower resolution for preview
		"-c:v", "libx264",
		"-c:a", "aac",
		"-b:v", "500k", // Lower bitrate
		"-y",
		outputPath,
	)

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("preview generation failed: %w", err)
	}

	return nil
}

// ScanForViruses scans file using ClamAV
func (vp *VideoProcessor) ScanForViruses(filePath string) (bool, error) {
	if !vp.clamavEnabled {
		return true, nil // Skip if ClamAV not available
	}

	cmd := exec.Command("clamscan", "--no-summary", filePath)
	output, err := cmd.Output()

	// ClamAV returns 0 for clean, 1 for infected, 2 for error
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 {
				// File is infected
				log.Printf("Virus detected: %s", string(output))
				return false, nil
			}
		}
		return false, fmt.Errorf("virus scan failed: %w", err)
	}

	return true, nil // Clean file
}

// downloadVideoFile downloads the video file from storage to a local path
func (vp *VideoProcessor) downloadVideoFile(ctx context.Context, bucket, objectKey string) (string, error) {
	// Create local file path
	filename := filepath.Base(objectKey)
	localPath := filepath.Join(vp.tempDir, fmt.Sprintf("%s_%s", uuid.New().String(), filename))

	// Get object from MinIO
	object, err := vp.storage.GetObject(ctx, bucket, objectKey)
	if err != nil {
		return "", fmt.Errorf("failed to get object from MinIO: %w", err)
	}
	defer object.Close()

	// Create local file
	file, err := os.Create(localPath)
	if err != nil {
		return "", fmt.Errorf("failed to create local file: %w", err)
	}
	defer file.Close()

	// Copy data
	_, err = io.Copy(file, object)
	if err != nil {
		return "", fmt.Errorf("failed to download file: %w", err)
	}

	return localPath, nil
}

func (vp *VideoProcessor) cleanupLocalFile(filePath string) {
	if err := os.Remove(filePath); err != nil {
		log.Printf("Failed to cleanup local file %s: %v", filePath, err)
	}
}

// encodeAndUploadHLS encode video sang HLS adaptive ladder rồi upload lên MinIO.
// Dùng utils.FFmpeg.EncodeVideoToHLS:
//   - Tự chọn renditions phù hợp (không upscale nếu source nhỏ hơn)
//   - Tạo master.m3u8 tự động qua -var_stream_map
//   - Tạo poster.jpg từ frame đầu tiên
//   - Chạy 1 lần ffmpeg duy nhất qua pipe:0 (đọc file local đã download)
//
// Cấu trúc upload lên MinIO: videos/{id}/master.m3u8, videos/{id}/v0/..., videos/{id}/poster.jpg
func (vp *VideoProcessor) encodeAndUploadHLS(ctx context.Context, videoPath string, message VideoProcessingMessage) (*string, error) {
	// Tạo thư mục tạm để ffmpeg output HLS files (master.m3u8, v0/..., poster.jpg)
	outDir := filepath.Join(vp.tempDir, "hls_"+message.UploadID.String())
	if err := os.MkdirAll(outDir, 0755); err != nil { // 0755 để thư mục có thể đọc/ghi bởi ffmpeg
		return nil, fmt.Errorf("failed to create HLS output dir: %w", err)
	}
	// Cleanup toàn bộ thư mục kể cả khi lỗi sớm
	// (UploadHLSDirectory xóa từng file sau khi upload; defer xóa nốt phần còn lại)
	defer os.RemoveAll(outDir)

	// GetInput mở lại local file mỗi lần gọi (Probe gọi 1 lần, EncodeVideoToHLS gọi 1 lần)
	getInput := utils.GetInput(func(_ context.Context) (io.ReadCloser, error) {
		return os.Open(videoPath)
	})

	// EncodeVideoToHLS tạo toàn bộ cấu trúc bên trong outDir/hls/
	_, err := vp.ffmpeg.EncodeVideoToHLS(ctx, getInput, outDir)
	if err != nil {
		return nil, fmt.Errorf("HLS encode failed: %w", err)
	}

	// Upload outDir/hls/ → MinIO: videos/{id}/master.m3u8, videos/{id}/v0/..., ...
	hlsDir := filepath.Join(outDir, "hls")
	videoID := message.UploadID.String()
	if err := vp.storage.UploadHLSDirectory(ctx, hlsDir, videoID, message.Bucket); err != nil {
		return nil, fmt.Errorf("failed to upload HLS directory: %w", err)
	}

	masterKey := "videos/" + videoID + "/master.m3u8"
	return &masterKey, nil
}

// generateAndUploadThumbnail tạo thumbnail từ video, upload lên MinIO và trả về URL
func (vp *VideoProcessor) generateAndUploadThumbnail(ctx context.Context, videoPath string, message VideoProcessingMessage) (*string, error) {
	// Generate local thumbnail
	thumbnailPath := filepath.Join(vp.tempDir, fmt.Sprintf("%s_thumbnail.jpg", message.UploadID.String()))
	err := vp.GenerateThumbnail(videoPath, thumbnailPath)
	if err != nil {
		return nil, err
	}
	defer vp.cleanupLocalFile(thumbnailPath)

	// Upload to MinIO
	thumbnailKey := fmt.Sprintf("thumbnails/%s/%s_thumbnail.jpg", message.ResourceType, message.UploadID.String())
	err = vp.storage.PutObjectFromFile(ctx, message.Bucket, thumbnailKey, thumbnailPath, "image/jpeg")
	if err != nil {
		return nil, fmt.Errorf("failed to upload thumbnail: %w", err)
	}

	// Generate presigned URL for thumbnail access
	thumbnailURL, err := vp.storage.GetPresignedDownloadURL(ctx, message.Bucket, thumbnailKey, 24*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("failed to generate thumbnail URL: %w", err)
	}

	return &thumbnailURL, nil
}

// generateAndUploadPreview tạo video preview ngắn từ video gốc, upload lên MinIO và trả về URL
func (vp *VideoProcessor) generateAndUploadPreview(ctx context.Context, videoPath string, message VideoProcessingMessage, duration float64) (*string, error) {
	previewPath := filepath.Join(vp.tempDir, fmt.Sprintf("%s_preview.mp4", message.UploadID.String()))
	err := vp.GenerateVideoPreview(videoPath, previewPath, duration)
	if err != nil {
		return nil, err
	}
	defer vp.cleanupLocalFile(previewPath)

	previewKey := fmt.Sprintf("previews/%s/%s_preview.mp4", message.ResourceType, message.UploadID.String())
	err = vp.storage.PutObjectFromFile(ctx, message.Bucket, previewKey, previewPath, "video/mp4")
	if err != nil {
		return nil, fmt.Errorf("failed to upload preview: %w", err)
	}

	previewURL, err := vp.storage.GetPresignedDownloadURL(ctx, message.Bucket, previewKey, 24*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("failed to generate preview URL: %w", err)
	}

	return &previewURL, nil
}

func (vp *VideoProcessor) encodeVideoProfiles(ctx context.Context, videoPath string, message VideoProcessingMessage) (map[string]string, error) {
	encodedFiles := make(map[string]string)

	for _, profile := range message.ProcessingOptions.EncodeProfiles {
		encodedPath := filepath.Join(vp.tempDir, fmt.Sprintf("%s_%s.mp4", message.UploadID.String(), profile))
		err := vp.encodeVideoProfile(videoPath, encodedPath, profile)
		if err != nil {
			log.Printf("Failed to encode profile %s: %v", profile, err)
			continue
		}
		defer vp.cleanupLocalFile(encodedPath)

		encodedKey := fmt.Sprintf("encoded/%s/%s/%s.mp4", message.ResourceType, message.UploadID.String(), profile)
		err = vp.storage.PutObjectFromFile(ctx, message.Bucket, encodedKey, encodedPath, "video/mp4")
		if err != nil {
			log.Printf("Failed to upload encoded profile %s: %v", profile, err)
			continue
		}

		encodedFiles[profile] = encodedKey
	}

	return encodedFiles, nil
}

func (vp *VideoProcessor) encodeVideoProfile(inputPath, outputPath, profile string) error {
	var scale, bitrate string

	switch profile {
	case "480p":
		scale = "scale=854:480"
		bitrate = "1000k"
	case "720p":
		scale = "scale=1280:720"
		bitrate = "2500k"
	case "1080p":
		scale = "scale=1920:1080"
		bitrate = "5000k"
	default:
		return fmt.Errorf("unsupported encoding profile: %s", profile)
	}

	cmd := exec.Command(vp.ffmpegPath,
		"-i", inputPath,
		"-vf", scale,
		"-c:v", "libx264",
		"-c:a", "aac",
		"-b:v", bitrate,
		"-preset", "medium",
		"-y",
		outputPath,
	)

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("encoding failed for profile %s: %w", profile, err)
	}

	return nil
}

func (vp *VideoProcessor) updateUploadProcessingStatus(ctx context.Context, uploadID uuid.UUID, startedAt *time.Time, completedAt *time.Time) error {
	upload, err := vp.uploadRepo.GetUploadByID(ctx, uploadID)
	if err != nil {
		return err
	}

	if startedAt != nil {
		upload.ProcessingStartedAt = startedAt
		upload.Status = model.VideoUploadStatusProcessing
	}

	if completedAt != nil {
		upload.ProcessingCompletedAt = completedAt
	}

	return vp.uploadRepo.UpdateUpload(ctx, upload)
}

func (vp *VideoProcessor) updateUploadStatus(ctx context.Context, uploadID uuid.UUID, status model.VideoUploadStatus, errorMsg *string) error {
	return vp.uploadRepo.UpdateUploadStatus(ctx, uploadID, status, errorMsg)
}

func (vp *VideoProcessor) updateUploadWithResults(ctx context.Context, uploadID uuid.UUID, results *VideoProcessingOutputs, completedAt *time.Time) error {
	upload, err := vp.uploadRepo.GetUploadByID(ctx, uploadID)
	if err != nil {
		return err
	}

	// Update with processing results
	if results.ThumbnailURL != nil {
		upload.ThumbnailURL = results.ThumbnailURL
	}
	if results.HLSMasterURL != nil {
		upload.HLSMasterURL = results.HLSMasterURL
	}
	if results.Duration != nil {
		upload.Duration = results.Duration
	}
	if results.Resolution != nil {
		upload.Resolution = results.Resolution
	}
	if results.Bitrate != nil {
		upload.Bitrate = results.Bitrate
	}
	if completedAt != nil {
		upload.ProcessingCompletedAt = completedAt
	}

	return vp.uploadRepo.UpdateUpload(ctx, upload)
}

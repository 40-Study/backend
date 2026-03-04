package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type FFmpeg struct {
	FFmpegPath  string
	FFprobePath string
}

func NewFFmpeg() *FFmpeg {
	return &FFmpeg{
		FFmpegPath:  "ffmpeg",
		FFprobePath: "ffprobe",
	}
}

type GetInput func(ctx context.Context) (io.ReadCloser, error)

// Đầu ra của lệnh ffprobe dưới dạng JSON
type ffprobeOut struct {
	Streams []struct {
		CodecType string `json:"codec_type"`
		CodecName string `json:"codec_name"`
		Width     int32  `json:"width"`
		Height    int32  `json:"height"`
		BitRate   string `json:"bit_rate"`
		Duration  string `json:"duration"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
		BitRate  string `json:"bit_rate"`
	} `json:"format"`
}

// SourceMeta chứa metadata của file media
type SourceMeta struct {
	Width    int32   // độ rộng video
	Height   int32   // độ cao video
	Duration float64 // độ dài tính bằng giây
	BitrateK int32   // bitrate tính bằng kbps
	VCodec   string  // codec video chính
	HasAudio bool    // có kênh audio hay không
}

// Probe lấy thông tin metadata từ file media
func (f *FFmpeg) Probe(ctx context.Context, getInput GetInput) (SourceMeta, error) {
	in, err := getInput(ctx)
	if err != nil {
		return SourceMeta{}, err
	}
	defer in.Close()

	// out là kết quả byte của 1 chuỗi JSON trả về từ ffprobe có định dạng giống struct ffprobeOut
	out, err := runCmdStdin(ctx, f.FFprobePath, in,
		"-v", "error",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		"pipe:0",
	)
	if err != nil {
		return SourceMeta{}, err
	}

	var p ffprobeOut // Đây là struct để parse kết quả JSON từ ffprobe
	if err := json.Unmarshal(out, &p); err != nil {
		return SourceMeta{}, fmt.Errorf("ffprobe json parse: %w", err)
	}
	// Chuyển thông tin từ ffprobeOut sang SourceMeta
	var meta SourceMeta
	// Lấy duration và bitrate từ phần format
	meta.Duration = parseFloat(p.Format.Duration)
	// p.Format.BitRate là chuỗi, cần chuyển sang int64 rồi chia 1000 để ra kbps
	if br := parseInt64(p.Format.BitRate); br > 0 {
		meta.BitrateK = int32(br / 1000)
	}
	// Duyệt qua các stream để lấy thông tin video và audio
	// Nếu là video có hình và tiếng thì dừng, ưu tiên lấy thông tin từ stream video đầu tiên
	for _, s := range p.Streams {
		switch s.CodecType {
		case "video":
			if meta.Width == 0 && meta.Height == 0 {
				meta.Width, meta.Height = s.Width, s.Height
				meta.VCodec = s.CodecName
				if br := parseInt64(s.BitRate); br > 0 {
					meta.BitrateK = int32(br / 1000)
				}
				if meta.Duration <= 0 {
					meta.Duration = parseFloat(s.Duration)
				}
			}
		case "audio":
			meta.HasAudio = true
			if meta.Duration <= 0 {
				meta.Duration = parseFloat(s.Duration)
			}
		}
	}
	return meta, nil
}

type ImageEncodeResult struct {
	OptimizedRel string            // Đường dẫn tương đối đến ảnh tối ưu hóa
	ThumbsRel    map[string]string // Bản đồ các thumbnail với key là kích thước (ví dụ "256", "1024") và value là đường dẫn tương đối
}

// Encode Image chuyển đổi ảnh đầu vào thành định dạng WebP với các kích thước khác nhau
func (f *FFmpeg) EncodeImage(ctx context.Context, getInput GetInput, outDir string) (*ImageEncodeResult, error) {
	in, err := getInput(ctx)
	if err != nil {
		return nil, err
	}
	defer in.Close()

	imgDir := filepath.Join(outDir, "image")
	if err := os.MkdirAll(imgDir, 0o755); err != nil {
		return nil, err
	}

	optimized := filepath.Join(imgDir, "optimized.webp")
	thumb256 := filepath.Join(imgDir, "thumb_256.webp")
	thumb1024 := filepath.Join(imgDir, "thumb_1024.webp")

	// Tạo ảnh optimized (giữ nguyên kích thước)
	// Thumbnail 256px (không upscale nếu ảnh nhỏ hơn)
	// Thumbnail 1024px (không upscale nếu ảnh nhỏ hơn)
	//
	// Gộp 3 output vào 1 lần chạy ffmpeg để tránh decode 3 lần.
	filter := "[0:v]split=3[v0][v1][v2];" +
		"[v1]scale=w='min(256,iw)':h='min(256,ih)':force_original_aspect_ratio=decrease[v1s];" +
		"[v2]scale=w='min(1024,iw)':h='min(1024,ih)':force_original_aspect_ratio=decrease[v2s]"

	if _, err := runCmdStdin(ctx, f.FFmpegPath, in,
		"-hide_banner", "-loglevel", "error",
		"-y",
		"-i", "pipe:0",
		"-filter_complex", filter,

		"-map", "[v0]",
		"-frames:v", "1",
		"-c:v", "libwebp",
		"-q:v", "75",
		optimized,

		"-map", "[v1s]",
		"-frames:v", "1",
		"-c:v", "libwebp",
		"-q:v", "75",
		thumb256,

		"-map", "[v2s]",
		"-frames:v", "1",
		"-c:v", "libwebp",
		"-q:v", "75",
		thumb1024,
	); err != nil {
		return nil, fmt.Errorf("image encode: %w", err)
	}

	return &ImageEncodeResult{
		OptimizedRel: filepath.ToSlash(filepath.Join("image", "optimized.webp")),
		ThumbsRel: map[string]string{
			"256":  filepath.ToSlash(filepath.Join("image", "thumb_256.webp")),
			"1024": filepath.ToSlash(filepath.Join("image", "thumb_1024.webp")),
		},
	}, nil
}

type VariantOut struct {
	VariantType string
	Quality     string
	RelPath     string
	Height      int32
	BitrateKbps int32
	Codec       string
	IsDefault   bool
}

type VideoHLSResult struct {
	MasterRel string
	PosterRel string
	Variants  []VariantOut
}

type rendition struct {
	name string
	h    int32
	bv   string
	max  string
	buf  string
}

var baseLadder = []rendition{
	{name: "1080p", h: 1080, bv: "5000k", max: "5350k", buf: "7500k"},
	{name: "720p", h: 720, bv: "2800k", max: "2996k", buf: "4200k"},
	{name: "480p", h: 480, bv: "1400k", max: "1498k", buf: "2100k"},
	{name: "360p", h: 360, bv: "800k", max: "856k", buf: "1200k"},
}

// selectLadder chọn các rendition phù hợp (không upscale)
func selectLadder(sourceH int32) []rendition {
	var out []rendition
	for _, r := range baseLadder {
		if r.h <= sourceH {
			out = append(out, r)
		}
	}
	// Nếu video quá nhỏ, tạo 1 rendition với độ phân giải gốc
	if len(out) == 0 && sourceH > 0 {
		out = append(out, rendition{name: "source", h: sourceH, bv: "800k", max: "856k", buf: "1200k"})
	}
	return out
}

// EncodeVideoToHLS chuyển video thành HLS với nhiều chất lượng
func (f *FFmpeg) EncodeVideoToHLS(ctx context.Context, getInput GetInput, outDir string) (*VideoHLSResult, error) {
	// Lấy thông tin video gốc
	meta, err := f.Probe(ctx, getInput)
	if err != nil {
		return nil, err
	}
	if meta.Height <= 0 {
		return nil, fmt.Errorf("invalid source height")
	}

	rends := selectLadder(meta.Height)

	hlsDir := filepath.Join(outDir, "hls")
	if err := os.MkdirAll(hlsDir, 0o755); err != nil {
		return nil, err
	}
	for i := 0; i < len(rends); i++ {
		if err := os.MkdirAll(filepath.Join(hlsDir, fmt.Sprintf("v%d", i)), 0o755); err != nil {
			return nil, err
		}
	}

	// Tạo poster từ frame ở giây thứ 1 (hoặc 0.1s nếu video ngắn)
	ss := "00:00:01"
	if meta.Duration > 0 && meta.Duration < 1.0 {
		ss = "00:00:00.10"
	}
	posterPath := filepath.Join(hlsDir, "poster.jpg")

	// poster cần 1 input stream riêng
	posterIn, err := getInput(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := runCmdStdin(ctx, f.FFmpegPath, posterIn,
		"-hide_banner", "-loglevel", "error",
		"-y",
		"-ss", ss,
		"-i", "pipe:0",
		"-frames:v", "1",
		"-vf", "scale=w='min(1280,iw)':h=-2",
		posterPath,
	); err != nil {
		posterIn.Close()
		return nil, fmt.Errorf("poster: %w", err)
	}
	posterIn.Close()

	// Tạo filter_complex: split video thành N stream và scale
	var fc strings.Builder
	fc.WriteString(fmt.Sprintf("[0:v]split=%d", len(rends)))
	for i := 0; i < len(rends); i++ {
		fmt.Fprintf(&fc, "[v%d]", i)
	}
	fc.WriteString(";")
	for i, r := range rends {
		fmt.Fprintf(&fc, "[v%d]scale=w=-2:h=%d[v%dout];", i, r.h, i)
	}

	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-y",
		"-i", "pipe:0",
		"-filter_complex", fc.String(),
		"-preset", "veryfast",
		"-g", "48", "-keyint_min", "48",
		"-sc_threshold", "0",
	}

	// Map streams và cấu hình codec cho từng variant
	for i, r := range rends {
		args = append(args, "-map", fmt.Sprintf("[v%dout]", i))
		if meta.HasAudio {
			args = append(args, "-map", "a:0")
		}

		// Cấu hình video codec
		args = append(args,
			fmt.Sprintf("-c:v:%d", i), "libx264",
			fmt.Sprintf("-b:v:%d", i), r.bv,
			fmt.Sprintf("-maxrate:v:%d", i), r.max,
			fmt.Sprintf("-bufsize:v:%d", i), r.buf,
			fmt.Sprintf("-profile:v:%d", i), "main",
			fmt.Sprintf("-crf:v:%d", i), "20",
			fmt.Sprintf("-pix_fmt:v:%d", i), "yuv420p",
		)

		// Cấu hình audio codec nếu có
		if meta.HasAudio {
			args = append(args,
				fmt.Sprintf("-c:a:%d", i), "aac",
				fmt.Sprintf("-b:a:%d", i), "128k",
				fmt.Sprintf("-ac:a:%d", i), "2",
				fmt.Sprintf("-ar:a:%d", i), "48000",
			)
		}
	}

	// Tạo var_stream_map
	var vsm strings.Builder
	for i := range rends {
		if i > 0 {
			vsm.WriteString(" ")
		}
		if meta.HasAudio {
			fmt.Fprintf(&vsm, "v:%d,a:%d", i, i)
		} else {
			fmt.Fprintf(&vsm, "v:%d", i)
		}
	}

	args = append(args,
		"-f", "hls",
		"-hls_time", "4",
		"-hls_playlist_type", "vod",
		"-hls_flags", "independent_segments",
		"-hls_segment_filename", filepath.Join(hlsDir, "v%v", "seg_%05d.ts"),
		"-master_pl_name", "master.m3u8",
		"-var_stream_map", vsm.String(),
		filepath.Join(hlsDir, "v%v", "index.m3u8"),
	)

	// encode HLS cần 1 input stream riêng
	encodeIn, err := getInput(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := runCmdStdin(ctx, f.FFmpegPath, encodeIn, args...); err != nil {
		encodeIn.Close()
		return nil, fmt.Errorf("encode hls: %w", err)
	}
	encodeIn.Close()

	// Chọn variant mặc định (ưu tiên 720p)
	defaultIdx := 0
	for i, r := range rends {
		if r.name == "720p" {
			defaultIdx = i
			break
		}
	}

	var variants []VariantOut
	for i, r := range rends {
		variants = append(variants, VariantOut{
			VariantType: "HLS_VIDEO",
			Quality:     r.name,
			RelPath:     filepath.ToSlash(filepath.Join("hls", fmt.Sprintf("v%d", i), "index.m3u8")),
			Height:      r.h,
			BitrateKbps: parseKbps(r.bv),
			Codec:       "h264",
			IsDefault:   i == defaultIdx,
		})
	}

	return &VideoHLSResult{
		MasterRel: filepath.ToSlash(filepath.Join("hls", "master.m3u8")),
		PosterRel: filepath.ToSlash(filepath.Join("hls", "poster.jpg")),
		Variants:  variants,
	}, nil
}

// runCmdStdin giống runCmd nhưng nhận thêm stdin để chạy với input pipe (pipe:0).
func runCmdStdin(ctx context.Context, bin string, stdin io.Reader, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdin = stdin

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("%s failed: %w\nargs=%v\nstderr=%s", bin, err, args, stderr.String())
	}
	return stdout.Bytes(), nil
}

func parseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func parseInt64(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

func parseKbps(bitrate string) int32 {
	bitrate = strings.TrimSpace(strings.ToLower(bitrate))
	bitrate = strings.TrimSuffix(bitrate, "k")
	n, _ := strconv.ParseInt(bitrate, 10, 32)
	return int32(n)
}

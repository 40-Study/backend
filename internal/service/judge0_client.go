package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"study.com/v1/internal/config"
	"study.com/v1/internal/dto"
)

// Giới hạn tối đa gửi tới Judge0 (contract sec. 8): mặc định của upstream v1.13.1 judge0.conf (MAX_CPU_TIME_LIMIT,
// MAX_MEMORY_LIMIT). Vượt là Judge0 trả 422, nên mọi request bị kẹp ở đây để giá trị của giảng viên không bao giờ
// thành lỗi.
const (
	Judge0MaxCPUTimeSeconds = 15
	Judge0MaxMemoryKB       = 512000
)

// judge0ErrorBodyLimit: số byte body lỗi của Judge0 giữ lại trong error (chỉ để log, không bao giờ trả client).
const judge0ErrorBodyLimit = 512

// Judge0Client chấm một lần chạy (POST /submissions?wait=true). Lỗi bọc ErrJudgeUnavailable khi máy chấm không
// trả lời được; lỗi khác (vd 422 do request ta dựng sai) là lỗi thường.
type Judge0Client interface {
	Submit(ctx context.Context, req dto.Judge0SubmissionDTO) (*dto.Judge0ResponseDTO, error)
}

type HTTPJudge0Client struct {
	baseURL    string
	authToken  string
	httpClient *http.Client
}

// NewJudge0ClientFromConfig dựng client từ JUDGE0_URL / JUDGE0_AUTH_TOKEN / JUDGE0_TIMEOUT_SECONDS. Config dựng tay
// (URL rỗng, timeout 0) vẫn ra đúng mặc định như LoadConfig.
func NewJudge0ClientFromConfig(cfg *config.Config) *HTTPJudge0Client {
	return newHTTPJudge0Client(config.ResolveJudge0URL(cfg.Judge0URL, cfg.Environment), cfg.Judge0AuthToken, cfg.Judge0Timeout())
}

func newHTTPJudge0Client(baseURL, authToken string, timeout time.Duration) *HTTPJudge0Client {
	return &HTTPJudge0Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		authToken:  authToken,
		httpClient: &http.Client{Timeout: timeout},
	}
}

// createJudge0Request creates a Judge0 submission request
// cpuTimeLimit is per test case (not total assignment time), default 5 seconds, max 20 seconds
// (Submit kẹp tiếp về Judge0MaxCPUTimeSeconds / Judge0MaxMemoryKB trước khi gửi).
func createJudge0Request(sourceCode string, languageID int, stdin string, expectedOutput string, cpuTimeLimit int, memoryLimit int) dto.Judge0SubmissionDTO {
	// Default 5 seconds per test case if not specified or too high
	cpuLimit := 5.0
	if cpuTimeLimit > 0 && cpuTimeLimit <= 20 {
		cpuLimit = float64(cpuTimeLimit)
	}
	return dto.Judge0SubmissionDTO{
		SourceCode:     sourceCode,
		LanguageID:     languageID,
		Stdin:          stdin,
		ExpectedOutput: expectedOutput,
		CPUTimeLimit:   cpuLimit,
		MemoryLimit:    memoryLimit,
	}
}

// clampJudge0Limits kẹp giới hạn về mức Judge0 chấp nhận (giá trị nhỏ hơn giữ nguyên).
func clampJudge0Limits(req dto.Judge0SubmissionDTO) dto.Judge0SubmissionDTO {
	if req.CPUTimeLimit > Judge0MaxCPUTimeSeconds {
		req.CPUTimeLimit = Judge0MaxCPUTimeSeconds
	}
	if req.MemoryLimit > Judge0MaxMemoryKB {
		req.MemoryLimit = Judge0MaxMemoryKB
	}
	return req
}

func (c *HTTPJudge0Client) Submit(ctx context.Context, req dto.Judge0SubmissionDTO) (*dto.Judge0ResponseDTO, error) {
	body, err := json.Marshal(clampJudge0Limits(req))
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/submissions?wait=true", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.authToken != "" {
		httpReq.Header.Set("X-Auth-Token", c.authToken)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		// Lỗi kết nối, DNS, timeout, context huỷ: máy chấm không trả lời.
		return nil, fmt.Errorf("%w: %w", ErrJudgeUnavailable, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: read response: %w", ErrJudgeUnavailable, err)
	}

	if resp.StatusCode >= 400 {
		detail := string(respBody)
		if len(detail) > judge0ErrorBodyLimit {
			detail = detail[:judge0ErrorBodyLimit]
		}
		if resp.StatusCode == http.StatusUnprocessableEntity {
			// 422: request ta dựng sai (contract sec. 1) -> lỗi nội bộ, không phải "máy chấm bận".
			return nil, fmt.Errorf("judge0 rejected request (HTTP 422): %s", detail)
		}
		// 5xx (gồm 503 "queue is full") và 4xx khác (gồm 400 "wait not allowed", 401 sai token).
		return nil, fmt.Errorf("%w: judge0 HTTP %d: %s", ErrJudgeUnavailable, resp.StatusCode, detail)
	}

	var result dto.Judge0ResponseDTO
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

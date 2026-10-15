package service

// Client Judge0 (contract sec. 1 và 8): lỗi vận chuyển, timeout, HTTP 5xx và 4xx khác 422 là ErrJudgeUnavailable
// (503 cho người dùng, không bao giờ thành verdict); 422 là lỗi request do ta dựng sai nên KHÔNG phải
// ErrJudgeUnavailable (500 / system_error). Header X-Auth-Token chỉ gửi khi có cấu hình; giới hạn gửi đi bị kẹp để
// giá trị của giảng viên không bao giờ gây 422. httptest.Server, không cần Judge0 thật.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"study.com/v1/internal/config"
	"study.com/v1/internal/dto"
)

type judge0Capture struct {
	mu      sync.Mutex
	body    dto.Judge0SubmissionDTO
	token   string
	hasAuth bool
	path    string
}

func newJudge0Stub(t *testing.T, status int, respBody string, delay time.Duration) (*httptest.Server, *judge0Capture) {
	t.Helper()
	capt := &judge0Capture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		capt.mu.Lock()
		_ = json.Unmarshal(raw, &capt.body)
		capt.token = r.Header.Get("X-Auth-Token")
		_, capt.hasAuth = r.Header["X-Auth-Token"]
		capt.path = r.URL.RequestURI()
		capt.mu.Unlock()
		if delay > 0 {
			time.Sleep(delay)
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, respBody)
	}))
	t.Cleanup(srv.Close)
	return srv, capt
}

const judge0AcceptedBody = `{"stdout":"3\n","stderr":null,"compile_output":null,"status":{"id":3,"description":"Accepted"},"time":"0.01","memory":3000}`

func TestJudge0Client_Accepted(t *testing.T) {
	srv, capt := newJudge0Stub(t, http.StatusCreated, judge0AcceptedBody, 0)
	c := newHTTPJudge0Client(srv.URL, "", time.Second)
	res, err := c.Submit(context.Background(), createJudge0Request("print(3)", 71, "", "3", 2, 1024))
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if res.Status.ID != 3 || res.Stdout != "3\n" || res.Memory != 3000 {
		t.Fatalf("kết quả = %+v", res)
	}
	if capt.path != "/submissions?wait=true" {
		t.Errorf("path = %q, muốn /submissions?wait=true", capt.path)
	}
}

func TestJudge0Client_UnavailableClasses(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"503 queue is full", http.StatusServiceUnavailable, `{"error":"queue is full"}`},
		{"500", http.StatusInternalServerError, `oops`},
		{"400 wait not allowed", http.StatusBadRequest, `{"error":"wait not allowed"}`},
		{"401 thiếu token", http.StatusUnauthorized, ``},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := newJudge0Stub(t, tc.status, tc.body, 0)
			_, err := newHTTPJudge0Client(srv.URL, "", time.Second).Submit(context.Background(), createJudge0Request("x", 71, "", "", 2, 1024))
			if !errors.Is(err, ErrJudgeUnavailable) {
				t.Fatalf("HTTP %d: err = %v, muốn ErrJudgeUnavailable", tc.status, err)
			}
		})
	}
}

func TestJudge0Client_422IsNotUnavailable(t *testing.T) {
	srv, _ := newJudge0Stub(t, http.StatusUnprocessableEntity, `{"cpu_time_limit":["must be less than or equal to 15"]}`, 0)
	_, err := newHTTPJudge0Client(srv.URL, "", time.Second).Submit(context.Background(), createJudge0Request("x", 71, "", "", 2, 1024))
	if err == nil {
		t.Fatal("422 phải là lỗi")
	}
	if errors.Is(err, ErrJudgeUnavailable) {
		t.Fatalf("422 (request ta dựng sai) không được là ErrJudgeUnavailable: %v", err)
	}
}

func TestJudge0Client_ClosedPortAndTimeoutAreUnavailable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closedURL := "http://" + ln.Addr().String()
	_ = ln.Close()
	_, err = newHTTPJudge0Client(closedURL, "", time.Second).Submit(context.Background(), createJudge0Request("x", 71, "", "", 2, 1024))
	if !errors.Is(err, ErrJudgeUnavailable) {
		t.Fatalf("cổng đóng: err = %v, muốn ErrJudgeUnavailable", err)
	}

	slow, _ := newJudge0Stub(t, http.StatusCreated, judge0AcceptedBody, 300*time.Millisecond)
	_, err = newHTTPJudge0Client(slow.URL, "", 50*time.Millisecond).Submit(context.Background(), createJudge0Request("x", 71, "", "", 2, 1024))
	if !errors.Is(err, ErrJudgeUnavailable) {
		t.Fatalf("timeout: err = %v, muốn ErrJudgeUnavailable", err)
	}
}

func TestJudge0Client_AuthTokenOnlyWhenConfigured(t *testing.T) {
	srv, capt := newJudge0Stub(t, http.StatusCreated, judge0AcceptedBody, 0)
	if _, err := newHTTPJudge0Client(srv.URL, "", time.Second).Submit(context.Background(), createJudge0Request("x", 71, "", "", 2, 1024)); err != nil {
		t.Fatal(err)
	}
	if capt.hasAuth {
		t.Errorf("không cấu hình token mà vẫn gửi X-Auth-Token %q", capt.token)
	}
	if _, err := newHTTPJudge0Client(srv.URL, "s3cret", time.Second).Submit(context.Background(), createJudge0Request("x", 71, "", "", 2, 1024)); err != nil {
		t.Fatal(err)
	}
	if capt.token != "s3cret" {
		t.Errorf("X-Auth-Token = %q, muốn s3cret", capt.token)
	}
}

func TestJudge0Client_ClampsLimits(t *testing.T) {
	srv, capt := newJudge0Stub(t, http.StatusCreated, judge0AcceptedBody, 0)
	c := newHTTPJudge0Client(srv.URL, "", time.Second)

	if _, err := c.Submit(context.Background(), createJudge0Request("x", 71, "", "", 20, 1024*1024)); err != nil {
		t.Fatal(err)
	}
	if capt.body.CPUTimeLimit != Judge0MaxCPUTimeSeconds || capt.body.MemoryLimit != Judge0MaxMemoryKB {
		t.Fatalf("gửi cpu=%v mem=%d, muốn kẹp về %d / %d", capt.body.CPUTimeLimit, capt.body.MemoryLimit, Judge0MaxCPUTimeSeconds, Judge0MaxMemoryKB)
	}

	if _, err := c.Submit(context.Background(), createJudge0Request("x", 71, "", "", 2, 256*1024)); err != nil {
		t.Fatal(err)
	}
	if capt.body.CPUTimeLimit != 2 || capt.body.MemoryLimit != 256*1024 {
		t.Fatalf("giá trị trong giới hạn bị đổi: cpu=%v mem=%d", capt.body.CPUTimeLimit, capt.body.MemoryLimit)
	}
}

func TestNewJudge0ClientFromConfig(t *testing.T) {
	c := NewJudge0ClientFromConfig(&config.Config{Environment: "dev"})
	if c.baseURL != "http://localhost:2358" || c.httpClient.Timeout != 30*time.Second || c.authToken != "" {
		t.Errorf("dev mặc định: url=%q timeout=%v token=%q", c.baseURL, c.httpClient.Timeout, c.authToken)
	}
	c = NewJudge0ClientFromConfig(&config.Config{Environment: "prod"})
	if c.baseURL != "http://judge0:2358" {
		t.Errorf("prod mặc định: url=%q, muốn http://judge0:2358", c.baseURL)
	}
	c = NewJudge0ClientFromConfig(&config.Config{Environment: "dev", Judge0URL: "http://j0.internal:9000/", Judge0AuthToken: "tok", Judge0TimeoutSeconds: 7})
	if c.baseURL != "http://j0.internal:9000" || c.httpClient.Timeout != 7*time.Second || c.authToken != "tok" {
		t.Errorf("ghi đè: url=%q timeout=%v token=%q", c.baseURL, c.httpClient.Timeout, c.authToken)
	}
}

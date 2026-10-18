package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"study.com/v1/internal/config"
	"sync/atomic"
	"testing"
	"time"
)

const teachingTestSlides = `{"kind":"slides","title":"Bài giảng Python","slides":[{"title":"Vòng lặp","subtitle":"Python cơ bản","bullets":["Lặp qua các phần tử"],"notes":"Cho học sinh thực hành.","layout":"cover"}]}`
const teachingTestQuiz = `{"kind":"quiz","title":"Quiz Python","questions":[{"question":"2 + 2 = ?","options":["3","4","5","6"],"correct_answer":1,"explanation":"Cộng hai với hai bằng bốn."}]}`

func designTestInput() DesignRequest {
	return DesignRequest{RequestID: "run-1", Output: "slides", Prompt: "Tạo bài giảng", Count: 1, Documents: []TeachingDocument{{Name: "lesson.txt", Text: "Vòng lặp for duyệt từng phần tử."}}}
}
func designTestService(server *httptest.Server) *OpenDesignService {
	return NewOpenDesignService(&config.Config{ViettelAIAPIKey: "test-secret-never-public", ViettelAIBaseURL: server.URL + "/v1", ViettelAIModel: "test-model"})
}
func providerResult(content string) string {
	raw, _ := json.Marshal(map[string]interface{}{"choices": []interface{}{map[string]interface{}{"message": map[string]string{"content": content}, "finish_reason": "stop"}}})
	return string(raw)
}
func TestOpenDesignProviderContract(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer test-secret-never-public" {
			t.Error("incorrect provider request")
		}
		var body struct {
			Model    string          `json:"model"`
			Messages []DesignMessage `json:"messages"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || body.Model != "test-model" || len(body.Messages) != 2 {
			t.Error("incorrect body")
		}
		if !strings.Contains(body.Messages[0].Content, "--primary: 221 83% 53%") || !strings.Contains(body.Messages[1].Content, "lesson.txt") {
			t.Error("missing tokens or source")
		}
		_, _ = w.Write([]byte(providerResult(teachingTestSlides)))
	}))
	defer server.Close()
	result, err := designTestService(server).Generate(context.Background(), "owner", designTestInput())
	if err != nil || result.Artifact.Kind != "slides" {
		t.Fatalf("%+v %v", result, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("duplicate calls %d", calls.Load())
	}
}
func TestOpenDesignQuizAnswerValidation(t *testing.T) {
	artifact, err := parseTeachingArtifact(teachingTestQuiz, "quiz")
	if err != nil || *artifact.Questions[0].CorrectAnswer != 1 {
		t.Fatal("correct answer not retained")
	}
	for _, raw := range []string{strings.Replace(teachingTestQuiz, `"correct_answer":1,`, "", 1), strings.Replace(teachingTestQuiz, `"correct_answer":1`, `"correct_answer":4`, 1), strings.Replace(teachingTestQuiz, `["3","4","5","6"]`, `["3","3","5","6"]`, 1)} {
		if _, err := parseTeachingArtifact(raw, "quiz"); err == nil {
			t.Fatal("invalid answer/options accepted")
		}
	}
	if _, err := parseTeachingArtifact(teachingTestSlides, "quiz"); err == nil {
		t.Fatal("wrong output type accepted")
	}
}
func TestOpenDesignFailureDoesNotReturnPartialDraft(t *testing.T) {
	for _, tc := range []struct {
		status   int
		body     string
		expected error
	}{{401, `secret provider trace`, ErrDesignUpstream}, {429, `{}`, ErrDesignRateLimit}, {200, `invalid JSON`, ErrDesignUpstream}, {200, providerResult(`{"kind":"slides"}`), nil}, {200, `{"choices":[{"finish_reason":"length"}]}`, ErrDesignTruncated}} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}))
		result, err := designTestService(server).Generate(context.Background(), "u", designTestInput())
		server.Close()
		if err == nil || result != nil || (tc.expected != nil && !errors.Is(err, tc.expected)) {
			t.Fatalf("result=%v err=%v", result, err)
		}
		if strings.Contains(err.Error(), "secret provider") {
			t.Fatal("provider detail leaked")
		}
	}
}
func TestOpenDesignMissingConfig(t *testing.T) {
	s := NewOpenDesignService(&config.Config{})
	if s.Configured() {
		t.Fatal("empty configuration ready")
	}
	_, err := s.Generate(context.Background(), "u", designTestInput())
	if !errors.Is(err, ErrDesignNotConfigured) {
		t.Fatal(err)
	}
}
func TestOpenDesignCancelAndConcurrentSend(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		select {
		case <-r.Context().Done():
			close(cancelled)
		case <-time.After(5 * time.Second):
		}
	}))
	defer server.Close()
	s := designTestService(server)
	done := make(chan error, 1)
	go func() { _, err := s.Generate(context.Background(), "owner", designTestInput()); done <- err }()
	<-started
	if _, err := s.Generate(context.Background(), "owner", designTestInput()); !errors.Is(err, ErrDesignBusy) {
		t.Fatal("duplicate accepted")
	}
	if s.Cancel("other", "run-1") || s.Cancel("owner", "wrong") {
		t.Fatal("cross-owner cancellation")
	}
	if !s.Cancel("owner", "run-1") {
		t.Fatal("cancel failed")
	}
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream not canceled")
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestOpenDesignCancelBeforeGenerate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("provider should not run") }))
	defer server.Close()
	s := designTestService(server)
	s.Cancel("owner", "run-1")
	_, err := s.Generate(context.Background(), "owner", designTestInput())
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestOpenDesignDocuments(t *testing.T) {
	var zipped bytes.Buffer
	archive := zip.NewWriter(&zipped)
	doc, _ := archive.Create("word/document.xml")
	_, _ = doc.Write([]byte(`<w:document xmlns:w="word"><w:p><w:r><w:t>Vòng lặp Python</w:t></w:r></w:p><w:p><w:r><w:t>Ví dụ thực hành</w:t></w:r></w:p></w:document>`))
	_ = archive.Close()
	for _, tc := range []struct {
		name string
		data []byte
		ok   bool
	}{{"lesson.txt", []byte("Vòng lặp for"), true}, {"lesson.docx", zipped.Bytes(), true}, {"fake.pdf", []byte("not PDF"), false}, {"empty.txt", []byte("  "), false}, {"binary.txt", []byte{0, 1, 2}, false}, {"image.png", []byte("hello"), false}} {
		result, err := ExtractTeachingDocument(tc.name, tc.data)
		if tc.ok && (err != nil || !strings.Contains(result.Text, "Vòng")) {
			t.Fatalf("%s: %+v %v", tc.name, result, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("%s invalid accepted", tc.name)
		}
	}
}

package handler

// Lỗi: khi service leaderboard trả lỗi hạ tầng (DB/SQL), handler trả 500 kèm nguyên văn err.Error()
// ra response, lộ chi tiết nội bộ cho client. Test khoá lại: vẫn 500 nhưng body KHÔNG chứa chuỗi lỗi gốc.

import (
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"study.com/v1/internal/dto"
)

const leaderboardInternalErrText = `pq: relation "leaderboard_entries" does not exist (SELECT secret_column FROM leaderboard_entries)`

type failingLeaderboardService struct{}

func (failingLeaderboardService) GetLeaderboard(ctx context.Context, periodType string, limit int) (*dto.LeaderboardResponse, error) {
	return nil, errors.New(leaderboardInternalErrText)
}

func (failingLeaderboardService) GetMyRank(ctx context.Context, userID uuid.UUID, periodType string) (*dto.MyRankResponse, error) {
	return nil, errors.New(leaderboardInternalErrText)
}

func TestLeaderboardHandler_InternalErrorNotLeaked(t *testing.T) {
	h := NewLeaderboardHandler(failingLeaderboardService{})
	endpoints := []struct {
		name string
		path string
		app  *fiber.App
	}{
		{"GetLeaderboard", "/leaderboard?period_type=weekly", func() *fiber.App {
			app := fiber.New()
			app.Get("/leaderboard", h.GetLeaderboard)
			return app
		}()},
		{"GetMyRank", "/leaderboard/me?period_type=weekly", mountWithCaller("GET", "/leaderboard/me", uuid.New(), h.GetMyRank)},
	}

	for _, ep := range endpoints {
		t.Run(ep.name, func(t *testing.T) {
			resp, err := ep.app.Test(httptest.NewRequest("GET", ep.path, nil))
			if err != nil {
				t.Fatalf("app.Test loi: %v", err)
			}
			if resp.StatusCode != fiber.StatusInternalServerError {
				t.Fatalf("status = %d, muon 500", resp.StatusCode)
			}
			body, _ := io.ReadAll(resp.Body)
			for _, leak := range []string{leaderboardInternalErrText, "leaderboard_entries", "secret_column", "pq:"} {
				if strings.Contains(string(body), leak) {
					t.Fatalf("response lo chi tiet noi bo %q: %s", leak, body)
				}
			}
		})
	}
}

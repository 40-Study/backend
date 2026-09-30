package router

// Test HTTP qua route THẬT (AuthMiddleware -> limiter -> handler -> service -> Postgres schema tạm) cho
// /api/friends (phase 01, plan 260930), từng vai: học viên, phụ huynh, giáo viên, admin, người lạ, người bị
// chặn, khách. Bỏ kiểm vai trò ở service thì nhóm "vai" đỏ. Nhóm "khách" vẫn 401 kể cả khi thiếu
// AuthMiddleware (parseUserID của handler cũng trả 401): đó là hai lớp bảo vệ, không phải test vô nghĩa.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"study.com/v1/internal/config"
	"study.com/v1/internal/constants"
	"study.com/v1/internal/database"
	"study.com/v1/internal/handler"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/service"
	"study.com/v1/internal/testutil/pgtest"
	"study.com/v1/internal/utils"
)

type friendLiveEnv struct {
	t   *testing.T
	app *fiber.App
	mr  *miniredis.Miniredis
	db  *gorm.DB
	tok map[string]string
	ids map[string]uuid.UUID
}

type friendResp struct {
	Status int
	Raw    string
	Body   struct {
		Message string          `json:"message"`
		Code    string          `json:"code"`
		Data    json.RawMessage `json:"data"`
	}
}

func newFriendLiveEnv(t *testing.T) *friendLiveEnv {
	t.Helper()
	db := pgtest.IsolatedSchema(t, database.Migrate)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cfg := &config.Config{JWTSecret: "friend-live-secret", JWTAccessExpiration: 15 * time.Minute, JWTRefreshExpiration: 7 * 24 * time.Hour}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	e := &friendLiveEnv{t: t, mr: mr, db: db, tok: map[string]string{}, ids: map[string]uuid.UUID{}}

	roleIDs := map[string]uuid.UUID{}
	mk := func(name string, roles ...string) {
		full := "Ho Ten " + name
		u := model.User{Email: name + "-" + uuid.NewString()[:6] + "@40study.test", PasswordHash: "x",
			UserName: name + uuid.NewString()[:6], FullName: &full, IsActive: true}
		must(db.Create(&u).Error)
		must(rdb.Set(context.Background(), constants.KeyUserVersion(u.ID.String()), 1, 0).Err())
		tok, _, err := utils.GenerateTokens(cfg, u.ID, uuid.New(), "STUDENT", nil, 1)
		must(err)
		e.tok[name], e.ids[name] = tok, u.ID
		for _, r := range roles {
			id, ok := roleIDs[r]
			if !ok {
				sr := model.SystemRole{Name: r, Status: "active"}
				must(db.Create(&sr).Error)
				id = sr.ID
				roleIDs[r] = id
			}
			must(db.Create(&model.UserSystemRole{UserID: u.ID, SystemRoleID: id, Status: model.UserSystemRoleStatusActive}).Error)
		}
	}
	for _, n := range []string{"alice", "bob", "carol", "dave", "erin", "frank"} {
		mk(n, "STUDENT")
	}
	mk("parent", "PARENT")
	mk("teacher", "TEACHER")
	mk("admin", "SYSTEM_ADMIN")

	svc := service.NewFriendshipService(repository.NewFriendshipRepository(db), repository.NewUserBlockRepository(db))
	app := fiber.New()
	SetupFriendRoutes(app.Group("/api"), cfg, handler.NewFriendshipHandler(svc), rdb)
	e.app = app
	return e
}

func (e *friendLiveEnv) do(who, method, path, body string) friendResp {
	e.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if who != "" {
		req.Header.Set("Authorization", "Bearer "+e.tok[who])
	}
	res, err := e.app.Test(req, -1)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := friendResp{Status: res.StatusCode, Raw: string(raw)}
	_ = json.Unmarshal(raw, &out.Body)
	return out
}

func (e *friendLiveEnv) target(name string) string {
	return fmt.Sprintf(`{"user_id":"%s"}`, e.ids[name])
}

// expect kiểm HTTP status + code lỗi (code rỗng = không kiểm).
func (e *friendLiveEnv) expect(r friendResp, status int, code, what string) {
	e.t.Helper()
	if r.Status != status || (code != "" && r.Body.Code != code) {
		e.t.Errorf("%s: muốn %d/%s, nhận %d/%s — %s", what, status, code, r.Status, r.Body.Code, r.Raw)
	}
}

func (e *friendLiveEnv) send(from, to string) string {
	e.t.Helper()
	r := e.do(from, "POST", "/api/friends/requests", e.target(to))
	e.expect(r, 201, "", "gửi lời mời "+from+"->"+to)
	var d struct{ ID string }
	_ = json.Unmarshal(r.Body.Data, &d)
	return d.ID
}

// Luồng đầy đủ của học viên và hình dạng envelope/field đúng contract-api.md §1.
func TestFriendRoutes_LuongHocVien_DungEnvelopeVaField(t *testing.T) {
	e := newFriendLiveEnv(t)

	// Gửi: 201 {message, data:{id,status:"PENDING",user:{user_id,user_name,full_name}}}.
	r := e.do("alice", "POST", "/api/friends/requests", e.target("bob"))
	e.expect(r, 201, "", "gửi lời mời")
	var sent struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		User   struct {
			UserID   string `json:"user_id"`
			UserName string `json:"user_name"`
			FullName string `json:"full_name"`
		} `json:"user"`
	}
	if err := json.Unmarshal(r.Body.Data, &sent); err != nil || sent.Status != "PENDING" || sent.User.UserID != e.ids["bob"].String() || sent.User.FullName == "" {
		t.Fatalf("data gửi lời mời sai contract: %s", r.Raw)
	}
	if strings.Contains(strings.ToLower(r.Raw), "email") {
		t.Errorf("phản hồi không được chứa email: %s", r.Raw)
	}

	// Bob thấy lời mời đến + summary.
	r = e.do("bob", "GET", "/api/friends/summary", "")
	e.expect(r, 200, "", "summary của bob")
	var sum struct{ FriendsCount, IncomingRequests, OutgoingRequests int64 }
	_ = json.Unmarshal([]byte(strings.NewReplacer("friends_count", "FriendsCount", "incoming_requests", "IncomingRequests", "outgoing_requests", "OutgoingRequests").Replace(string(r.Body.Data))), &sum)
	if sum.FriendsCount != 0 || sum.IncomingRequests != 1 || sum.OutgoingRequests != 0 {
		t.Errorf("summary bob sai: %s", r.Body.Data)
	}
	r = e.do("bob", "GET", "/api/friends/requests", "") // mặc định incoming
	e.expect(r, 200, "", "requests incoming")
	var reqs struct {
		Requests []struct {
			ID        string `json:"id"`
			Direction string `json:"direction"`
		} `json:"requests"`
		TotalCount int `json:"total_count"`
		Page       int `json:"page"`
		Limit      int `json:"limit"`
	}
	_ = json.Unmarshal(r.Body.Data, &reqs)
	if reqs.TotalCount != 1 || reqs.Page != 1 || reqs.Limit != 20 || len(reqs.Requests) != 1 || reqs.Requests[0].ID != sent.ID || reqs.Requests[0].Direction != "incoming" {
		t.Errorf("requests incoming sai: %s", r.Body.Data)
	}
	r = e.do("alice", "GET", "/api/friends/requests?direction=outgoing", "")
	e.expect(r, 200, "", "requests outgoing")
	if !strings.Contains(string(r.Body.Data), `"direction":"outgoing"`) {
		t.Errorf("requests outgoing sai: %s", r.Body.Data)
	}
	e.expect(e.do("alice", "GET", "/api/friends/requests?direction=x", ""), 400, "ERR_VALIDATION", "direction lạ")

	// Tìm kiếm trả relationship + request_id (chỉ khi đang có lời mời chờ) để web thu hồi/chấp nhận ngay.
	r = e.do("alice", "GET", "/api/friends/search?q=bob", "")
	e.expect(r, 200, "", "search")
	if !strings.Contains(r.Raw, `"relationship":"PENDING_OUT"`) || !strings.Contains(r.Raw, `"request_id":"`+sent.ID+`"`) {
		t.Errorf("search alice->bob muốn PENDING_OUT kèm request_id %s: %s", sent.ID, r.Raw)
	}
	if r := e.do("carol", "GET", "/api/friends/search?q=bob", ""); strings.Contains(r.Raw, "request_id") || !strings.Contains(r.Raw, `"relationship":"NONE"`) {
		t.Errorf("search carol->bob (không có lời mời) muốn NONE và KHÔNG có request_id: %s", r.Raw)
	}

	// Quan hệ theo từng phía.
	r = e.do("alice", "GET", "/api/friends/relationship/"+e.ids["bob"].String(), "")
	if !strings.Contains(r.Raw, `"PENDING_OUT"`) || !strings.Contains(r.Raw, sent.ID) {
		t.Errorf("relationship alice->bob muốn PENDING_OUT + request_id: %s", r.Raw)
	}
	r = e.do("bob", "GET", "/api/friends/relationship/"+e.ids["alice"].String(), "")
	if !strings.Contains(r.Raw, `"PENDING_IN"`) {
		t.Errorf("relationship bob->alice muốn PENDING_IN: %s", r.Raw)
	}
	r = e.do("alice", "GET", "/api/friends/relationship/"+e.ids["alice"].String(), "")
	if !strings.Contains(r.Raw, `"SELF"`) {
		t.Errorf("relationship với chính mình muốn SELF: %s", r.Raw)
	}

	// Chấp nhận: 200 {id,status:"ACCEPTED",user}.
	r = e.do("bob", "POST", "/api/friends/requests/"+sent.ID+"/accept", "")
	e.expect(r, 200, "", "accept")
	if !strings.Contains(r.Raw, `"ACCEPTED"`) || !strings.Contains(r.Raw, e.ids["alice"].String()) {
		t.Errorf("accept sai contract: %s", r.Raw)
	}

	// Danh sách bạn: {friends:[{friendship_id,user,since}],total_count,page,limit}.
	r = e.do("alice", "GET", "/api/friends", "")
	e.expect(r, 200, "", "danh sách bạn")
	var list struct {
		Friends []struct {
			FriendshipID string `json:"friendship_id"`
			User         struct {
				UserID string `json:"user_id"`
			} `json:"user"`
			Since string `json:"since"`
		} `json:"friends"`
		TotalCount int `json:"total_count"`
	}
	_ = json.Unmarshal(r.Body.Data, &list)
	if list.TotalCount != 1 || len(list.Friends) != 1 || list.Friends[0].User.UserID != e.ids["bob"].String() || list.Friends[0].FriendshipID == "" || list.Friends[0].Since == "" {
		t.Errorf("danh sách bạn sai contract: %s", r.Body.Data)
	}
	r = e.do("alice", "GET", "/api/friends/relationship/"+e.ids["bob"].String(), "")
	if !strings.Contains(r.Raw, `"FRIENDS"`) {
		t.Errorf("muốn FRIENDS: %s", r.Raw)
	}

	// Huỷ kết bạn: 200 data null, và route DELETE /:userId không nuốt DELETE /requests/:id.
	r = e.do("alice", "DELETE", "/api/friends/"+e.ids["bob"].String(), "")
	e.expect(r, 200, "", "huỷ kết bạn")
	if string(r.Body.Data) != "null" {
		t.Errorf("huỷ kết bạn muốn data null, nhận %s", r.Body.Data)
	}
	e.expect(e.do("alice", "DELETE", "/api/friends/"+e.ids["bob"].String(), ""), 404, "FRIEND_NOT_FOUND", "huỷ kết bạn lần hai")
}

func TestFriendRoutes_TuChoi_ThuHoi_TuChapNhan(t *testing.T) {
	e := newFriendLiveEnv(t)

	id := e.send("alice", "bob")
	r := e.do("bob", "POST", "/api/friends/requests/"+id+"/decline", "")
	e.expect(r, 200, "", "decline")
	if !strings.Contains(r.Raw, `"DECLINED"`) || !strings.Contains(r.Raw, id) {
		t.Errorf("decline muốn {id,status:DECLINED}: %s", r.Raw)
	}
	// Người bị từ chối chỉ thấy "chưa thể gửi lúc này" (mã cooldown), không có chữ "từ chối".
	r = e.do("alice", "POST", "/api/friends/requests", e.target("bob"))
	e.expect(r, 409, "FRIEND_REQUEST_COOLDOWN", "gửi lại ngay sau khi bị từ chối")
	if strings.Contains(strings.ToLower(r.Body.Message), "từ chối") {
		t.Errorf("thông điệp cooldown không được nói 'bị từ chối': %q", r.Body.Message)
	}

	cid := e.send("carol", "dave")
	r = e.do("carol", "DELETE", "/api/friends/requests/"+cid, "")
	e.expect(r, 200, "", "thu hồi")
	if string(r.Body.Data) != "null" {
		t.Errorf("thu hồi muốn data null: %s", r.Body.Data)
	}
	e.expect(e.do("dave", "POST", "/api/friends/requests/"+cid+"/accept", ""), 404, "FRIEND_REQUEST_NOT_FOUND", "accept lời mời đã thu hồi")

	// Tự chấp nhận: bob đã gửi erin, erin gửi lại bob -> 200 ACCEPTED (không phải 201).
	e.send("erin", "frank")
	r = e.do("frank", "POST", "/api/friends/requests", e.target("erin"))
	e.expect(r, 200, "", "gửi ngược chiều")
	if !strings.Contains(r.Raw, `"ACCEPTED"`) {
		t.Errorf("gửi ngược chiều muốn ACCEPTED: %s", r.Raw)
	}
}

// Mỗi endpoint × từng vai không được phép (phụ huynh, giáo viên, admin) = 403 FRIEND_ROLE_NOT_ALLOWED;
// khách = 401.
func TestFriendRoutes_VaiTro_VaKhach_MoiEndpoint(t *testing.T) {
	e := newFriendLiveEnv(t)
	uid := e.ids["bob"].String()
	rid := uuid.NewString()
	body := e.target("bob")
	endpoints := []struct{ method, path, body string }{
		{"GET", "/api/friends", ""},
		{"GET", "/api/friends/summary", ""},
		{"GET", "/api/friends/requests", ""},
		{"POST", "/api/friends/requests", body},
		{"POST", "/api/friends/requests/" + rid + "/accept", ""},
		{"POST", "/api/friends/requests/" + rid + "/decline", ""},
		{"DELETE", "/api/friends/requests/" + rid, ""},
		{"DELETE", "/api/friends/" + uid, ""},
		{"GET", "/api/friends/search?q=abc", ""},
		{"GET", "/api/friends/relationship/" + uid, ""},
		{"GET", "/api/friends/blocks", ""},
		{"POST", "/api/friends/blocks", body},
		{"DELETE", "/api/friends/blocks/" + uid, ""},
	}
	for _, ep := range endpoints {
		name := ep.method + " " + ep.path
		for _, who := range []string{"parent", "teacher", "admin"} {
			e.expect(e.do(who, ep.method, ep.path, ep.body), 403, "FRIEND_ROLE_NOT_ALLOWED", who+" gọi "+name)
		}
		e.expect(e.do("", ep.method, ep.path, ep.body), 401, "", "khách gọi "+name)
	}
}

func TestFriendRoutes_NguoiLa_KhongDuocDungLoiMoiCuaNguoiKhac(t *testing.T) {
	e := newFriendLiveEnv(t)
	id := e.send("alice", "bob")
	for _, who := range []string{"carol", "alice"} { // người lạ, và chính người gửi
		e.expect(e.do(who, "POST", "/api/friends/requests/"+id+"/accept", ""), 404, "FRIEND_REQUEST_NOT_FOUND", who+" accept")
		e.expect(e.do(who, "POST", "/api/friends/requests/"+id+"/decline", ""), 404, "FRIEND_REQUEST_NOT_FOUND", who+" decline")
	}
	for _, who := range []string{"carol", "bob"} { // người lạ, và chính người nhận
		e.expect(e.do(who, "DELETE", "/api/friends/requests/"+id, ""), 404, "FRIEND_REQUEST_NOT_FOUND", who+" thu hồi")
	}
	// Người lạ không thấy lời mời trong danh sách của mình.
	r := e.do("carol", "GET", "/api/friends/requests", "")
	if !strings.Contains(r.Raw, `"total_count":0`) {
		t.Errorf("carol không được thấy lời mời của người khác: %s", r.Raw)
	}
	// Huỷ bạn với người chưa là bạn.
	e.expect(e.do("carol", "DELETE", "/api/friends/"+e.ids["dave"].String(), ""), 404, "FRIEND_NOT_FOUND", "huỷ bạn người lạ")
}

func TestFriendRoutes_NguoiBiChan(t *testing.T) {
	e := newFriendLiveEnv(t)
	friendship := e.send("alice", "bob")
	e.expect(e.do("bob", "POST", "/api/friends/requests/"+friendship+"/accept", ""), 200, "", "kết bạn")

	r := e.do("alice", "POST", "/api/friends/blocks", e.target("bob"))
	e.expect(r, 201, "", "chặn")
	if string(r.Body.Data) != "null" {
		t.Errorf("chặn muốn data null: %s", r.Body.Data)
	}
	// Tình bạn biến mất, hai chiều đều không gửi được, cùng một mã lỗi.
	e.expect(e.do("alice", "DELETE", "/api/friends/"+e.ids["bob"].String(), ""), 404, "FRIEND_NOT_FOUND", "bạn đã bị xoá")
	e.expect(e.do("bob", "POST", "/api/friends/requests", e.target("alice")), 403, "FRIEND_REQUEST_NOT_ALLOWED", "người bị chặn gửi")
	e.expect(e.do("alice", "POST", "/api/friends/requests", e.target("bob")), 403, "FRIEND_REQUEST_NOT_ALLOWED", "người chặn gửi")
	// Người bị chặn thấy NONE (không lộ), người chặn thấy BLOCKED_BY_ME.
	if r := e.do("bob", "GET", "/api/friends/relationship/"+e.ids["alice"].String(), ""); !strings.Contains(r.Raw, `"NONE"`) {
		t.Errorf("người bị chặn muốn NONE: %s", r.Raw)
	}
	if r := e.do("alice", "GET", "/api/friends/relationship/"+e.ids["bob"].String(), ""); !strings.Contains(r.Raw, `"BLOCKED_BY_ME"`) {
		t.Errorf("người chặn muốn BLOCKED_BY_ME: %s", r.Raw)
	}
	// Ẩn khỏi tìm kiếm hai phía.
	for _, who := range []string{"alice", "bob"} {
		other := map[string]string{"alice": "bob", "bob": "alice"}[who]
		r := e.do(who, "GET", "/api/friends/search?q="+other, "")
		if !strings.Contains(r.Raw, `"users":[]`) {
			t.Errorf("%s vẫn tìm thấy %s: %s", who, other, r.Raw)
		}
	}
	r = e.do("alice", "GET", "/api/friends/blocks", "")
	if !strings.Contains(r.Raw, e.ids["bob"].String()) || !strings.Contains(r.Raw, `"total_count":1`) {
		t.Errorf("danh sách chặn sai: %s", r.Raw)
	}
	e.expect(e.do("alice", "DELETE", "/api/friends/blocks/"+e.ids["bob"].String(), ""), 200, "", "bỏ chặn")
	e.expect(e.do("bob", "POST", "/api/friends/requests", e.target("alice")), 201, "", "gửi sau khi bỏ chặn")
}

func TestFriendRoutes_LoiDauVao(t *testing.T) {
	e := newFriendLiveEnv(t)
	me := e.ids["alice"].String()
	e.expect(e.do("alice", "POST", "/api/friends/requests", fmt.Sprintf(`{"user_id":"%s"}`, me)), 400, "FRIEND_SELF_REQUEST", "tự kết bạn")
	e.expect(e.do("alice", "POST", "/api/friends/blocks", fmt.Sprintf(`{"user_id":"%s"}`, me)), 400, "FRIEND_SELF_REQUEST", "tự chặn")
	e.expect(e.do("alice", "POST", "/api/friends/requests", `{"user_id":"khong-phai-uuid"}`), 400, "ERR_INVALID_ID", "user_id sai")
	e.expect(e.do("alice", "POST", "/api/friends/requests", `{}`), 400, "ERR_INVALID_ID", "thiếu user_id")
	e.expect(e.do("alice", "POST", "/api/friends/requests", `{hỏng`), 400, "ERR_INVALID_BODY", "JSON hỏng")
	e.expect(e.do("alice", "POST", "/api/friends/requests/xyz/accept", ""), 400, "ERR_INVALID_ID", "id lời mời sai")
	e.expect(e.do("alice", "DELETE", "/api/friends/requests/xyz", ""), 400, "ERR_INVALID_ID", "id thu hồi sai")
	e.expect(e.do("alice", "DELETE", "/api/friends/xyz", ""), 400, "ERR_INVALID_ID", "userId huỷ bạn sai")
	e.expect(e.do("alice", "GET", "/api/friends/relationship/xyz", ""), 400, "ERR_INVALID_ID", "userId relationship sai")
	e.expect(e.do("alice", "POST", "/api/friends/requests", e.target("teacher")), 404, "FRIEND_USER_NOT_FOUND", "gửi cho giáo viên")
	e.expect(e.do("alice", "POST", "/api/friends/requests", fmt.Sprintf(`{"user_id":"%s"}`, uuid.New())), 404, "FRIEND_USER_NOT_FOUND", "gửi cho id không tồn tại")
	e.expect(e.do("alice", "GET", "/api/friends/search?q=ab", ""), 400, "ERR_VALIDATION", "search 2 ký tự")
	e.expect(e.do("alice", "GET", "/api/friends/search", ""), 400, "ERR_VALIDATION", "search thiếu q")

	e.send("alice", "bob")
	e.expect(e.do("alice", "POST", "/api/friends/requests", e.target("bob")), 409, "FRIEND_REQUEST_EXISTS", "gửi trùng")
	id := e.send("carol", "dave")
	e.expect(e.do("dave", "POST", "/api/friends/requests/"+id+"/accept", ""), 200, "", "accept")
	e.expect(e.do("carol", "POST", "/api/friends/requests", e.target("dave")), 409, "FRIEND_ALREADY_FRIENDS", "gửi khi đã là bạn")
	e.expect(e.do("dave", "POST", "/api/friends/requests/"+id+"/accept", ""), 409, "FRIEND_REQUEST_NOT_PENDING", "accept hai lần")
}

// Hạn mức đếm ở DB trả 429 đúng mã (không phải limiter Redis).
func TestFriendRoutes_HanMuc_429(t *testing.T) {
	e := newFriendLiveEnv(t)
	now := time.Now()
	// 20 lời mời đã gửi trong 24h qua (thu hồi rồi vẫn tính): lời mời kế tiếp bị chặn.
	for i := 0; i < constants.FriendDailyRequestLimit; i++ {
		u := model.User{Email: fmt.Sprintf("q%d-%s@40study.test", i, uuid.NewString()[:6]), PasswordHash: "x", UserName: "q" + uuid.NewString()[:8]}
		if err := e.db.Create(&u).Error; err != nil {
			t.Fatal(err)
		}
		f := model.Friendship{RequesterID: e.ids["alice"], AddresseeID: u.ID, Status: model.FriendshipStatusCancelled, RequestedAt: now, RespondedAt: &now}
		if err := e.db.Create(&f).Error; err != nil {
			t.Fatal(err)
		}
	}
	e.expect(e.do("alice", "POST", "/api/friends/requests", e.target("bob")), 429, "FRIEND_DAILY_LIMIT_REACHED", "quá 20 lời mời/24h")

	// 30 lời mời đang chờ (đã quá 24h) cho carol.
	old := now.Add(-48 * time.Hour)
	for i := 0; i < constants.FriendPendingRequestLimit; i++ {
		u := model.User{Email: fmt.Sprintf("p%d-%s@40study.test", i, uuid.NewString()[:6]), PasswordHash: "x", UserName: "p" + uuid.NewString()[:8]}
		if err := e.db.Create(&u).Error; err != nil {
			t.Fatal(err)
		}
		f := model.Friendship{RequesterID: e.ids["carol"], AddresseeID: u.ID, Status: model.FriendshipStatusPending, RequestedAt: old}
		if err := e.db.Create(&f).Error; err != nil {
			t.Fatal(err)
		}
	}
	e.expect(e.do("carol", "POST", "/api/friends/requests", e.target("bob")), 429, "FRIEND_PENDING_LIMIT_REACHED", "quá 30 lời mời đang chờ")
}

// 10 POST/phút và 30 search/phút mỗi user (Redis).
func TestFriendRoutes_GioiHanTocDo_Redis(t *testing.T) {
	e := newFriendLiveEnv(t)
	for i := 0; i < constants.FriendPostsPerMinute; i++ {
		r := e.do("alice", "POST", "/api/friends/requests", `{}`) // body sai vẫn tính vào bộ đếm POST
		e.expect(r, 400, "ERR_INVALID_ID", fmt.Sprintf("POST #%d", i+1))
	}
	e.expect(e.do("alice", "POST", "/api/friends/requests", `{}`), 429, "", "POST thứ 11 trong phút")
	e.expect(e.do("alice", "POST", "/api/friends/blocks", `{}`), 429, "", "POST blocks dùng chung bộ đếm")
	e.expect(e.do("bob", "POST", "/api/friends/requests", `{}`), 400, "", "user khác có bộ đếm riêng")

	for i := 0; i < constants.FriendSearchPerMinute; i++ {
		e.expect(e.do("carol", "GET", "/api/friends/search?q=abc", ""), 200, "", fmt.Sprintf("search #%d", i+1))
	}
	e.expect(e.do("carol", "GET", "/api/friends/search?q=abc", ""), 429, "", "search thứ 31 trong phút")
}

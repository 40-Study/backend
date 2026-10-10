package service

// Test dịch vụ nhật ký quản trị (contract C2, plan D5/D6). Phần thuần (làm sạch metadata, parse
// bộ lọc) chạy với repo giả; phần lọc/phân trang/JOIN chạy trên Postgres THẬT trong schema tạm
// riêng của test (pgtest.IsolatedSchema) nên không đụng DB dùng chung hay lane khác.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/testutil/pgtest"
)

type auditFakeRepo struct {
	repository.AuditLogRepositoryInterface
	created []*model.AuditLog
	filter  repository.AuditLogFilter
}

func (f *auditFakeRepo) Create(_ context.Context, l *model.AuditLog) error {
	f.created = append(f.created, l)
	return nil
}

func (f *auditFakeRepo) List(_ context.Context, flt repository.AuditLogFilter) ([]repository.AuditLogRow, int64, error) {
	f.filter = flt
	return nil, 0, nil
}

func TestAuditService_Record_RedactsSensitiveMetadataKeysRecursively(t *testing.T) {
	repo := &auditFakeRepo{}
	meta := map[string]any{
		"old": 1, "new": 2,
		"Password": "x", "refresh_TOKEN": "x", "clientSecret": "x", "Authorization": "Bearer x",
		"nested": map[string]any{"keep": "ok", "api_token": "x"},
	}
	err := NewAuditLogService(repo).Record(t.Context(), model.AuditEntry{
		ActorID: uuid.New(), Action: model.AuditActionUserLock, TargetType: "user", TargetID: "u1", StatusCode: 200, Metadata: meta,
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(repo.created[0].Metadata, &got); err != nil {
		t.Fatalf("metadata không phải JSON: %v", err)
	}
	want := `{"nested":{"keep":"ok"},"new":2,"old":1}`
	raw, _ := json.Marshal(got)
	if string(raw) != want {
		t.Fatalf("metadata = %s, muốn %s", raw, want)
	}
	if _, stillThere := meta["Password"]; !stillThere {
		t.Fatal("Record không được sửa map của caller")
	}
}

func TestAuditService_Record_NoMetadataStoresNull_AndRejectsInvalidEntry(t *testing.T) {
	repo := &auditFakeRepo{}
	svc := NewAuditLogService(repo)
	actor := uuid.New()
	if err := svc.Record(t.Context(), model.AuditEntry{ActorID: actor, Action: "x.y", Metadata: map[string]any{"token": "t"}}); err != nil {
		t.Fatal(err)
	}
	if len(repo.created[0].Metadata) != 0 || repo.created[0].TargetID != nil || repo.created[0].IP != nil {
		t.Fatalf("metadata/target/ip rỗng phải lưu NULL: %+v", repo.created[0])
	}
	for _, bad := range []model.AuditEntry{{Action: "x.y"}, {ActorID: actor, Action: "  "}} {
		if err := svc.Record(t.Context(), bad); !errors.Is(err, ErrAuditEntryInvalid) {
			t.Fatalf("entry %+v: err = %v, muốn ErrAuditEntryInvalid", bad, err)
		}
	}
	if len(repo.created) != 1 {
		t.Fatalf("entry sai không được tạo dòng: %d dòng", len(repo.created))
	}
}

func TestAuditService_Record_TruncatesOverlongTargetSoRowIsNotLost(t *testing.T) {
	repo := &auditFakeRepo{}
	err := NewAuditLogService(repo).Record(t.Context(), model.AuditEntry{
		ActorID: uuid.New(), Action: "x.y", TargetType: "t", TargetID: strings.Repeat("ạ", 200),
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := len([]rune(*repo.created[0].TargetID)); n != 64 {
		t.Fatalf("target_id dài %d ký tự, muốn cắt còn 64", n)
	}
}

func TestAuditService_List_InvalidFilterAndPaginationBounds(t *testing.T) {
	repo := &auditFakeRepo{}
	svc := NewAuditLogService(repo)
	for field, q := range map[string]dto.AuditLogFilterDTO{
		"actor_id": {ActorID: "not-a-uuid"},
		"from":     {From: "yesterday"},
		"to":       {To: "2026-13-45"},
	} {
		_, err := svc.List(t.Context(), q)
		var bad *InvalidAuditFilterError
		if !errors.As(err, &bad) || bad.Field != field {
			t.Errorf("%s: err = %v, muốn InvalidAuditFilterError{%s}", field, err, field)
		}
	}
	if _, err := svc.List(t.Context(), dto.AuditLogFilterDTO{From: "2026-10-05", To: "2026-10-01"}); err == nil {
		t.Error("from > to phải là INVALID_FILTER")
	}

	for _, c := range []struct{ page, size, wantPage, wantSize int }{
		{0, 0, 1, 20}, {-3, -9, 1, 20}, {4, 50, 4, 50}, {1, 100, 1, 100}, {1, 101, 1, 100}, {1, 5000, 1, 100},
	} {
		res, err := svc.List(t.Context(), dto.AuditLogFilterDTO{Page: c.page, PageSize: c.size})
		if err != nil {
			t.Fatal(err)
		}
		if res.Page != c.wantPage || res.PageSize != c.wantSize || res.Items == nil {
			t.Errorf("page=%d size=%d -> %d/%d items=%v, muốn %d/%d và items là mảng", c.page, c.size, res.Page, res.PageSize, res.Items, c.wantPage, c.wantSize)
		}
	}
}

func TestAuditService_List_DateOnlyToCoversWholeDayInVietnamTime(t *testing.T) {
	repo := &auditFakeRepo{}
	if _, err := NewAuditLogService(repo).List(t.Context(), dto.AuditLogFilterDTO{From: "2026-10-01", To: "2026-10-01"}); err != nil {
		t.Fatal(err)
	}
	f := repo.filter
	if f.From.Format(time.RFC3339) != "2026-10-01T00:00:00+07:00" {
		t.Errorf("from = %s", f.From.Format(time.RFC3339))
	}
	if f.To.Format(time.RFC3339Nano) != "2026-10-01T23:59:59.999999+07:00" {
		t.Errorf("to = %s, muốn hết ngày 01/10 giờ VN", f.To.Format(time.RFC3339Nano))
	}
}

// ---- Postgres thật ----

func auditPG(t *testing.T) (*gorm.DB, *AuditLogService) {
	t.Helper()
	db := pgtest.IsolatedSchema(t, func(db *gorm.DB) error { return db.AutoMigrate(&model.User{}, &model.AuditLog{}) })
	return db, NewAuditLogService(repository.NewAuditLogRepository(db))
}

func auditSeed(t *testing.T, db *gorm.DB, actor uuid.UUID, action, targetType, targetID string, at time.Time) {
	t.Helper()
	row := model.AuditLog{ID: uuid.New(), CreatedAt: at, ActorID: actor, Action: action, TargetType: targetType, StatusCode: 200}
	if targetID != "" {
		row.TargetID = &targetID
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("seed audit: %v", err)
	}
}

func TestAuditService_List_FiltersJoinAndOrderOnPostgres(t *testing.T) {
	db, svc := auditPG(t)
	full := "Quản Trị Viên"
	admin := model.User{Email: "admin-audit@40study.test", PasswordHash: "x", UserName: "admin1", FullName: &full}
	plain := model.User{Email: "plain-audit@40study.test", PasswordHash: "x", UserName: "plain1"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&plain).Error; err != nil {
		t.Fatal(err)
	}
	ghost := uuid.New() // actor không còn trong users
	base := time.Date(2026, 10, 3, 10, 0, 0, 0, vnZone)
	auditSeed(t, db, admin.ID, model.AuditActionUserLock, "user", "u1", base)
	auditSeed(t, db, admin.ID, model.AuditActionUserUnlock, "user", "u1", base.Add(time.Hour))
	auditSeed(t, db, plain.ID, model.AuditActionCourseApprove, "course", "c9", base.Add(48*time.Hour))
	auditSeed(t, db, ghost, model.AuditActionNotificationBroadcast, "", "", base.Add(72*time.Hour))

	all, err := svc.List(t.Context(), dto.AuditLogFilterDTO{})
	if err != nil || all.Total != 4 || len(all.Items) != 4 {
		t.Fatalf("all = %+v err=%v", all, err)
	}
	if all.Items[0].Action != model.AuditActionNotificationBroadcast || all.Items[3].Action != model.AuditActionUserLock {
		t.Fatalf("phải sắp mới nhất trước: %s ... %s", all.Items[0].Action, all.Items[3].Action)
	}
	if all.Items[0].Actor != nil || all.Items[0].TargetID != nil || all.Items[0].Metadata != nil || all.Items[0].TargetType != "" {
		t.Errorf("actor đã mất / không đích phải null: %+v", all.Items[0])
	}
	if a := all.Items[3].Actor; a == nil || a.Name != full || a.Email != admin.Email || a.ID != admin.ID {
		t.Errorf("actor = %+v, muốn tên đầy đủ + email", a)
	}
	if a := all.Items[1].Actor; a == nil || a.Name != "plain1" {
		t.Errorf("không có full_name thì dùng user_name: %+v", a)
	}

	check := func(name string, q dto.AuditLogFilterDTO, want int64) {
		t.Helper()
		res, err := svc.List(t.Context(), q)
		if err != nil || res.Total != want || int64(len(res.Items)) != want {
			t.Errorf("%s: total=%d items=%d err=%v, muốn %d", name, res.Total, len(res.Items), err, want)
		}
	}
	check("action", dto.AuditLogFilterDTO{Action: model.AuditActionUserLock}, 1)
	check("actor", dto.AuditLogFilterDTO{ActorID: admin.ID.String()}, 2)
	check("target_type", dto.AuditLogFilterDTO{TargetType: "user"}, 2)
	check("target_type+id", dto.AuditLogFilterDTO{TargetType: "course", TargetID: "c9"}, 1)
	check("from", dto.AuditLogFilterDTO{From: "2026-10-06"}, 1)
	check("from lấy cả ngày đó", dto.AuditLogFilterDTO{From: "2026-10-05"}, 2)
	check("to ngày thuần gồm cả ngày đó", dto.AuditLogFilterDTO{To: "2026-10-03"}, 2)
	check("range rfc3339", dto.AuditLogFilterDTO{From: base.Add(30 * time.Minute).Format(time.RFC3339), To: base.Add(49 * time.Hour).Format(time.RFC3339)}, 2)
	check("không khớp", dto.AuditLogFilterDTO{Action: "nope"}, 0)

	page2, err := svc.List(t.Context(), dto.AuditLogFilterDTO{Page: 2, PageSize: 3})
	if err != nil || page2.Total != 4 || len(page2.Items) != 1 || page2.Items[0].Action != model.AuditActionUserLock {
		t.Fatalf("trang 2 = %+v err=%v", page2, err)
	}
}

func TestAuditService_RecordThenListRoundTripOnPostgres(t *testing.T) {
	db, svc := auditPG(t)
	u := model.User{Email: "rt-audit@40study.test", PasswordHash: "x", UserName: "rt1"}
	if err := db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	err := svc.Record(t.Context(), model.AuditEntry{
		ActorID: u.ID, Action: model.AuditActionSettingPlatformFeeUpdate, TargetType: "setting", TargetID: "platform_fee",
		StatusCode: 200, IP: "203.0.113.7", Metadata: map[string]any{"old": "0", "new": "10", "password": "x"},
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.List(t.Context(), dto.AuditLogFilterDTO{})
	if err != nil || len(res.Items) != 1 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	it := res.Items[0]
	if it.IP == nil || *it.IP != "203.0.113.7" || it.TargetID == nil || *it.TargetID != "platform_fee" || it.Metadata["new"] != "10" {
		t.Errorf("round-trip sai: %+v", it)
	}
	if _, leaked := it.Metadata["password"]; leaked {
		t.Error("password lọt vào metadata đã lưu")
	}
}

func TestAuditService_ActionsReturnsCopyOfSSOT(t *testing.T) {
	svc := NewAuditLogService(&auditFakeRepo{})
	got := svc.Actions()
	if len(got) != len(model.AuditActions) || got[0] != model.AuditActionUserLock {
		t.Fatalf("actions = %v", got)
	}
	got[0] = "tampered"
	if model.AuditActions[0] != model.AuditActionUserLock {
		t.Fatal("Actions() lộ slice SSOT")
	}
	seen := map[string]bool{}
	for _, a := range model.AuditActions {
		if seen[a] || a == "" {
			t.Fatalf("mã hành động trùng/rỗng: %q", a)
		}
		seen[a] = true
	}
}

// L5 (review 261009): khoá nhạy cảm bị loại cả khi nằm trong phần tử của slice ([]any, []map[string]any, slice lồng
// slice, slice trong map lồng nhau). Trước đây chỉ map lồng map được duyệt nên `[]any{map{"password": ...}}` lọt qua.
func TestSanitizeAuditMetadata_WalksSlices(t *testing.T) {
	in := map[string]any{
		"keep":      1,
		"any_slice": []any{map[string]any{"password": "x", "ok": "a"}, "scalar", 7},
		"map_slice": []map[string]any{{"api_token": "t", "ok": "b"}},
		"deep":      []any{[]any{map[string]any{"client_secret": "s", "ok": "c"}}},
		"nested":    map[string]any{"items": []any{map[string]any{"authorization": "Bearer z", "ok": "d"}}},
		"strings":   []string{"a", "b"},
	}
	out := sanitizeAuditMetadata(in)

	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, leaked := range []string{"password", "api_token", "client_secret", "authorization", `"x"`, `"t"`, `"s"`, "Bearer"} {
		if strings.Contains(string(raw), leaked) {
			t.Errorf("metadata đã làm sạch còn lọt %q: %s", leaked, raw)
		}
	}
	want := `{"any_slice":[{"ok":"a"},"scalar",7],"deep":[[{"ok":"c"}]],"keep":1,"map_slice":[{"ok":"b"}],"nested":{"items":[{"ok":"d"}]},"strings":["a","b"]}`
	if string(raw) != want {
		t.Errorf("metadata làm sạch = %s\nmuốn            = %s", raw, want)
	}
	// Không sửa đầu vào (bản sao): phần tử gốc vẫn giữ khoá nhạy cảm.
	if _, still := in["any_slice"].([]any)[0].(map[string]any)["password"]; !still {
		t.Error("sanitizeAuditMetadata không được sửa map đầu vào")
	}
}

package dto

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"study.com/v1/internal/utils"
)

// Golden test của plans/261008-qa-followup-features/contract.md (C2-C4 + C6).
// Đổi hình dạng JSON ở đây = đổi contract: sửa contract.md trước, rồi DTO, rồi type web.

func marshalKeys(t *testing.T, v any) map[string]json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal object: %v (%s)", err, raw)
	}
	return m
}

func assertKeySet(t *testing.T, got map[string]json.RawMessage, want ...string) {
	t.Helper()
	have := make([]string, 0, len(got))
	for k := range got {
		have = append(have, k)
	}
	sort.Strings(have)
	sort.Strings(want)
	if !reflect.DeepEqual(have, want) {
		t.Fatalf("key set lệch contract\n got: %v\nwant: %v", have, want)
	}
}

func assertRawEquals(t *testing.T, got map[string]json.RawMessage, key, want string) {
	t.Helper()
	if string(got[key]) != want {
		t.Fatalf("%s = %s, muốn %s", key, got[key], want)
	}
}

func TestQAFollowupContract_AuditLogItem_FullAndNulls(t *testing.T) {
	id, actorID := uuid.New(), uuid.New()
	target, ip := "u-1", "10.0.0.1"
	full := AuditLogItemDTO{
		ID: id, CreatedAt: time.Date(2026, 10, 8, 1, 2, 3, 0, time.UTC),
		Actor:  &AuditActorDTO{ID: actorID, Name: "Admin", Email: "a@x.vn"},
		Action: "user.lock", TargetType: "user", TargetID: &target, StatusCode: 200, IP: &ip,
		Metadata: map[string]any{"reason": "spam"},
	}
	got := marshalKeys(t, full)
	assertKeySet(t, got, "id", "created_at", "actor", "action", "target_type", "target_id", "status_code", "ip", "metadata")
	assertRawEquals(t, got, "created_at", `"2026-10-08T01:02:03Z"`)
	assertKeySet(t, marshalKeysFromRaw(t, got["actor"]), "id", "name", "email")

	// actor/target_id/ip/metadata là null (KHÔNG bị bỏ) khi không có.
	empty := marshalKeys(t, AuditLogItemDTO{ID: id, Action: "user.lock", TargetType: "user", StatusCode: 200})
	assertKeySet(t, empty, "id", "created_at", "actor", "action", "target_type", "target_id", "status_code", "ip", "metadata")
	for _, k := range []string{"actor", "target_id", "ip", "metadata"} {
		assertRawEquals(t, empty, k, "null")
	}
}

func marshalKeysFromRaw(t *testing.T, raw json.RawMessage) map[string]json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal %s: %v", raw, err)
	}
	return m
}

func TestQAFollowupContract_AuditLogList_EmptyItemsIsArrayNotNull(t *testing.T) {
	got := marshalKeys(t, NewAuditLogListDTO(nil, 0, 1, 20))
	assertKeySet(t, got, "items", "total", "page", "page_size")
	assertRawEquals(t, got, "items", "[]")
	assertRawEquals(t, got, "total", "0")
	assertRawEquals(t, got, "page", "1")
	assertRawEquals(t, got, "page_size", "20")
}

func TestQAFollowupContract_AuditLogFilter_QueryTags(t *testing.T) {
	typ := reflect.TypeOf(AuditLogFilterDTO{})
	var tags []string
	for i := 0; i < typ.NumField(); i++ {
		tags = append(tags, typ.Field(i).Tag.Get("query"))
	}
	want := []string{"page", "page_size", "action", "actor_id", "target_type", "target_id", "from", "to"}
	if !reflect.DeepEqual(tags, want) {
		t.Fatalf("query tags = %v, muốn %v", tags, want)
	}
}

func TestQAFollowupContract_BroadcastRequest_Validation(t *testing.T) {
	ok := BroadcastRequestDTO{Title: "Bảo trì", Content: "Hệ thống bảo trì 2h", Audience: "roles", Roles: []string{"STUDENT"}}
	if errs := utils.ValidateStruct(ok); len(errs) != 0 {
		t.Fatalf("request hợp lệ bị từ chối: %+v", errs)
	}
	ok.NotificationType = "promotion"
	if errs := utils.ValidateStruct(ok); len(errs) != 0 {
		t.Fatalf("notification_type=promotion bị từ chối: %+v", errs)
	}

	cases := []struct {
		name  string
		field string
		mut   func(r *BroadcastRequestDTO)
	}{
		{"thiếu title", "title", func(r *BroadcastRequestDTO) { r.Title = "" }},
		{"title 256 ký tự", "title", func(r *BroadcastRequestDTO) { r.Title = strings.Repeat("a", 256) }},
		{"thiếu content", "content", func(r *BroadcastRequestDTO) { r.Content = "" }},
		{"content 2001 ký tự", "content", func(r *BroadcastRequestDTO) { r.Content = strings.Repeat("a", 2001) }},
		{"audience lạ", "audience", func(r *BroadcastRequestDTO) { r.Audience = "everyone" }},
		{"thiếu audience", "audience", func(r *BroadcastRequestDTO) { r.Audience = "" }},
		{"notification_type lạ", "notification_type", func(r *BroadcastRequestDTO) { r.NotificationType = "course_update" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := BroadcastRequestDTO{Title: "t", Content: "c", Audience: "all"}
			c.mut(&r)
			errs := utils.ValidateStruct(r)
			if len(errs) != 1 || errs[0].Field != c.field {
				t.Fatalf("muốn đúng 1 lỗi ở %s, được %+v", c.field, errs)
			}
		})
	}

	// Biên: title 255 và content 2000 ký tự là hợp lệ; notification_type bỏ trống = hợp lệ (mặc định system).
	edge := BroadcastRequestDTO{Title: strings.Repeat("a", 255), Content: strings.Repeat("b", 2000), Audience: "all"}
	if errs := utils.ValidateStruct(edge); len(errs) != 0 {
		t.Fatalf("biên 255/2000 bị từ chối: %+v", errs)
	}
}

func TestQAFollowupContract_BroadcastRequest_JSONKeys(t *testing.T) {
	var r BroadcastRequestDTO
	body := `{"title":"t","content":"c","audience":"roles","roles":["STUDENT"],"notification_type":"promotion"}`
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatal(err)
	}
	if r.Title != "t" || r.Content != "c" || r.Audience != "roles" || !reflect.DeepEqual(r.Roles, []string{"STUDENT"}) || r.NotificationType != "promotion" {
		t.Fatalf("decode lệch contract C3: %+v", r)
	}
	var p BroadcastPreviewRequestDTO
	if err := json.Unmarshal([]byte(`{"audience":"roles","roles":["STUDENT"]}`), &p); err != nil {
		t.Fatal(err)
	}
	if p.Audience != "roles" || !reflect.DeepEqual(p.Roles, []string{"STUDENT"}) {
		t.Fatalf("preview request decode lệch contract C3: %+v", p)
	}
	if errs := utils.ValidateStruct(BroadcastPreviewRequestDTO{Audience: "nobody"}); len(errs) != 1 || errs[0].Field != "audience" {
		t.Fatalf("preview audience lạ phải bị từ chối đúng 1 lỗi: %+v", errs)
	}
}

func TestQAFollowupContract_BroadcastResponses(t *testing.T) {
	preview := marshalKeys(t, BroadcastPreviewDTO{RecipientCount: 123})
	assertKeySet(t, preview, "recipient_count")
	assertRawEquals(t, preview, "recipient_count", "123")

	res := marshalKeys(t, BroadcastResultDTO{RecipientCount: 123, Audience: "roles", Roles: []string{"STUDENT"}, NotificationType: "system"})
	assertKeySet(t, res, "recipient_count", "audience", "roles", "notification_type")
	assertRawEquals(t, res, "roles", `["STUDENT"]`)

	// audience=all: key roles vẫn có mặt (không omitempty); lane phải truyền []string{} để ra [] thay vì null.
	all := marshalKeys(t, BroadcastResultDTO{RecipientCount: 5, Audience: "all", Roles: []string{}, NotificationType: "system"})
	assertRawEquals(t, all, "roles", "[]")
}

func TestQAFollowupContract_AdminSettings(t *testing.T) {
	fee := decimal.NewFromInt(10)

	// Chưa từng sửa: updated_at/updated_by là null (không bị bỏ).
	never := marshalKeys(t, AdminSettingsDTO{PlatformFeePercent: fee})
	assertKeySet(t, never, "platform_fee_percent", "updated_at", "updated_by")
	assertRawEquals(t, never, "updated_at", "null")
	assertRawEquals(t, never, "updated_by", "null")

	// platform_fee_percent mã hoá GIỐNG GET /admin/settings/platform-fee (decimal -> chuỗi).
	legacy, err := json.Marshal(PlatformFeeSettingResponse{PlatformFeePercent: fee})
	if err != nil {
		t.Fatal(err)
	}
	var legacyKeys map[string]json.RawMessage
	_ = json.Unmarshal(legacy, &legacyKeys)
	assertRawEquals(t, never, "platform_fee_percent", string(legacyKeys["platform_fee_percent"]))

	at := time.Date(2026, 10, 8, 4, 5, 6, 0, time.UTC)
	set := marshalKeys(t, AdminSettingsDTO{PlatformFeePercent: fee, UpdatedAt: &at, UpdatedBy: &AdminSettingsUpdaterDTO{ID: uuid.New(), Name: "Admin"}})
	assertRawEquals(t, set, "updated_at", `"2026-10-08T04:05:06Z"`)
	assertKeySet(t, marshalKeysFromRaw(t, set["updated_by"]), "id", "name")
}

// C6 / QA T10: danh sách quiz hiện question_count=0 dù có câu hỏi. Contract: question_count LUÔN có mặt
// trong mọi response quiz (kể cả 0, không omitempty) và là SỐ NGUYÊN do server đếm.
func TestQAFollowupContract_QuizResponse_QuestionCountAlwaysPresent(t *testing.T) {
	zero := marshalKeys(t, QuizResponseDTO{ID: uuid.New(), Title: "q"})
	if _, ok := zero["question_count"]; !ok {
		t.Fatal("question_count phải luôn có mặt, kể cả khi 0 (không omitempty)")
	}
	assertRawEquals(t, zero, "question_count", "0")

	three := marshalKeys(t, QuizResponseDTO{ID: uuid.New(), Title: "q", QuestionCount: 3})
	assertRawEquals(t, three, "question_count", "3")

	// QuizDetailDTO nhúng QuizResponseDTO: question_count phải nằm ở cấp trên cùng cạnh "questions".
	detail := marshalKeys(t, QuizDetailDTO{QuizResponseDTO: QuizResponseDTO{QuestionCount: 2}})
	assertRawEquals(t, detail, "question_count", "2")
	if _, ok := detail["questions"]; !ok {
		t.Fatal("detail phải có questions")
	}
}

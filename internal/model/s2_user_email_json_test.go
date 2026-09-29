package model

// Lane S2, lỗi 2: model.User được serialize thẳng ở nhiều quan hệ (Sender, Inviter, Student, Parent,
// Granter...) nên nếu Email không bị chặn JSON thì email của người này lộ cho người xem quan hệ đó.
// Bỏ `json:"-"` khỏi User.Email thì test ĐỎ.

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestS2_UserEmailKhongLotRaJSON(t *testing.T) {
	const secret = "hocvien-bi-mat@40study.test"
	u := User{Email: secret, UserName: "hv"}

	direct, err := json.Marshal(u)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(direct), secret) || strings.Contains(strings.ToLower(string(direct)), `"email"`) {
		t.Fatalf("User lộ email khi serialize trực tiếp: %s", direct)
	}

	// Qua quan hệ lồng (ví dụ ParentStudentRelation.Student) — đường lộ thực tế nhất.
	nested, err := json.Marshal(ParentStudentRelation{Student: &u})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(nested), secret) {
		t.Fatalf("email lộ qua quan hệ lồng: %s", nested)
	}
}

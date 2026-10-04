package socket

// A-15: payload relay "đang gõ" mang tên do SERVER tra, không tin user_name client gửi.
// Bỏ nhánh TypingNamer (trả client-value hoặc luôn rỗng) thì test ĐỎ.

import (
	"testing"

	"github.com/google/uuid"
)

type namerAuthorizer struct {
	guardAuthorizer
	name string
}

func (n *namerAuthorizer) TypingDisplayName(uuid.UUID) string { return n.name }

func TestTypingDisplayName(t *testing.T) {
	id := uuid.New()
	if got := typingDisplayName(&namerAuthorizer{name: "Lê Văn C"}, id); got != "Lê Văn C" {
		t.Fatalf("authorizer có tên: got %q", got)
	}
	if got := typingDisplayName(&guardAuthorizer{}, id); got != "" {
		t.Fatalf("authorizer không cài TypingNamer phải rỗng, got %q", got)
	}
}

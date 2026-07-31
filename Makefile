# Tien ich cho CI (Linux) va may co `make`.
# Windows khong co `make` san — chay thang lenh go:
#   go test ./...            (toan bo test)
#   go vet ./internal/...    (vet)
#   go build ./...           (build)

.PHONY: test test-verbose test-unit vet build run tidy check

# Chay toan bo test (khong can DB — test dung stub repo/service)
test:
	go test ./...

test-verbose:
	go test ./... -v

# Chi test cac package da co test
test-unit:
	go test ./internal/handler/ ./internal/service/

vet:
	go vet ./internal/...

build:
	go build ./...

run:
	go run ./cmd/api

tidy:
	go mod tidy

# Gate truoc khi commit/push
check: vet build test
	@echo "OK: vet + build + test deu sach"

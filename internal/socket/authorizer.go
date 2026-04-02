package socket

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"study.com/v1/internal/repository"
)

// DBChannelAuthorizer kiểm tra quyền subscribe channel bằng DB query.
// Channel format: "prefix:uuid" — ví dụ "class:550e8400-...", "course:..."
type DBChannelAuthorizer struct {
	classRepo      *repository.ClassRepository
	enrollmentRepo *repository.EnrollmentRepository
}

func NewDBChannelAuthorizer(
	classRepo *repository.ClassRepository,
	enrollmentRepo *repository.EnrollmentRepository,
) *DBChannelAuthorizer {
	return &DBChannelAuthorizer{
		classRepo:      classRepo,
		enrollmentRepo: enrollmentRepo,
	}
}

func (a *DBChannelAuthorizer) CanSubscribe(userID uuid.UUID, channel string) (bool, error) {
	prefix, resourceID, err := parseChannel(channel)
	if err != nil {
		return false, err
	}

	ctx := context.Background()

	switch prefix {
	case "class":
		return a.canSubscribeClass(ctx, userID, resourceID)
	case "course":
		return a.canSubscribeCourse(ctx, userID, resourceID)
	case "user":
		// user chỉ subscribe channel của chính mình
		return resourceID == userID, nil
	default:
		return false, fmt.Errorf("unknown channel prefix: %s", prefix)
	}
}

// canSubscribeClass — user phải là teacher hoặc student của class
func (a *DBChannelAuthorizer) canSubscribeClass(ctx context.Context, userID, classID uuid.UUID) (bool, error) {
	isTeacher, err := a.classRepo.TeacherClassExists(ctx, classID, userID)
	if err != nil {
		return false, err
	}
	if isTeacher {
		return true, nil
	}

	return a.classRepo.StudentClassExists(ctx, classID, userID)
}

// canSubscribeCourse — user phải đã enroll khóa học
func (a *DBChannelAuthorizer) canSubscribeCourse(ctx context.Context, userID, courseID uuid.UUID) (bool, error) {
	enrollment, err := a.enrollmentRepo.GetByUserAndCourse(ctx, userID, courseID)
	if err != nil {
		return false, err
	}
	return enrollment != nil, nil
}

// parseChannel tách "class:uuid" thành ("class", uuid)
func parseChannel(channel string) (string, uuid.UUID, error) {
	parts := strings.SplitN(channel, ":", 2)
	if len(parts) != 2 {
		return "", uuid.Nil, fmt.Errorf("invalid channel format: %s (expected prefix:uuid)", channel)
	}

	id, err := uuid.Parse(parts[1])
	if err != nil {
		return "", uuid.Nil, fmt.Errorf("invalid channel uuid: %s", parts[1])
	}

	return parts[0], id, nil
}

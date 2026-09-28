package seeds

import (
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/model"
)

// demoVideoBrokenURL (S-P1-1, QA 260927): URL video mẫu cũ trỏ vào bucket Google Cloud Storage
// công khai NHƯNG bị chặn (403 — xác nhận bằng curl trực tiếp, không phải lỗi tạm thời), khiến
// mọi bài giảng video demo không phát được. demoVideoURL là video mẫu công khai khác còn sống
// (MDN interactive-examples, CC0, xác nhận 200 + Content-Type video/mp4).
const demoVideoBrokenURL = "https://commondatastorage.googleapis.com/gtv-videos-bucket/sample/ForBiggerBlazes.mp4"
const demoVideoURL = "https://interactive-examples.mdn.mozilla.net/media/cc0-videos/flower.mp4"

// demoVideoDurationSeconds (A1, QA vòng 2 — N5/S-P1-1, lỗi P0): độ dài THẬT của demoVideoURL,
// đo bằng ffprobe ngày 28/09/2026 (5,055s). Trước đây content video khai Duration =
// DurationMin*60 (600–1800s) cho một video chỉ dài 5s; server dùng con số khai đó làm mẫu số nên
// xem hết video chỉ được 5/900 = 0,6% < ngưỡng 90% và KHÔNG bài video demo nào hoàn thành được.
// Làm tròn XUỐNG (5 chứ không phải 6): xem trọn 5,055s trên mẫu số 6 chỉ được 84%, lại trượt
// ngưỡng. Video này không có bản ghi video_uploads (URL ngoài hệ thống) nên server không tự đối
// chiếu được — con số ở đây phải đúng. Đổi demoVideoURL thì PHẢI đo và đổi hằng số này theo.
const demoVideoDurationSeconds = 5

// SeedDemoCourses tạo khoá học kèm chương, bài học và nội dung bài học.
// Trả về map slug -> Course để các seeder sau (enrollment) tham chiếu.
func (s *Seeder) SeedDemoCourses(
	users map[string]model.User,
	categories map[string]model.Category,
	tags map[string]model.Tag,
) (map[string]model.Course, error) {
	log.Println("Seeding demo courses...")

	// Backfill (S-P1-1): các bản ghi lesson_content ĐÃ TỒN TẠI từ lần seed trước còn giữ URL cũ —
	// `Attrs()` trong `FirstOrCreate` bên dưới chỉ áp dụng khi TẠO MỚI bản ghi, không cập nhật bản
	// ghi đã có sẵn, nên chỉ sửa hằng số là chưa đủ để DB dev hiện tại hết lỗi 403.
	if err := s.db.Model(&model.LessonContent{}).
		Where("video_url = ?", demoVideoBrokenURL).
		Update("video_url", demoVideoURL).Error; err != nil {
		return nil, fmt.Errorf("failed to backfill lesson content video_url: %w", err)
	}
	// Backfill (A1): content video demo đã seed từ trước còn khai duration sai (DurationMin*60) —
	// FirstOrCreate + Attrs không cập nhật bản ghi có sẵn. Điều kiện `duration <> ?` giữ lệnh này
	// idempotent: chạy lại không ghi gì thêm.
	if err := s.db.Model(&model.LessonContent{}).
		Where("video_url = ? AND type = ? AND duration <> ?", demoVideoURL, "video", demoVideoDurationSeconds).
		Update("duration", demoVideoDurationSeconds).Error; err != nil {
		return nil, fmt.Errorf("failed to backfill demo video duration: %w", err)
	}

	result := make(map[string]model.Course, len(demoCourses))

	for _, spec := range demoCourses {
		instructor, ok := users[spec.InstructorEmail]
		if !ok {
			return nil, fmt.Errorf("instructor %s not found for course %s", spec.InstructorEmail, spec.Slug)
		}
		category, ok := categories[spec.CategorySlug]
		if !ok {
			return nil, fmt.Errorf("category %s not found for course %s", spec.CategorySlug, spec.Slug)
		}

		course, err := s.upsertCourse(spec, instructor.ID, category.ID)
		if err != nil {
			return nil, err
		}

		if err := s.attachCourseTags(course, spec.TagNames, tags); err != nil {
			return nil, err
		}
		if err := s.seedCourseCurriculum(course.ID, spec.Sections); err != nil {
			return nil, err
		}
		if err := s.syncCourseRatingStats(&course); err != nil {
			return nil, err
		}

		result[spec.Slug] = course
	}

	log.Printf("Seeded %d courses\n", len(result))
	return result, nil
}

// upsertCourse tạo khoá học nếu chưa có (tra theo slug).
func (s *Seeder) upsertCourse(spec courseSpec, instructorID, categoryID uuid.UUID) (model.Course, error) {
	publishedAt := daysAgo(30)

	course := model.Course{
		InstructorID:      instructorID,
		CategoryID:        &categoryID,
		Title:             spec.Title,
		Slug:              spec.Slug,
		ShortDescription:  ptr(spec.ShortDescription),
		Description:       ptr(spec.Description),
		ThumbnailURL:      ptr(fmt.Sprintf("https://picsum.photos/seed/%s/800/450", spec.Slug)),
		Level:             spec.Level,
		Language:          "vi",
		Price:             money(spec.Price),
		TotalDurationMins: totalDuration(spec.Sections),
		TotalLessons:      totalLessons(spec.Sections),
		TotalStudents:     spec.TotalStudents,
		// AverageRating/TotalReviews KHÔNG ghi cứng nữa (A6, QA vòng 2): con số 318/204/... cũ không
		// khớp bảng reviews (thực tế 0 review) nên trang khoá hiện "318 đánh giá" mà danh sách rỗng.
		// Hai cột này là giá trị suy ra từ bảng reviews — syncCourseRatingStats tính lại sau upsert.
		Requirements:   pq.StringArray(spec.Requirements),
		Objectives:     pq.StringArray(spec.Objectives),
		TargetAudience: pq.StringArray(spec.TargetAudience),
		Status:         "published",
		PublishedAt:    &publishedAt,
		IsFeatured:     spec.IsFeatured,
		IsFree:         spec.IsFree,
	}

	if spec.DiscountPrice > 0 {
		discount := money(spec.DiscountPrice)
		expires := daysAhead(30)
		course.DiscountPrice = &discount
		course.DiscountExpiresAt = &expires
	}

	if err := s.db.Where("slug = ?", spec.Slug).
		Attrs(course).
		FirstOrCreate(&course).Error; err != nil {
		return model.Course{}, fmt.Errorf("failed to seed course %s: %w", spec.Slug, err)
	}
	return course, nil
}

// syncCourseRatingStats (A6, QA vòng 2) ghi average_rating/total_reviews của khoá bằng số THẬT
// tính từ bảng reviews (bỏ review đã xoá mềm — Model(&Review{}) tự thêm deleted_at IS NULL), cùng
// công thức với ReviewService.recomputeCourseRatingStats. Chạy mỗi lần seed nên sửa luôn cả DB đã
// seed số giả từ trước; chạy lại nhiều lần cho cùng kết quả (idempotent).
func (s *Seeder) syncCourseRatingStats(course *model.Course) error {
	var stats struct {
		Total int64
		Avg   float64
	}
	if err := s.db.Model(&model.Review{}).
		Where("course_id = ?", course.ID).
		Select("COUNT(*) AS total, COALESCE(AVG(rating), 0) AS avg").
		Scan(&stats).Error; err != nil {
		return fmt.Errorf("failed to compute rating stats for course %s: %w", course.Slug, err)
	}
	avg := decimal.NewFromFloat(stats.Avg).Round(2)
	if err := s.db.Model(&model.Course{}).
		Where("id = ?", course.ID).
		UpdateColumns(map[string]interface{}{
			"average_rating": avg,
			"total_reviews":  stats.Total,
		}).Error; err != nil {
		return fmt.Errorf("failed to sync rating stats for course %s: %w", course.Slug, err)
	}
	course.AverageRating = avg
	course.TotalReviews = int(stats.Total)
	return nil
}

// attachCourseTags gắn tag vào khoá học qua bảng many2many course_tags.
func (s *Seeder) attachCourseTags(course model.Course, names []string, tags map[string]model.Tag) error {
	var linked []model.Tag
	for _, name := range names {
		tag, ok := tags[name]
		if !ok {
			return fmt.Errorf("tag %s not found for course %s", name, course.Slug)
		}
		linked = append(linked, tag)
	}
	if len(linked) == 0 {
		return nil
	}
	// Append bỏ qua bản ghi trùng nhờ khoá chính tổ hợp của bảng nối.
	if err := s.db.Model(&course).Association("Tags").Append(linked); err != nil {
		return fmt.Errorf("failed to attach tags to course %s: %w", course.Slug, err)
	}
	return nil
}

// seedCourseCurriculum tạo chương -> bài học -> nội dung bài học.
func (s *Seeder) seedCourseCurriculum(courseID uuid.UUID, specs []sectionSpec) error {
	for sIdx, secSpec := range specs {
		section := model.Section{
			CourseID:     courseID,
			Title:        secSpec.Title,
			DisplayOrder: sIdx + 1,
		}
		if err := s.db.Where("course_id = ? AND title = ?", courseID, secSpec.Title).
			Attrs(section).
			FirstOrCreate(&section).Error; err != nil {
			return fmt.Errorf("failed to seed section %s: %w", secSpec.Title, err)
		}

		for lIdx, lesSpec := range secSpec.Lessons {
			lesson := model.Lesson{
				SectionID:    section.ID,
				Title:        lesSpec.Title,
				DisplayOrder: lIdx + 1,
				DurationMins: lesSpec.DurationMin,
				IsPreview:    lesSpec.IsPreview,
				IsMandatory:  true,
			}
			if err := s.db.Where("section_id = ? AND title = ?", section.ID, lesSpec.Title).
				Attrs(lesson).
				FirstOrCreate(&lesson).Error; err != nil {
				return fmt.Errorf("failed to seed lesson %s: %w", lesSpec.Title, err)
			}

			if err := s.seedLessonContent(lesson, lesSpec); err != nil {
				return err
			}
		}
	}
	return nil
}

// seedLessonContent tạo bản ghi nội dung tương ứng loại bài học.
func (s *Seeder) seedLessonContent(lesson model.Lesson, spec lessonSpec) error {
	content := model.LessonContent{
		LessonID:     lesson.ID,
		Type:         spec.ContentType,
		Title:        ptr(spec.Title),
		Duration:     spec.DurationMin * 60,
		IsMandatory:  true,
		DisplayOrder: 1,
	}

	// Chỉ nội dung video mới có URL phát; demo dùng video mẫu công khai còn sống (S-P1-1).
	// Duration của content video PHẢI là độ dài thật của file đó (A1) — không phải DurationMin.
	if spec.ContentType == "video" {
		content.VideoURL = ptr(demoVideoURL)
		content.Duration = demoVideoDurationSeconds
	}

	if err := s.db.Where("lesson_id = ?", lesson.ID).
		Attrs(content).
		FirstOrCreate(&content).Error; err != nil {
		return fmt.Errorf("failed to seed content for lesson %s: %w", spec.Title, err)
	}
	return nil
}

func totalLessons(sections []sectionSpec) int {
	n := 0
	for _, sec := range sections {
		n += len(sec.Lessons)
	}
	return n
}

func totalDuration(sections []sectionSpec) int {
	mins := 0
	for _, sec := range sections {
		for _, les := range sec.Lessons {
			mins += les.DurationMin
		}
	}
	return mins
}

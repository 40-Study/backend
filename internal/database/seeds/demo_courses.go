package seeds

import (
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"study.com/v1/internal/model"
)

// SeedDemoCourses tạo khoá học kèm chương, bài học và nội dung bài học.
// Trả về map slug -> Course để các seeder sau (enrollment) tham chiếu.
func (s *Seeder) SeedDemoCourses(
	users map[string]model.User,
	categories map[string]model.Category,
	tags map[string]model.Tag,
) (map[string]model.Course, error) {
	log.Println("Seeding demo courses...")

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
		AverageRating:     rating(spec.Rating),
		TotalReviews:      spec.TotalReviews,
		Requirements:      pq.StringArray(spec.Requirements),
		Objectives:        pq.StringArray(spec.Objectives),
		TargetAudience:    pq.StringArray(spec.TargetAudience),
		Status:            "published",
		PublishedAt:       &publishedAt,
		IsFeatured:        spec.IsFeatured,
		IsFree:            spec.IsFree,
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

	// Chỉ nội dung video mới có URL phát; demo dùng video mẫu công khai.
	if spec.ContentType == "video" {
		content.VideoURL = ptr("https://commondatastorage.googleapis.com/gtv-videos-bucket/sample/ForBiggerBlazes.mp4")
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

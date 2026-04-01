package seeds

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/lib/pq"
	"github.com/shopspring/decimal"
	"study.com/v1/internal/model"
)

type LessonSeed struct {
	Title        string `json:"title"`
	ContentType  string `json:"content_type"`
	DisplayOrder int    `json:"display_order"`
	DurationMins int    `json:"duration_minutes"`
	IsPreview    bool   `json:"is_preview"`
}

type SectionSeed struct {
	Title        string       `json:"title"`
	DisplayOrder int          `json:"display_order"`
	Lessons      []LessonSeed `json:"lessons"`
}

type CourseSeed struct {
	Title           string      `json:"title"`
	Slug            string      `json:"slug"`
	InstructorEmail string      `json:"instructor_email"`
	CategorySlug    string      `json:"category_slug"`
	ShortDesc       string      `json:"short_description"`
	Description     string      `json:"description"`
	Level           string      `json:"level"`
	Language        string      `json:"language"`
	Price           float64     `json:"price"`
	Status          string      `json:"status"`
	IsFeatured      bool        `json:"is_featured"`
	IsFree          bool        `json:"is_free"`
	Requirements    []string    `json:"requirements"`
	Objectives      []string    `json:"objectives"`
	TargetAudience  []string    `json:"target_audience"`
	Tags            []string    `json:"tags"`
	Sections        []SectionSeed `json:"sections"`
}

func (s *Seeder) SeedCourses(filePath string) error {
	log.Println("Seeding courses...")

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read courses file: %w", err)
	}

	var seeds []CourseSeed
	if err := json.Unmarshal(data, &seeds); err != nil {
		return fmt.Errorf("failed to parse courses JSON: %w", err)
	}

	// Build user email -> model map
	var users []model.User
	if err := s.db.Find(&users).Error; err != nil {
		return fmt.Errorf("failed to load users: %w", err)
	}
	userMap := make(map[string]model.User)
	for _, u := range users {
		userMap[u.Email] = u
	}

	// Build category slug -> model map
	var categories []model.Category
	if err := s.db.Find(&categories).Error; err != nil {
		return fmt.Errorf("failed to load categories: %w", err)
	}
	catMap := make(map[string]model.Category)
	for _, c := range categories {
		catMap[c.Slug] = c
	}

	// Build tag slug -> model map
	var tags []model.Tag
	if err := s.db.Find(&tags).Error; err != nil {
		return fmt.Errorf("failed to load tags: %w", err)
	}
	tagMap := make(map[string]model.Tag)
	for _, t := range tags {
		tagMap[t.Slug] = t
	}

	for _, cs := range seeds {
		instructor, ok := userMap[cs.InstructorEmail]
		if !ok {
			log.Printf("Warning: instructor %s not found, skipping course %s\n", cs.InstructorEmail, cs.Slug)
			continue
		}

		var catID *model.Category
		if cat, exists := catMap[cs.CategorySlug]; exists {
			catID = &cat
		}

		price := decimal.NewFromFloat(cs.Price)
		shortDesc := cs.ShortDesc
		desc := cs.Description
		now := time.Now()

		course := model.Course{
			InstructorID:     instructor.ID,
			Title:            cs.Title,
			Slug:             cs.Slug,
			ShortDescription: &shortDesc,
			Description:      &desc,
			Level:            cs.Level,
			Language:         cs.Language,
			Price:            price,
			Status:           cs.Status,
			IsFeatured:       cs.IsFeatured,
			IsFree:           cs.IsFree || cs.Price == 0,
			Requirements:     pq.StringArray(cs.Requirements),
			Objectives:       pq.StringArray(cs.Objectives),
			TargetAudience:   pq.StringArray(cs.TargetAudience),
			PublishedAt:      &now,
		}
		if catID != nil {
			course.CategoryID = &catID.ID
		}

		// FirstOrCreate by slug
		result := s.db.Where("slug = ?", cs.Slug).FirstOrCreate(&course)
		if result.Error != nil {
			return fmt.Errorf("failed to seed course %s: %w", cs.Slug, result.Error)
		}

		// Associate tags via many2many (replace existing)
		var courseTags []model.Tag
		for _, tagSlug := range cs.Tags {
			if tag, exists := tagMap[tagSlug]; exists {
				courseTags = append(courseTags, tag)
			}
		}
		if len(courseTags) > 0 {
			if err := s.db.Model(&course).Association("Tags").Replace(courseTags); err != nil {
				log.Printf("Warning: could not assign tags to course %s: %v\n", cs.Slug, err)
			}
		}

		// Seed sections and lessons
		totalDuration := 0
		totalLessons := 0
		for _, sec := range cs.Sections {
			section := model.Section{
				CourseID:     course.ID,
				Title:        sec.Title,
				DisplayOrder: sec.DisplayOrder,
			}
			sResult := s.db.Where("course_id = ? AND display_order = ?", course.ID, sec.DisplayOrder).
				FirstOrCreate(&section)
			if sResult.Error != nil {
				log.Printf("Warning: could not seed section %s: %v\n", sec.Title, sResult.Error)
				continue
			}

			for _, ls := range sec.Lessons {
				lesson := model.Lesson{
					SectionID:    section.ID,
					Title:        ls.Title,
					ContentType:  ls.ContentType,
					DisplayOrder: ls.DisplayOrder,
					DurationMins: ls.DurationMins,
					IsPreview:    ls.IsPreview,
					IsMandatory:  true,
				}
				lResult := s.db.Where("section_id = ? AND display_order = ?", section.ID, ls.DisplayOrder).
					FirstOrCreate(&lesson)
				if lResult.Error != nil {
					log.Printf("Warning: could not seed lesson %s: %v\n", ls.Title, lResult.Error)
					continue
				}
				totalDuration += ls.DurationMins
				totalLessons++
			}
		}

		// Update totals
		s.db.Model(&course).Updates(map[string]interface{}{
			"total_duration_minutes": totalDuration,
			"total_lessons":          totalLessons,
		})

		log.Printf("Seeded course: %s (%d sections, %d lessons)\n", cs.Title, len(cs.Sections), totalLessons)
	}

	log.Printf("Successfully seeded %d courses\n", len(seeds))
	return nil
}

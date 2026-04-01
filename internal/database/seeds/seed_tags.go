package seeds

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"study.com/v1/internal/model"
)

type TagSeed struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func (s *Seeder) SeedTags(filePath string) error {
	log.Println("Seeding tags...")

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read tags file: %w", err)
	}

	var seeds []TagSeed
	if err := json.Unmarshal(data, &seeds); err != nil {
		return fmt.Errorf("failed to parse tags JSON: %w", err)
	}

	for _, ts := range seeds {
		tag := model.Tag{
			Name: ts.Name,
			Slug: ts.Slug,
		}

		result := s.db.Where("slug = ?", ts.Slug).FirstOrCreate(&tag)
		if result.Error != nil {
			return fmt.Errorf("failed to seed tag %s: %w", ts.Slug, result.Error)
		}

		log.Printf("Seeded tag: %s\n", ts.Name)
	}

	log.Printf("Successfully seeded %d tags\n", len(seeds))
	return nil
}

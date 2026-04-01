package seeds

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"study.com/v1/internal/model"
)

type CategorySeed struct {
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	Description  string `json:"description"`
	DisplayOrder int    `json:"display_order"`
}

func (s *Seeder) SeedCategories(filePath string) error {
	log.Println("Seeding categories...")

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read categories file: %w", err)
	}

	var seeds []CategorySeed
	if err := json.Unmarshal(data, &seeds); err != nil {
		return fmt.Errorf("failed to parse categories JSON: %w", err)
	}

	for _, cs := range seeds {
		desc := cs.Description
		cat := model.Category{
			Name:         cs.Name,
			Slug:         cs.Slug,
			Description:  &desc,
			DisplayOrder: cs.DisplayOrder,
			IsActive:     true,
		}

		result := s.db.Where("slug = ?", cs.Slug).FirstOrCreate(&cat)
		if result.Error != nil {
			return fmt.Errorf("failed to seed category %s: %w", cs.Slug, result.Error)
		}

		log.Printf("Seeded category: %s\n", cs.Name)
	}

	log.Printf("Successfully seeded %d categories\n", len(seeds))
	return nil
}

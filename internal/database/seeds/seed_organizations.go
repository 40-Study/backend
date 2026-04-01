package seeds

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"study.com/v1/internal/model"
)

type OrganizationSeed struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

func (s *Seeder) SeedOrganizations(filePath string) error {
	log.Println("Seeding organizations...")

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read organizations file: %w", err)
	}

	var seeds []OrganizationSeed
	if err := json.Unmarshal(data, &seeds); err != nil {
		return fmt.Errorf("failed to parse organizations JSON: %w", err)
	}

	for _, os_ := range seeds {
		org := model.Organization{
			Name:   os_.Name,
			Status: os_.Status,
			Description: sql.NullString{
				String: os_.Description,
				Valid:  os_.Description != "",
			},
		}

		result := s.db.Where("name = ?", os_.Name).FirstOrCreate(&org)
		if result.Error != nil {
			return fmt.Errorf("failed to seed organization %s: %w", os_.Name, result.Error)
		}

		log.Printf("Seeded organization: %s\n", os_.Name)
	}

	log.Printf("Successfully seeded %d organizations\n", len(seeds))
	return nil
}

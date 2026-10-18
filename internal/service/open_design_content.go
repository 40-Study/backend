package service

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

type TeachingSlide struct {
	Title    string   `json:"title"`
	Subtitle string   `json:"subtitle"`
	Bullets  []string `json:"bullets"`
	Notes    string   `json:"notes"`
	Layout   string   `json:"layout"`
}
type TeachingQuestion struct {
	Question      string   `json:"question"`
	Options       []string `json:"options"`
	CorrectAnswer *int     `json:"correct_answer"`
	Explanation   string   `json:"explanation"`
}
type TeachingArtifact struct {
	Kind      string             `json:"kind"`
	Title     string             `json:"title"`
	Slides    []TeachingSlide    `json:"slides,omitempty"`
	Questions []TeachingQuestion `json:"questions,omitempty"`
}
type TeachingDocument struct {
	Name string `json:"name"`
	Text string `json:"text"`
}

func boundedText(s string, min, max int) bool {
	n := utf8.RuneCountInString(strings.TrimSpace(s))
	return n >= min && n <= max
}

func ValidateTeachingArtifact(a *TeachingArtifact) error {
	invalid := errors.New("Kết quả AI chưa đúng cấu trúc. Bản nháp trước được giữ nguyên; hãy thử lại với yêu cầu cụ thể hơn.")
	if a == nil || !boundedText(a.Title, 1, 160) {
		return invalid
	}
	switch a.Kind {
	case "slides":
		if len(a.Slides) < 1 || len(a.Slides) > 20 || len(a.Questions) > 0 {
			return invalid
		}
		for _, s := range a.Slides {
			if !boundedText(s.Title, 1, 140) || !boundedText(s.Subtitle, 0, 320) || !boundedText(s.Notes, 0, 3000) || len(s.Bullets) > 6 {
				return invalid
			}
			if s.Layout != "cover" && s.Layout != "content" && s.Layout != "split" && s.Layout != "summary" {
				return invalid
			}
			for _, b := range s.Bullets {
				if !boundedText(b, 1, 240) {
					return invalid
				}
			}
		}
	case "quiz":
		if len(a.Questions) < 1 || len(a.Questions) > 30 || len(a.Slides) > 0 {
			return invalid
		}
		for _, q := range a.Questions {
			if !boundedText(q.Question, 1, 1000) || len(q.Options) != 4 || q.CorrectAnswer == nil || *q.CorrectAnswer < 0 || *q.CorrectAnswer > 3 || !boundedText(q.Explanation, 1, 1600) {
				return invalid
			}
			seen := map[string]bool{}
			for _, option := range q.Options {
				key := strings.ToLower(strings.TrimSpace(option))
				if !boundedText(option, 1, 500) || seen[key] {
					return invalid
				}
				seen[key] = true
			}
		}
	default:
		return invalid
	}
	return nil
}

func parseTeachingArtifact(content, kind string) (*TeachingArtifact, error) {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		if pos := strings.Index(content, "\n"); pos >= 0 {
			content = content[pos+1:]
		}
		content = strings.TrimSpace(strings.TrimSuffix(content, "```"))
	}
	var artifact TeachingArtifact
	if json.Unmarshal([]byte(content), &artifact) != nil || artifact.Kind != kind {
		return nil, ErrDesignTruncated
	}
	if err := ValidateTeachingArtifact(&artifact); err != nil {
		return nil, err
	}
	return &artifact, nil
}

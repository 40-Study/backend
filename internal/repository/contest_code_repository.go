package repository

import (
	"context"
	"github.com/google/uuid"
	"study.com/v1/internal/model"
)

func (r *ContestRepository) CodeProblems(ctx context.Context, contestID uuid.UUID) ([]model.ContestProblem, error) {
	out := []model.ContestProblem{}
	err := r.db.WithContext(ctx).Where("contest_id = ?", contestID).Order("display_order ASC, id ASC").Find(&out).Error
	return out, err
}
func (r *ContestRepository) CodeParticipant(ctx context.Context, contestID, userID uuid.UUID) (*model.ContestParticipant, error) {
	var p model.ContestParticipant
	err := r.db.WithContext(ctx).Where("contest_id = ? AND user_id = ?", contestID, userID).First(&p).Error
	if err != nil {
		return nil, firstOrNil(err)
	}
	return &p, nil
}
func (r *ContestRepository) CodeSubmissions(ctx context.Context, contestID, userID uuid.UUID) ([]model.ContestSubmission, error) {
	out := []model.ContestSubmission{}
	err := r.db.WithContext(ctx).Where("contest_id = ? AND user_id = ?", contestID, userID).Order("created_at DESC").Limit(100).Find(&out).Error
	return out, err
}

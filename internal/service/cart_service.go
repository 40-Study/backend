package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
)

type CartServiceInterface interface {
	AddToCart(ctx context.Context, userID uuid.UUID, req dto.AddToCartDTO) (*dto.CartItemResponseDTO, error)
	GetCart(ctx context.Context, userID uuid.UUID) (*dto.CartListResponseDTO, error)
	RemoveFromCart(ctx context.Context, userID uuid.UUID, courseIDs []uuid.UUID) error
	ClearCart(ctx context.Context, userID uuid.UUID) error
	CheckCourseInCart(ctx context.Context, userID uuid.UUID, courseID uuid.UUID) (bool, error)
}

type CartService struct {
	cartRepo      repository.CartItemRepositoryInterface
	courseRepo    repository.CourseRepositoryInterface
	enrollmentRepo repository.EnrollmentRepositoryInterface
}

func NewCartService(cartRepo repository.CartItemRepositoryInterface, courseRepo repository.CourseRepositoryInterface, enrollmentRepo repository.EnrollmentRepositoryInterface) *CartService {
	return &CartService{
		cartRepo:      cartRepo,
		courseRepo:    courseRepo,
		enrollmentRepo: enrollmentRepo,
	}
}

func (s *CartService) AddToCart(ctx context.Context, userID uuid.UUID, req dto.AddToCartDTO) (*dto.CartItemResponseDTO, error) {
	// Check if course exists
	course, err := s.courseRepo.GetByID(ctx, req.CourseID)
	if err != nil {
		return nil, errors.New("course not found")
	}
	if course == nil {
		return nil, errors.New("course not found")
	}

	// Check if course is already in cart
	exists, err := s.cartRepo.Exists(ctx, userID, req.CourseID)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, errors.New("course already in cart")
	}

	// Check if user already enrolled
	enrollment, err := s.enrollmentRepo.GetByUserAndCourse(ctx, userID, req.CourseID)
	if err != nil && err.Error() != "record not found" {
		return nil, err
	}
	if enrollment != nil {
		return nil, errors.New("you are already enrolled in this course")
	}

	// Create cart item
	cartItem := &model.CartItem{
		UserID:   userID,
		CourseID: req.CourseID,
	}

	if err := s.cartRepo.Create(ctx, cartItem); err != nil {
		return nil, err
	}

	return s.toCartItemResponseDTO(cartItem, course), nil
}

func (s *CartService) GetCart(ctx context.Context, userID uuid.UUID) (*dto.CartListResponseDTO, error) {
	items, err := s.cartRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	var responseItems []dto.CartItemResponseDTO
	var totalPrice float64

	for i := range items {
		course, err := s.courseRepo.GetByID(ctx, items[i].CourseID)
		if err != nil {
			continue // Skip if course not found
		}

		itemDTO := s.toCartItemResponseDTO(&items[i], course)
		responseItems = append(responseItems, *itemDTO)

		// Calculate total price (use discount price if available)
		price := course.Price.InexactFloat64()
		if course.DiscountPrice != nil {
			price = course.DiscountPrice.InexactFloat64()
		}
		totalPrice += price
	}

	return &dto.CartListResponseDTO{
		Items:     responseItems,
		Total:     int(totalPrice),
		TotalItem: len(responseItems),
	}, nil
}

func (s *CartService) RemoveFromCart(ctx context.Context, userID uuid.UUID, courseIDs []uuid.UUID) error {
	for _, courseID := range courseIDs {
		if err := s.cartRepo.Delete(ctx, userID, courseID); err != nil {
			return err
		}
	}
	return nil
}

func (s *CartService) ClearCart(ctx context.Context, userID uuid.UUID) error {
	return s.cartRepo.DeleteByUserID(ctx, userID)
}

func (s *CartService) CheckCourseInCart(ctx context.Context, userID uuid.UUID, courseID uuid.UUID) (bool, error) {
	return s.cartRepo.Exists(ctx, userID, courseID)
}

func (s *CartService) toCartItemResponseDTO(item *model.CartItem, course *model.Course) *dto.CartItemResponseDTO {
	resp := &dto.CartItemResponseDTO{
		ID:        item.ID,
		CourseID:  item.CourseID,
		UserID:    item.UserID,
		CreatedAt: item.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}

	if course != nil {
		resp.Course = &dto.CartCourseInfoDTO{
			ID:                course.ID,
			Title:             course.Title,
			Slug:              course.Slug,
			ThumbnailURL:      getStringPointer(course.ThumbnailURL),
			Price:             course.Price.String(),
			Level:             course.Level,
			TotalDurationMins: course.TotalDurationMins,
			TotalLessons:      course.TotalLessons,
			AverageRating:     course.AverageRating.String(),
			TotalStudents:     course.TotalStudents,
		}

		if course.DiscountPrice != nil {
			resp.Course.DiscountPrice = course.DiscountPrice.String()
		}

		// Get instructor name
		if course.InstructorID != uuid.Nil && course.Instructor.FullName != nil {
			resp.Course.InstructorName = *course.Instructor.FullName
		}
	}

	return resp
}

func getStringPointer(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

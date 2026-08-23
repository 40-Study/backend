package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"study.com/v1/internal/config"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	asynq_queue "study.com/v1/internal/queue/asynq"
	"study.com/v1/internal/repository"
)

type LivestreamServiceInterface interface {
	Create(ctx context.Context, req dto.CreateLivestreamDTO) (*model.LivestreamSession, error)
	GetByID(ctx context.Context, id uuid.UUID) (*dto.LivestreamDetailDTO, error)
	GetByLessonContentID(ctx context.Context, lessonContentID uuid.UUID) (*dto.LivestreamResponseDTO, error)
	GetAll(ctx context.Context, page, pageSize int, status string, hostID *uuid.UUID) (*dto.LivestreamListDTO, error)
	Update(ctx context.Context, id uuid.UUID, req dto.UpdateLivestreamDTO) (*model.LivestreamSession, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Start(ctx context.Context, id uuid.UUID) (*model.LivestreamSession, error)
	End(ctx context.Context, id uuid.UUID) (*model.LivestreamSession, error)
	Join(ctx context.Context, sessionID uuid.UUID, req dto.JoinLivestreamDTO) (*dto.ParticipantResponseDTO, error)
	Leave(ctx context.Context, sessionID uuid.UUID, req dto.LeaveLivestreamDTO) error
	GetParticipants(ctx context.Context, sessionID uuid.UUID, page, pageSize int) ([]model.Participant, int64, error)
	MuteParticipant(ctx context.Context, sessionID, userID uuid.UUID) error
	UnmuteParticipant(ctx context.Context, sessionID, userID uuid.UUID) error
	KickParticipant(ctx context.Context, sessionID, userID uuid.UUID) error
	LockWhiteboard(ctx context.Context, sessionID uuid.UUID, locked bool) error
	StartScreenShare(ctx context.Context, sessionID, userID uuid.UUID) error
	StopScreenShare(ctx context.Context, sessionID, userID uuid.UUID) error
}

type LivestreamService struct {
	repo            repository.LivestreamRepositoryInterface
	participantRepo repository.ParticipantRepositoryInterface
	analyticsRepo   repository.AnalyticsRepositoryInterface
	redis           *redis.Client
	livekitSvc      LivekitServiceInterface
	q               *asynq_queue.Queue
	cfg             *config.Config
}

func NewLivestreamService(
	repo repository.LivestreamRepositoryInterface,
	participantRepo repository.ParticipantRepositoryInterface,
	analyticsRepo repository.AnalyticsRepositoryInterface,
	redis *redis.Client,
	livekitSvc LivekitServiceInterface,
	q *asynq_queue.Queue,
	cfg *config.Config,
) *LivestreamService {
	return &LivestreamService{
		repo:            repo,
		participantRepo: participantRepo,
		analyticsRepo:   analyticsRepo,
		redis:           redis,
		livekitSvc:      livekitSvc,
		q:               q,
		cfg:             cfg,
	}
}

func livestreamJoinedKey(sessionID uuid.UUID) string {
	return fmt.Sprintf("livestream:%s:joined", sessionID.String())
}

func (s *LivestreamService) Create(ctx context.Context, req dto.CreateLivestreamDTO) (*model.LivestreamSession, error) {
	hostID, err := uuid.Parse(req.HostID)
	if err != nil {
		return nil, errors.New("invalid host_id")
	}

	// ClassID optional
	var classIDPtr *uuid.UUID
	if req.ClassID != "" {
		classID, err := uuid.Parse(req.ClassID)
		if err != nil {
			return nil, errors.New("invalid class_id")
		}
		classIDPtr = &classID
	}

	// CourseID optional
	var courseIDPtr *uuid.UUID
	if req.CourseID != "" {
		courseID, err := uuid.Parse(req.CourseID)
		if err != nil {
			return nil, errors.New("invalid course_id")
		}
		courseIDPtr = &courseID
	}

	// LessonID optional
	var lessonIDPtr *uuid.UUID
	if req.LessonID != "" {
		lessonID, err := uuid.Parse(req.LessonID)
		if err != nil {
			return nil, errors.New("invalid lesson_id")
		}
		lessonIDPtr = &lessonID
	}

	// LessonContentID optional
	var lessonContentIDPtr *uuid.UUID
	if req.LessonContentID != "" {
		lcID, err := uuid.Parse(req.LessonContentID)
		if err != nil {
			return nil, errors.New("invalid lesson_content_id")
		}
		lessonContentIDPtr = &lcID
	}

	// Dùng SessionID làm room name trong LiveKit để đảm bảo unique và stable
	sessionID := uuid.New()
	roomName := sessionID.String()

	maxViewers := req.MaxViewers
	if maxViewers <= 0 {
		maxViewers = 100
	}

	// Default values
	platform := req.Platform
	if platform == "" {
		platform = "40study"
	}
	durationMinutes := req.DurationMinutes
	if durationMinutes <= 0 {
		durationMinutes = 60
	}
	var customLinkPtr *string
	if req.CustomLink != "" {
		customLinkPtr = &req.CustomLink
	}

	settings := model.LivestreamSettings{
		IsChatEnabled:        true,
		IsQAEnabled:          true,
		IsWhiteboardEnabled:  true,
		IsScreenShareEnabled: true,
		IsPollsEnabled:       true,
		WhiteboardLocked:     false,
	}

	session := &model.LivestreamSession{
		BaseModel:       model.BaseModel{ID: sessionID},
		Title:           req.Title,
		Description:     &req.Description,
		HostID:          hostID,
		ClassID:         classIDPtr,
		CourseID:        courseIDPtr,
		LessonID:        lessonIDPtr,
		LessonContentID: lessonContentIDPtr,
		RoomName:        roomName,
		Status:          model.LivestreamStatusScheduled,
		DurationMinutes: durationMinutes,
		Platform:        platform,
		CustomLink:      customLinkPtr,
		EnableReminder:  req.EnableReminder,
		MaxViewers:      maxViewers,
		IsRecorded:      req.EnableRecording,
		Settings:        settings,
	}

	// Parse scheduled_date + start_time into ScheduledAt
	if req.ScheduledDate != "" && req.StartTime != "" {
		dateTimeStr := req.ScheduledDate + "T" + req.StartTime + ":00"
		scheduledTime, err := time.Parse("2006-01-02T15:04:05", dateTimeStr)
		if err == nil {
			session.ScheduledAt = &scheduledTime
		}
	}

	if err := s.repo.Create(ctx, session); err != nil {
		return nil, err
	}

	// Enqueue reminder tasks sau khi lưu DB thành công
	if session.ScheduledAt != nil && session.EnableReminder {
		payload := asynq_queue.ScheduleLivestreamRemindPayload{
			SessionID:   sessionID,
			ClassID:     classIDPtr,
			Title:       req.Title,
			ScheduledAt: *session.ScheduledAt,
		}
		s.q.Enqueue(asynq_queue.TaskScheduleLivestreamRemind, payload, "notifications")
	}

	analytics := &model.LivestreamAnalytics{SessionID: session.ID}
	_ = s.analyticsRepo.Create(ctx, analytics)

	return session, nil
}

func (s *LivestreamService) GetByID(ctx context.Context, id uuid.UUID) (*dto.LivestreamDetailDTO, error) {
	session, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, nil
	}

	detail := &dto.LivestreamDetailDTO{
		LivestreamResponseDTO: s.toResponseDTO(*session),
	}
	if session.Status == model.LivestreamStatusLive {
		if room, err := s.livekitSvc.GetRoom(ctx, session.RoomName); err == nil {
			detail.ActiveParticipants = int(room.NumParticipants)
			detail.NumPublishers = int(room.NumPublishers)
			detail.ActiveRecording = room.ActiveRecording
		}
		if participants, err := s.livekitSvc.ListParticipants(ctx, session.RoomName); err == nil {
			for _, p := range participants {
				detail.Participants = append(detail.Participants, dto.LivekitParticipantDTO{
					Identity:     p.Identity,
					Name:         p.Name,
					State:        p.State.String(),
					JoinedAt:     p.JoinedAt,
					NumTracks:    len(p.Tracks),
					IsPublishing: len(p.Tracks) > 0,
				})
			}
		}
	}

	return detail, nil
}

func (s *LivestreamService) GetByLessonContentID(ctx context.Context, lessonContentID uuid.UUID) (*dto.LivestreamResponseDTO, error) {
	session, err := s.repo.GetByLessonContentID(ctx, lessonContentID)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, nil
	}
	resp := s.toResponseDTO(*session)
	return &resp, nil
}

func (s *LivestreamService) GetAll(ctx context.Context, page, pageSize int, status string, hostID *uuid.UUID) (*dto.LivestreamListDTO, error) {
	sessions, total, err := s.repo.GetAll(ctx, page, pageSize, status, hostID)
	if err != nil {
		return nil, err
	}

	var data []dto.LivestreamResponseDTO
	for _, session := range sessions {
		data = append(data, s.toResponseDTO(session))
	}

	return &dto.LivestreamListDTO{
		Data:     data,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *LivestreamService) Update(ctx context.Context, id uuid.UUID, req dto.UpdateLivestreamDTO) (*model.LivestreamSession, error) {
	session, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, errors.New("session not found")
	}

	if req.Title != nil {
		session.Title = *req.Title
	}
	if req.Description != nil {
		session.Description = req.Description
	}
	if req.MaxViewers != nil {
		session.MaxViewers = *req.MaxViewers
	}

	if err := s.repo.Update(ctx, session); err != nil {
		return nil, err
	}

	return session, nil
}

func (s *LivestreamService) Delete(ctx context.Context, id uuid.UUID) error {
	session, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if session == nil {
		return errors.New("session not found")
	}

	_ = s.livekitSvc.DeleteRoom(ctx, session.RoomName)
	return s.repo.Delete(ctx, id)
}

func (s *LivestreamService) Start(ctx context.Context, id uuid.UUID) (*model.LivestreamSession, error) {
	session, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, errors.New("session not found")
	}

	if session.Status != model.LivestreamStatusScheduled {
		return nil, errors.New("session cannot be started")
	}

	// Tạo room LiveKit khi start
	settingsJSON, _ := json.Marshal(session.Settings)
	livekitReq := dto.CreateRoomDTO{
		RoomName:        session.RoomName,
		EmptyTimeout:    3600,
		MaxParticipants: uint32(session.MaxViewers),
		Metadata:        string(settingsJSON),
	}
	if _, err := s.livekitSvc.CreateRoom(ctx, livekitReq); err != nil {
		return nil, fmt.Errorf("failed to create livekit room: %w", err)
	}

	if err := s.repo.StartSession(ctx, id); err != nil {
		// Cleanup room nếu update DB fail
		_ = s.livekitSvc.DeleteRoom(ctx, session.RoomName)
		return nil, err
	}

	return s.repo.GetByID(ctx, id)
}

func (s *LivestreamService) End(ctx context.Context, id uuid.UUID) (*model.LivestreamSession, error) {
	session, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, errors.New("session not found")
	}

	if session.Status != model.LivestreamStatusLive {
		return nil, errors.New("session is not live")
	}

	if err := s.repo.EndSession(ctx, id); err != nil {
		return nil, err
	}

	_ = s.livekitSvc.DeleteRoom(ctx, session.RoomName)
	// Cleanup Redis joined set
	s.redis.Del(ctx, livestreamJoinedKey(id))

	return s.repo.GetByID(ctx, id)
}

func (s *LivestreamService) Join(ctx context.Context, sessionID uuid.UUID, req dto.JoinLivestreamDTO) (*dto.ParticipantResponseDTO, error) {
	// check xem session có đang live không, nếu không thì không cho join
	userID, err := uuid.Parse(req.UserID)
	if err != nil {
		return nil, errors.New("invalid user_id")
	}
	// nếu đã tham gia rồi thì trả về token luôn, không tạo participant mới
	session, err := s.repo.GetByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, errors.New("session not found")
	}

	// Check if user is host - host can ALWAYS join their own session
	isHost := userID == session.HostID

	// Host can always join, others need session to be live or within early join window
	if !isHost {
		canJoinEarly := false
		if session.ScheduledAt != nil {
			earlyJoinTime := session.ScheduledAt.Add(-10 * time.Minute)
			canJoinEarly = time.Now().After(earlyJoinTime)
		}
		if session.Status != model.LivestreamStatusLive && !canJoinEarly {
			return nil, errors.New("session is not live")
		}
	}

	if session.MaxViewers > 0 {
		count, _ := s.participantRepo.CountActiveBySession(ctx, sessionID)
		if count >= session.MaxViewers {
			return nil, errors.New("session is full")
		}
	}
	// setup default role là viewer, nếu userID trùng với hostID thì set role là teacher
	role := model.ParticipantRoleViewer
	if req.Role != "" {
		role = model.ParticipantRole(req.Role)
	}
	if userID == session.HostID {
		role = model.ParticipantRoleTeacher
	}

	// check nếu đã tham gia rồi thì trả về token luôn, không tạo participant mới
	existing, _ := s.participantRepo.GetBySessionAndUser(ctx, sessionID, userID)
	if existing != nil {
		// Update role nếu cần (e.g. host re-join)
		if existing.Role != role {
			existing.Role = role
			_ = s.participantRepo.Update(ctx, existing)
		}
		res := s.toParticipantResponseDTO(existing)
		token, err := s.livekitSvc.CreateJoinToken(ctx, session.RoomName, dto.JoinTokenDTO{
			Identity: req.UserID,
			Name:     req.Name,
			IsHost:   role == model.ParticipantRoleTeacher,
		})
		if err != nil {
			return nil, err
		}
		res.Token = token
		res.ServerURL = s.cfg.LivekitURL
		res.RoomName = session.RoomName
		// Track joined user in Redis for reminder filtering
		key := livestreamJoinedKey(sessionID)
		s.redis.SAdd(ctx, key, userID.String())
		s.redis.Expire(ctx, key, 24*time.Hour)
		return res, nil
	}
	participant := &model.Participant{
		SessionID: sessionID,
		UserID:    userID,
		Role:      role,
		IsActive:  true,
	}
	if err := s.participantRepo.Create(ctx, participant); err != nil {
		return nil, err
	}

	res := s.toParticipantResponseDTO(participant)
	res.ID = participant.ID
	token, err := s.livekitSvc.CreateJoinToken(ctx, session.RoomName, dto.JoinTokenDTO{
		Identity: req.UserID,
		Name:     req.Name,
		IsHost:   role == model.ParticipantRoleTeacher,
	})
	if err != nil {
		return nil, err
	}
	res.Token = token
	res.ServerURL = s.cfg.LivekitURL
	res.RoomName = session.RoomName
	// tăng total viewers lên 1 đơn vị
	_ = s.analyticsRepo.IncrementTotalViewers(ctx, sessionID)
	s.updatePeakViewers(ctx, sessionID, session.RoomName)

	// Track joined user in Redis for reminder filtering (TTL 24h as fallback cleanup)
	key := livestreamJoinedKey(sessionID)
	s.redis.SAdd(ctx, key, userID.String())
	s.redis.Expire(ctx, key, 24*time.Hour)

	return res, nil
}

func (s *LivestreamService) Leave(ctx context.Context, sessionID uuid.UUID, req dto.LeaveLivestreamDTO) error {
	participant, err := s.participantRepo.GetBySessionAndUser(ctx, sessionID, req.UserID)
	if err != nil {
		return err
	}
	if participant == nil {
		return errors.New("participant not found")
	}

	return s.participantRepo.SetLeft(ctx, participant.ID)
}

func (s *LivestreamService) GetParticipants(ctx context.Context, sessionID uuid.UUID, page, pageSize int) ([]model.Participant, int64, error) {
	return s.participantRepo.GetBySession(ctx, sessionID, page, pageSize)
}

func (s *LivestreamService) MuteParticipant(ctx context.Context, sessionID, userID uuid.UUID) error {
	session, err := s.repo.GetByID(ctx, sessionID)
	if err != nil {
		return err
	}
	if session == nil {
		return errors.New("session not found")
	}
	if userID == session.HostID {
		return errors.New("cannot mute host")
	}

	participant, err := s.participantRepo.GetBySessionAndUser(ctx, sessionID, userID)
	if err != nil {
		return err
	}
	if participant == nil {
		return errors.New("participant not found")
	}
	updateReq := dto.UpdateParticipantDTO{
		CanPublish: ptrBool(false),
	}
	_, err = s.livekitSvc.UpdateParticipant(ctx, session.RoomName, userID.String(), updateReq)
	return err
}

func (s *LivestreamService) UnmuteParticipant(ctx context.Context, sessionID, userID uuid.UUID) error {
	session, err := s.repo.GetByID(ctx, sessionID)
	if err != nil {
		return err
	}
	if session == nil {
		return errors.New("session not found")
	}

	participant, err := s.participantRepo.GetBySessionAndUser(ctx, sessionID, userID)
	if err != nil {
		return err
	}
	if participant == nil {
		return errors.New("participant not found")
	}

	updateReq := dto.UpdateParticipantDTO{
		CanPublish: ptrBool(true),
	}
	_, err = s.livekitSvc.UpdateParticipant(ctx, session.RoomName, userID.String(), updateReq)
	return err
}

func (s *LivestreamService) KickParticipant(ctx context.Context, sessionID, userID uuid.UUID) error {
	session, err := s.repo.GetByID(ctx, sessionID)
	if err != nil {
		return err
	}
	if session == nil {
		return errors.New("session not found")
	}

	if err := s.livekitSvc.RemoveParticipant(ctx, session.RoomName, userID.String()); err != nil {
		return err
	}

	participant, _ := s.participantRepo.GetBySessionAndUser(ctx, sessionID, userID)
	if participant != nil {
		_ = s.participantRepo.SetLeft(ctx, participant.ID)
	}

	return nil
}

func (s *LivestreamService) LockWhiteboard(ctx context.Context, sessionID uuid.UUID, locked bool) error {
	session, err := s.repo.GetByID(ctx, sessionID)
	if err != nil {
		return err
	}
	if session == nil {
		return errors.New("session not found")
	}

	session.Settings.WhiteboardLocked = locked
	settingsJSON, _ := json.Marshal(session.Settings)
	updateReq := dto.UpdateRoomMetadataDTO{
		Metadata: string(settingsJSON),
	}
	_, err = s.livekitSvc.UpdateRoomMetadata(ctx, session.RoomName, updateReq)
	if err != nil {
		return err
	}

	return s.repo.Update(ctx, session)
}

func (s *LivestreamService) StartScreenShare(ctx context.Context, sessionID, userID uuid.UUID) error {
	_, err := s.repo.GetByID(ctx, sessionID)
	if err != nil {
		return err
	}
	return nil
}

func (s *LivestreamService) StopScreenShare(ctx context.Context, sessionID, userID uuid.UUID) error {
	_, err := s.repo.GetByID(ctx, sessionID)
	if err != nil {
		return err
	}
	return nil
}

func (s *LivestreamService) toResponseDTO(session model.LivestreamSession) dto.LivestreamResponseDTO {
	var startedAt, endedAt *string
	if session.StartedAt != nil {
		t := session.StartedAt.Format(time.RFC3339)
		startedAt = &t
	}
	if session.EndedAt != nil {
		t := session.EndedAt.Format(time.RFC3339)
		endedAt = &t
	}

	settingsJSON, _ := json.Marshal(session.Settings)

	var scheduledAt *string
	if session.ScheduledAt != nil {
		t := session.ScheduledAt.Format(time.RFC3339)
		scheduledAt = &t
	}

	return dto.LivestreamResponseDTO{
		ID:              session.ID,
		Title:           session.Title,
		Description:     ptrToStr(session.Description),
		HostID:          session.HostID,
		ClassID:         session.ClassID,
		CourseID:        session.CourseID,
		LessonID:        session.LessonID,
		LessonContentID: session.LessonContentID,
		RoomName:        session.RoomName,
		Status:          string(session.Status),
		StartedAt:       startedAt,
		EndedAt:         endedAt,
		ScheduledAt:     scheduledAt,
		DurationMinutes: session.DurationMinutes,
		Platform:        session.Platform,
		CustomLink:      session.CustomLink,
		EnableReminder:  session.EnableReminder,
		MaxViewers:      session.MaxViewers,
		IsRecorded:      session.IsRecorded,
		Settings:        string(settingsJSON),
		CreatedAt:       session.CreatedAt.Format(time.RFC3339),
	}
}

func (s *LivestreamService) toParticipantResponseDTO(p *model.Participant) *dto.ParticipantResponseDTO {
	var leftAt *string
	if p.LeftAt != nil {
		t := p.LeftAt.Format(time.RFC3339)
		leftAt = &t
	}
	return &dto.ParticipantResponseDTO{
		ID:        p.ID,
		SessionID: p.SessionID,
		UserID:    p.UserID,
		Role:      string(p.Role),
		JoinedAt:  p.JoinedAt.Format(time.RFC3339),
		LeftAt:    leftAt,
		IsActive:  p.IsActive,
	}
}

func (s *LivestreamService) updatePeakViewers(ctx context.Context, sessionID uuid.UUID, roomName string) {
	participants, err := s.livekitSvc.ListParticipants(ctx, roomName)
	if err != nil || len(participants) == 0 {
		return
	}
	_ = s.analyticsRepo.UpdatePeakViewers(ctx, sessionID, len(participants))
}

func ptrBool(v bool) *bool {
	return &v
}

func ptrToStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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
	// Create: hostID la nguoi goi THAT SU (lay tu access token o tang handler), khong nam trong
	// req — xem comment tai dto.CreateLivestreamDTO.
	Create(ctx context.Context, hostID uuid.UUID, req dto.CreateLivestreamDTO) (*model.LivestreamSession, error)
	// GetByID/GetParticipants (F-1, issue #58 review vong 2): userID/isAdmin de kiem thanh vien
	// phien truoc khi tra chi tiet/roster — truoc day 2 handler nay chi co AuthMiddleware, bat ky
	// user dang nhap nao cung doc duoc chi tiet + roster cua phien bat ky.
	GetByID(ctx context.Context, userID uuid.UUID, isAdmin bool, id uuid.UUID) (*dto.LivestreamDetailDTO, error)
	// GetAll: lessonContentID (N10, review vòng 2) lọc phiên theo lesson_content_id, nil = không
	// lọc. userID/isAdmin (F-1): non-admin chi thay phien minh la host/GV lop/instructor
	// khoa/hoc sinh lop — khong duoc liet ke toan he thong.
	GetAll(ctx context.Context, userID uuid.UUID, isAdmin bool, page, pageSize int, status string, hostID *uuid.UUID, lessonContentID *uuid.UUID) (*dto.LivestreamListDTO, error)
	// Update/Delete/Start/End: userID la nguoi goi THAT SU (access token), kiem quyen quan tri phien
	// truoc khi lam bat cu thu gi — xem canManageSession. isAdmin (D2, issue #58 review vong 2):
	// admin he thong quan tri duoc phien du khong phai host/GV lop.
	Update(ctx context.Context, userID uuid.UUID, isAdmin bool, id uuid.UUID, req dto.UpdateLivestreamDTO) (*model.LivestreamSession, error)
	Delete(ctx context.Context, userID uuid.UUID, isAdmin bool, id uuid.UUID) error
	Start(ctx context.Context, userID uuid.UUID, isAdmin bool, id uuid.UUID) (*model.LivestreamSession, error)
	End(ctx context.Context, userID uuid.UUID, isAdmin bool, id uuid.UUID) (*model.LivestreamSession, error)
	// Join: userID la nguoi tham gia THAT SU (access token) — xem dto.JoinLivestreamDTO. isAdmin
	// (D2/D3): admin he thong giam sat duoc phien du khong phai thanh vien lop.
	Join(ctx context.Context, sessionID, userID uuid.UUID, isAdmin bool, req dto.JoinLivestreamDTO) (*dto.ParticipantResponseDTO, error)
	// Leave: userID la nguoi roi phong THAT SU (access token), khong con nam trong DTO.
	Leave(ctx context.Context, sessionID, userID uuid.UUID) error
	GetParticipants(ctx context.Context, userID uuid.UUID, isAdmin bool, sessionID uuid.UUID, page, pageSize int) ([]model.Participant, int64, error)
	// MuteParticipant/KickParticipant: actorID la nguoi goi, targetID la doi tuong bi tac dong.
	MuteParticipant(ctx context.Context, actorID uuid.UUID, isAdmin bool, sessionID, targetID uuid.UUID) error
	KickParticipant(ctx context.Context, actorID uuid.UUID, isAdmin bool, sessionID, targetID uuid.UUID) error
	// LockWhiteboard: actorID la nguoi goi (chi host/GV lop/instructor khoa/admin moi duoc khoa bang).
	LockWhiteboard(ctx context.Context, actorID uuid.UUID, isAdmin bool, sessionID uuid.UUID, locked bool) error
	// StartScreenShare/StopScreenShare: actorID la nguoi goi (phai quan tri duoc phien), targetID
	// la nguoi duoc CAP/THU quyen publish (D3) — rong = actor tu chia se (== actorID).
	StartScreenShare(ctx context.Context, actorID uuid.UUID, isAdmin bool, sessionID, targetID uuid.UUID) error
	StopScreenShare(ctx context.Context, actorID uuid.UUID, isAdmin bool, sessionID, targetID uuid.UUID) error
	// EnsureSessionMember/EnsureSessionManage (V3-6, issue #58): loi UY QUYEN dung chung cho
	// ChatService/WhiteboardService — hai module nay cung phai tra loi "nguoi nay co quan he gi
	// voi phien khong" nhung khong nen tu lam lai phep kiem host/GV lop/instructor/hoc sinh da co
	// san o day (resolveJoinRole/canManageSession). Xem tung ham de biet muc kiem khac nhau the nao.
	EnsureSessionMember(ctx context.Context, sessionID, userID uuid.UUID) error
	EnsureSessionManage(ctx context.Context, sessionID, userID uuid.UUID) error
}

type LivestreamService struct {
	repo            repository.LivestreamRepositoryInterface
	participantRepo repository.ParticipantRepositoryInterface
	analyticsRepo   repository.AnalyticsRepositoryInterface
	classRepo       repository.ClassRepositoryInterface
	courseRepo      repository.CourseRepositoryInterface
	// enrollmentRepo (finding V3-6, issue #58): du phong cho phien KHONG gan lop (session.ClassID
	// == uuid.Nil) — xem resolveJoinRole. Khong xay ra voi schema hien tai (ClassID NOT NULL)
	// nhung giu de phong mo rong sau nay.
	enrollmentRepo repository.EnrollmentRepositoryInterface
	redis          *redis.Client
	livekitSvc     LivekitServiceInterface
	q              *asynq_queue.Queue
	cfg            *config.Config
}

func NewLivestreamService(
	repo repository.LivestreamRepositoryInterface,
	participantRepo repository.ParticipantRepositoryInterface,
	analyticsRepo repository.AnalyticsRepositoryInterface,
	classRepo repository.ClassRepositoryInterface,
	courseRepo repository.CourseRepositoryInterface,
	enrollmentRepo repository.EnrollmentRepositoryInterface,
	redis *redis.Client,
	livekitSvc LivekitServiceInterface,
	q *asynq_queue.Queue,
	cfg *config.Config,
) *LivestreamService {
	return &LivestreamService{
		repo:            repo,
		participantRepo: participantRepo,
		analyticsRepo:   analyticsRepo,
		classRepo:       classRepo,
		courseRepo:      courseRepo,
		enrollmentRepo:  enrollmentRepo,
		redis:           redis,
		livekitSvc:      livekitSvc,
		q:               q,
		cfg:             cfg,
	}
}

func livestreamJoinedKey(sessionID uuid.UUID) string {
	return fmt.Sprintf("livestream:%s:joined", sessionID.String())
}

// ErrNotSessionMember: nguoi goi khong co quan he that voi phien livestream — khong phai host,
// khong phai GV lop/instructor khoa, cung khong phai hoc sinh da enroll lop/khoa cua phien. La
// loi UY QUYEN (403), dung sentinel de tang handler phan loai bang errors.Is thay vi so chuoi.
var ErrNotSessionMember = errors.New("forbidden: not a member of this session")

// ErrCannotKickHost (V3-7, issue #58): chan da chinh host ra khoi phong — host la nguoi duy nhat
// con quyen quan tri phien, da host di thi phien khong con ai quan tri duoc nua. La loi UY QUYEN
// (403) chu khong phai loi du lieu (400).
var ErrCannotKickHost = errors.New("forbidden: cannot kick the host of this session")

// ErrParticipantKicked (F-5, issue #58 review vong 2): nguoi da bi kick khoi phien co goi lai
// POST /:id/join — truoc day khong co gi ngan lai vi quan he DB (GV lop/hoc sinh lop) khong doi,
// nen resolveJoinRole van cho qua va cap token moi.
var ErrParticipantKicked = errors.New("forbidden: kicked from this session")

// ErrWhiteboardLocked (F-4, issue #58 review vong 2): bang trang dang bi khoa va nguoi goi khong
// phai nguoi quan tri phien — dung cho SaveSnapshot (truoc day khong kiem khoa bang, hoc sinh ghi
// de duoc snapshot ca khi GV da khoa).
var ErrWhiteboardLocked = errors.New("forbidden: whiteboard is locked")

// IsForbiddenErr (finding review V3-6/V3-7, issue #58) — gom moi sentinel UY QUYEN cua nhom
// livestream/chat/whiteboard ve MOT cho, de tang handler khong phai liet ke lai tung sentinel.
func IsForbiddenErr(err error) bool {
	return errors.Is(err, ErrNotClassTeacher) ||
		errors.Is(err, ErrNotClassMember) ||
		errors.Is(err, ErrNotSessionMember) ||
		errors.Is(err, ErrCannotKickHost) ||
		errors.Is(err, ErrParticipantKicked) ||
		errors.Is(err, ErrWhiteboardLocked)
}

// ForbiddenCode (D4, issue #58 review vong 2): anh xa MOT sentinel uy quyen sang ma loi CO DINH
// ma web ghim vao (truong "message" trong envelope 403) — truoc day moi handler tra chuoi tu do
// ("Forbidden"), khien web khong phan biet duoc "khong phai thanh vien" voi "bang dang khoa" de
// hien thi dung thong bao. Dung thong nhat o moi handler livestream/chat/whiteboard.
func ForbiddenCode(err error) string {
	switch {
	case errors.Is(err, ErrParticipantKicked):
		return "KICKED"
	case errors.Is(err, ErrWhiteboardLocked):
		return "WHITEBOARD_LOCKED"
	case errors.Is(err, ErrNotSessionMember):
		return "NOT_SESSION_MEMBER"
	case errors.Is(err, ErrCannotKickHost):
		return "CANNOT_KICK_HOST"
	case errors.Is(err, ErrNotClassTeacher), errors.Is(err, ErrNotClassMember):
		return "NOT_SESSION_HOST"
	default:
		return "FORBIDDEN"
	}
}

// isClassTeacherOrInstructor — dinh nghia "giao vien lop/instructor khoa" da duoc rut ve
// class_access.go (V3-7, issue #58) vi nhom handler /lesson-contents/:id/classes cung can dung
// y nguyen phep kiem nay. Giu lai method manh nay lam lop mo mong de noi goi cua
// LivestreamService khong phai truyen repository qua lai.
func (s *LivestreamService) isClassTeacherOrInstructor(ctx context.Context, userID uuid.UUID, class *model.Class) (bool, error) {
	return classTeacherOrInstructor(ctx, s.classRepo, s.courseRepo, userID, class)
}

// canManageClass tra ErrNotClassTeacher khi userID khong duoc quan tri lop. Uy quyen cho
// ensureClassManage (class_access.go) — mot dinh nghia duy nhat cho ca livestream lan
// class-lesson-content.
func (s *LivestreamService) canManageClass(ctx context.Context, userID, classID uuid.UUID, isAdmin bool) error {
	return ensureClassManage(ctx, s.classRepo, s.courseRepo, userID, classID, isAdmin)
}

// canManageSession = host cua phien, HOAC admin he thong (D2), HOAC nguoi quan tri duoc lop cua
// phien (canManageClass). isAdmin thao tac tren phien nguoi khac duoc ghi log de audit — admin
// khong phai host van co quyen day du nhung hanh dong cua ho de lai dau vet.
func (s *LivestreamService) canManageSession(ctx context.Context, userID uuid.UUID, isAdmin bool, session *model.LivestreamSession) error {
	if session == nil {
		return errors.New("session not found")
	}
	if userID == session.HostID {
		return nil
	}
	if isAdmin {
		log.Printf("[ADMIN-ACTION] user=%s quan tri phien=%s (khong phai host=%s)", userID, session.ID, session.HostID)
		return nil
	}
	return s.canManageClass(ctx, userID, session.ClassID, false)
}

// getManageableSession tai phien va kiem quyen quan tri trong MOT buoc, de moi handler quan tri
// khong the vo tinh bo qua mot trong hai. Xem canManageSession.
func (s *LivestreamService) getManageableSession(ctx context.Context, userID uuid.UUID, isAdmin bool, sessionID uuid.UUID) (*model.LivestreamSession, error) {
	session, err := s.repo.GetByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, errors.New("session not found")
	}
	if err := s.canManageSession(ctx, userID, isAdmin, session); err != nil {
		return nil, err
	}
	return session, nil
}

func (s *LivestreamService) Create(ctx context.Context, hostID uuid.UUID, req dto.CreateLivestreamDTO) (*model.LivestreamSession, error) {
	classID, err := uuid.Parse(req.ClassID)
	if err != nil {
		return nil, errors.New("invalid class_id")
	}

	// N1/V3-6/V3-7 (issue #58): host phai la giao vien cua class_id HOAC instructor cua khoa
	// hoc chua lop do, truoc khi tao bat cu thu gi.
	if err := s.canManageClass(ctx, hostID, classID, false); err != nil {
		return nil, err
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
		ClassID:         classID,
		CourseID:        courseIDPtr,
		LessonContentID: lessonContentIDPtr,
		RoomName:        roomName,
		Status:          model.LivestreamStatusScheduled,
		MaxViewers:      maxViewers,
		IsRecorded:      req.IsRecorded,
		Settings:        settings,
	}

	// Set ScheduledAt trước khi lưu DB
	if req.ScheduledAt != "" {
		scheduledTime, err := time.Parse(time.RFC3339, req.ScheduledAt)
		if err == nil {
			session.ScheduledAt = &scheduledTime
		}
	}

	if err := s.repo.Create(ctx, session); err != nil {
		return nil, err
	}

	// Enqueue reminder tasks sau khi lưu DB thành công
	if session.ScheduledAt != nil {
		payload := asynq_queue.ScheduleLivestreamRemindPayload{
			SessionID:   sessionID,
			ClassID:     &classID,
			Title:       req.Title,
			ScheduledAt: *session.ScheduledAt,
		}
		s.q.Enqueue(asynq_queue.TaskScheduleLivestreamRemind, payload, "notifications")
	}

	analytics := &model.LivestreamAnalytics{SessionID: session.ID}
	_ = s.analyticsRepo.Create(ctx, analytics)

	return session, nil
}

// ensureMemberOfSession la phan than dung chung cho GetByID/GetParticipants/EnsureSessionMember —
// giu MOT dinh nghia "thanh vien" duy nhat (resolveJoinRole), khong lam lai o tung noi goi.
func (s *LivestreamService) ensureMemberOfSession(ctx context.Context, userID uuid.UUID, session *model.LivestreamSession) error {
	_, err := s.resolveJoinRole(ctx, userID, session)
	return err
}

func (s *LivestreamService) GetByID(ctx context.Context, userID uuid.UUID, isAdmin bool, id uuid.UUID) (*dto.LivestreamDetailDTO, error) {
	session, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, nil
	}

	// F-1 (issue #58 review vong 2): truoc day handler nay chi co AuthMiddleware — bat ky user
	// dang nhap nao cung xem duoc chi tiet (kem room_name + roster realtime tu LiveKit) cua phien
	// bat ky, ke ca phien cua lop/org khac.
	if !isAdmin {
		if err := s.ensureMemberOfSession(ctx, userID, session); err != nil {
			return nil, err
		}
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

func (s *LivestreamService) GetAll(ctx context.Context, userID uuid.UUID, isAdmin bool, page, pageSize int, status string, hostID *uuid.UUID, lessonContentID *uuid.UUID) (*dto.LivestreamListDTO, error) {
	// F-1 (issue #58 review vong 2): truoc day khong loc theo nguoi goi — bat ky user dang nhap
	// nao cung liet ke duoc TOAN BO phien cua he thong. Loc thuc su nam o tang repo (mot truy
	// van, giu dung tinh chinh xac cua phan trang).
	sessions, total, err := s.repo.GetAll(ctx, userID, isAdmin, page, pageSize, status, hostID, lessonContentID)
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

func (s *LivestreamService) Update(ctx context.Context, userID uuid.UUID, isAdmin bool, id uuid.UUID, req dto.UpdateLivestreamDTO) (*model.LivestreamSession, error) {
	session, err := s.getManageableSession(ctx, userID, isAdmin, id)
	if err != nil {
		return nil, err
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

func (s *LivestreamService) Delete(ctx context.Context, userID uuid.UUID, isAdmin bool, id uuid.UUID) error {
	session, err := s.getManageableSession(ctx, userID, isAdmin, id)
	if err != nil {
		return err
	}

	_ = s.livekitSvc.DeleteRoom(ctx, session.RoomName)
	return s.repo.Delete(ctx, id)
}

func (s *LivestreamService) Start(ctx context.Context, userID uuid.UUID, isAdmin bool, id uuid.UUID) (*model.LivestreamSession, error) {
	session, err := s.getManageableSession(ctx, userID, isAdmin, id)
	if err != nil {
		return nil, err
	}
	return s.startSession(ctx, session)
}

// StartAsSystem mo phien cho task auto-start chay NEN (asynq TaskAutoStartLivestream) — task nay
// khong co nguoi goi, khong co access token, va khong den tu HTTP: no duoc len lich tu luc tao
// phien. Actor cua no chinh la host cua phien (nguoi da duoc kiem quyen ngay tai Create), nen day
// KHONG phai duong vong quyen: khong co tham so nao den tu client. Ten ham co chu "AsSystem" de
// bat ky ai doc code cung thay ngay day la duong noi bo — KHONG GOI TU HANDLER, khong nam trong
// LivestreamServiceInterface (chi app.go giu con tro cu the *LivestreamService de goi truc tiep).
func (s *LivestreamService) StartAsSystem(ctx context.Context, id uuid.UUID) (*model.LivestreamSession, error) {
	session, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, errors.New("session not found")
	}
	return s.startSession(ctx, session)
}

// startSession la phan than dung chung cua Start/StartAsSystem (mot nguon su that duy nhat).
func (s *LivestreamService) startSession(ctx context.Context, session *model.LivestreamSession) (*model.LivestreamSession, error) {
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

	if err := s.repo.StartSession(ctx, session.ID); err != nil {
		// Cleanup room nếu update DB fail
		_ = s.livekitSvc.DeleteRoom(ctx, session.RoomName)
		return nil, err
	}

	return s.repo.GetByID(ctx, session.ID)
}

func (s *LivestreamService) End(ctx context.Context, userID uuid.UUID, isAdmin bool, id uuid.UUID) (*model.LivestreamSession, error) {
	session, err := s.getManageableSession(ctx, userID, isAdmin, id)
	if err != nil {
		return nil, err
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

// resolveJoinRole (finding review V3-6, issue #58; sua D1 vong 2) quyet dinh vai tro cua nguoi
// tham gia phien, dua tren QUAN HE THAT trong DB — khong bao gio dua tren gia tri client gui len:
//
//	teacher  : host phien, GV cua lop, hoac instructor cua khoa chua lop
//	student  : hoc sinh da enroll CHINH LOP cua phien
//	ErrNotSessionMember: khong co quan he nao o tren -> khong duoc vao phong
//
// D1 (issue #58 review vong 2, sua F-6): truoc day co them nhanh du phong enroll theo KHOA
// (Enrollment.GetByUserAndCourse) khi khong phai hoc sinh CUA LOP — khien hoc sinh lop B (cung
// khoa) join duoc phien cua lop A, doc/ghi chat va bang trang cua lop A. Phien luon gan mot lop
// cu the (session.ClassID NOT NULL trong model) nen fallback do khong con dieu kien de kich hoat
// dung: fallback Enrollment CHI con y nghia neu mot phien nao do KHONG gan lop
// (session.ClassID == uuid.Nil) — khong xay ra voi schema hien tai, giu lai de phong mo rong.
func (s *LivestreamService) resolveJoinRole(ctx context.Context, userID uuid.UUID, session *model.LivestreamSession) (model.ParticipantRole, error) {
	if userID == session.HostID {
		return model.ParticipantRoleTeacher, nil
	}

	if session.ClassID == uuid.Nil {
		if session.CourseID == nil {
			return "", ErrNotSessionMember
		}
		enrollment, err := s.enrollmentRepo.GetByUserAndCourse(ctx, userID, *session.CourseID)
		if err != nil {
			return "", fmt.Errorf("failed to verify course enrollment: %w", err)
		}
		if enrollment == nil {
			return "", ErrNotSessionMember
		}
		return model.ParticipantRoleStudent, nil
	}

	class, err := s.classRepo.GetByID(ctx, session.ClassID)
	if err != nil {
		return "", fmt.Errorf("failed to load class: %w", err)
	}
	if class == nil {
		return "", errors.New("class not found")
	}

	isTeacher, err := s.isClassTeacherOrInstructor(ctx, userID, class)
	if err != nil {
		return "", err
	}
	if isTeacher {
		return model.ParticipantRoleTeacher, nil
	}

	isStudent, err := s.classRepo.StudentClassExists(ctx, session.ClassID, userID)
	if err != nil {
		return "", fmt.Errorf("failed to verify class enrollment: %w", err)
	}
	if isStudent {
		return model.ParticipantRoleStudent, nil
	}

	return "", ErrNotSessionMember
}

// EnsureSessionMember (V3-6, issue #58; toi uu F-8 vong 2): tra ErrNotSessionMember neu userID
// khong co quan he gi voi phien — dung cho doc/ghi chat va bang trong mot phien (chi thanh vien
// phien do moi duoc tham gia). Day la duong NONG (chay tren MOI tin nhan chat, MOI su kien bang
// trang), nen chi tra loi CO/KHONG (khong can vai tro chinh xac nhu resolveJoinRole) qua MOT truy
// van gop IsUserRelatedToClass — tong cong 2 truy van (session + membership), thay vi toi da 4
// truy van rieng le cua duong Join.
func (s *LivestreamService) EnsureSessionMember(ctx context.Context, sessionID, userID uuid.UUID) error {
	session, err := s.repo.GetByID(ctx, sessionID)
	if err != nil {
		return err
	}
	if session == nil {
		return errors.New("session not found")
	}
	if userID == session.HostID {
		return nil
	}

	// R2-2 (issue #58 review vong 3, BLOCKER): F-5 (vong 2) chi chan nguoi bi kick o duong Join —
	// EnsureSessionMember la cong gac cua chat gui/doc VA bang trang doc/ghi/broadcast, khong kiem
	// IsKicked, nen nguoi bi kick van gui chat va ghi bang binh thuong qua REST du da bi ngat khoi
	// LiveKit. Kiem TRUOC ca quan he lop — day la ly do tu choi cu the hon "khong phai thanh vien"
	// (ho VAN la thanh vien lop, chi la da bi kick khoi buoi hoc nay). Lam tang tu 2 len 3 truy
	// van cho duong khong phai host (session + participant + membership) — danh doi chap nhan
	// duoc vi day la loi CHAN MERGE, F-8 chi la toi uu "cang re cang tot" o vong truoc.
	participant, err := s.participantRepo.GetBySessionAndUser(ctx, sessionID, userID)
	if err != nil {
		return fmt.Errorf("failed to verify participant state: %w", err)
	}
	if participant != nil && participant.IsKicked {
		return ErrParticipantKicked
	}

	if session.ClassID == uuid.Nil {
		if session.CourseID == nil {
			return ErrNotSessionMember
		}
		enrollment, err := s.enrollmentRepo.GetByUserAndCourse(ctx, userID, *session.CourseID)
		if err != nil {
			return fmt.Errorf("failed to verify course enrollment: %w", err)
		}
		if enrollment == nil {
			return ErrNotSessionMember
		}
		return nil
	}

	related, err := s.classRepo.IsUserRelatedToClass(ctx, session.ClassID, userID)
	if err != nil {
		return fmt.Errorf("failed to verify session membership: %w", err)
	}
	if !related {
		return ErrNotSessionMember
	}
	return nil
}

// EnsureSessionManage (V3-6, issue #58): uy quyen cho canManageSession (host/GV lop/instructor
// khoa) — dung cho thao tac kiem duyet cua ChatService (ghim/xoa tin nhan cua NGUOI KHAC). Khong
// co nhanh admin (D2 chi ap dung cho quan tri PHIEN LIVE — End/Kick/Mute/khoa bang — khong mo
// rong sang kiem duyet chat).
func (s *LivestreamService) EnsureSessionManage(ctx context.Context, sessionID, userID uuid.UUID) error {
	_, err := s.getManageableSession(ctx, userID, false, sessionID)
	return err
}

// participantGrant (D3, issue #58 review vong 2) tinh 3 quyen LiveKit tu VAI TRO da duoc server
// suy ra (khong bao gio tu client): giao vien/host/admin duoc publish AV day du; hoc sinh MAC
// DINH khong publish AV (CanPublish=false) — chi duoc khi host duyet chia se man hinh
// (StartScreenShare cap rieng qua UpdateParticipant). CanPublishData LUON true tru khi bang trang
// dang khoa VA nguoi nay khong phai nguoi quan tri — web ve bang trang va gui share_request qua
// data channel topic "whiteboard" (VideoTab.tsx), nen khong the tat hoan toan.
func participantGrant(role model.ParticipantRole, whiteboardLocked bool) (canPublish, canSubscribe, canPublishData bool) {
	isManager := role == model.ParticipantRoleTeacher
	canPublish = isManager
	canSubscribe = true
	canPublishData = isManager || !whiteboardLocked
	return
}

func (s *LivestreamService) Join(ctx context.Context, sessionID, userID uuid.UUID, isAdmin bool, req dto.JoinLivestreamDTO) (*dto.ParticipantResponseDTO, error) {
	// V3-6 (issue #58): userID la nguoi goi THAT SU (access token), khong con lay tu body.
	session, err := s.repo.GetByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, errors.New("session not found")
	}

	// Check if user is host - host can join 30 min early, regular users 10 min early
	isHost := userID == session.HostID
	canJoinEarly := false
	if session.ScheduledAt != nil {
		var earlyMinutes int
		if isHost {
			earlyMinutes = 30
		} else {
			earlyMinutes = 10
		}
		earlyJoinTime := session.ScheduledAt.Add(-time.Duration(earlyMinutes) * time.Minute)
		canJoinEarly = time.Now().After(earlyJoinTime)
	}

	if session.Status != model.LivestreamStatusLive && !canJoinEarly {
		return nil, errors.New("session is not live")
	}

	if session.MaxViewers > 0 {
		count, _ := s.participantRepo.CountActiveBySession(ctx, sessionID)
		if count >= session.MaxViewers {
			return nil, errors.New("session is full")
		}
	}

	// F-5 (issue #58 review vong 2): nguoi da bi kick khoi PHIEN NAY khong duoc vao lai — kiem
	// TRUOC resolveJoinRole vi day la ly do tu choi cu the hon "khong phai thanh vien" (ho VAN la
	// hoc sinh cua lop, chi la da bi kick khoi buoi hoc nay).
	existing, err := s.participantRepo.GetBySessionAndUser(ctx, sessionID, userID)
	if err != nil {
		return nil, err
	}
	if existing != nil && existing.IsKicked {
		return nil, ErrParticipantKicked
	}

	// V3-6 (issue #58): chi nguoi co quan he that voi phien moi vao duoc, va vai tro do SERVER
	// suy ra tu quan he do (khong nhan tu body). Dat sau cac buoc kiem trang thai/suc chua de giu
	// nguyen thu tu thong bao loi cu cho nguoi dung hop le, va truoc moi thao tac ghi.
	role, err := s.resolveJoinRole(ctx, userID, session)
	if err != nil {
		if !isAdmin {
			return nil, err
		}
		// D2/D3 (issue #58 review vong 2): admin he thong giam sat duoc phien du khong phai
		// thanh vien lop — cap quyen tuong duong giao vien (chi de xem/quan tri, khong phai vi
		// admin "hoc" lop nay).
		role = model.ParticipantRoleTeacher
		log.Printf("[ADMIN-ACTION] user=%s tham gia phien=%s voi tu cach giam sat (khong phai thanh vien lop)", userID, sessionID)
	}

	canPublish, canSubscribe, canPublishData := participantGrant(role, session.Settings.WhiteboardLocked)

	// check nếu đã tham gia rồi thì trả về token luôn, không tạo participant mới
	if existing != nil {
		// Update role nếu cần (e.g. host re-join)
		if existing.Role != role {
			existing.Role = role
			_ = s.participantRepo.Update(ctx, existing)
		}
		res := s.toParticipantResponseDTO(existing)
		// V3-6 (issue #58): Identity PHAI la userID that (da xac thuc tu access token). Truoc day
		// lay req.UserID tu body nen token mang danh tinh cua nguoi khac neu client khai vay.
		token, err := s.livekitSvc.CreateJoinToken(ctx, session.RoomName, dto.JoinTokenDTO{
			Identity:       userID.String(),
			Name:           req.Name,
			IsHost:         role == model.ParticipantRoleTeacher,
			CanPublish:     ptrBool(canPublish),
			CanSubscribe:   ptrBool(canSubscribe),
			CanPublishData: ptrBool(canPublishData),
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
		Identity:       userID.String(),
		Name:           req.Name,
		IsHost:         role == model.ParticipantRoleTeacher,
		CanPublish:     ptrBool(canPublish),
		CanSubscribe:   ptrBool(canSubscribe),
		CanPublishData: ptrBool(canPublishData),
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

func (s *LivestreamService) Leave(ctx context.Context, sessionID, userID uuid.UUID) error {
	// V3-6 (issue #58): userID la nguoi goi THAT SU (access token), khong con lay tu body — truoc
	// day bat ky ai cung co the da MOT NGUOI KHAC ra khoi phong bang cach khai user_id cua ho.
	participant, err := s.participantRepo.GetBySessionAndUser(ctx, sessionID, userID)
	if err != nil {
		return err
	}
	if participant == nil {
		return errors.New("participant not found")
	}

	return s.participantRepo.SetLeft(ctx, participant.ID)
}

func (s *LivestreamService) GetParticipants(ctx context.Context, userID uuid.UUID, isAdmin bool, sessionID uuid.UUID, page, pageSize int) ([]model.Participant, int64, error) {
	// F-1 (issue #58 review vong 2): truoc day khong kiem gi — bat ky user dang nhap nao cung
	// doc duoc roster (user_id, role, joined_at) cua phien bat ky.
	if !isAdmin {
		session, err := s.repo.GetByID(ctx, sessionID)
		if err != nil {
			return nil, 0, err
		}
		if session == nil {
			return nil, 0, errors.New("session not found")
		}
		if err := s.ensureMemberOfSession(ctx, userID, session); err != nil {
			return nil, 0, err
		}
	}
	return s.participantRepo.GetBySession(ctx, sessionID, page, pageSize)
}

func (s *LivestreamService) MuteParticipant(ctx context.Context, actorID uuid.UUID, isAdmin bool, sessionID, targetID uuid.UUID) error {
	// V3-6 (issue #58): mute la thao tac quan tri — actorID (nguoi goi) phai la host/GV lop/
	// instructor khoa/admin; targetID la nguoi bi mute.
	session, err := s.getManageableSession(ctx, actorID, isAdmin, sessionID)
	if err != nil {
		return err
	}

	participant, err := s.participantRepo.GetBySessionAndUser(ctx, sessionID, targetID)
	if err != nil {
		return err
	}
	if participant == nil {
		return errors.New("participant not found")
	}

	// R2-1 (issue #58 review vong 3): LiveKit THAY THE toan bo Permission moi lan
	// UpdateParticipant duoc goi voi Permission khac nil — chi truyen mot minh CanPublish se pho
	// mac CanSubscribe/CanPublishData cho gia tri mac dinh cua livekit_service, co the vo tinh MO
	// LAI kenh du lieu (CanPublishData) neu bang trang dang bi khoa dung luc nguoi nay bi mute.
	// Tu tinh lai ca bo ba theo dung vai tro/trang thai khoa bang HIEN TAI roi ghi de rieng
	// CanPublish=false — dung participant.Role da luu trong DB lam phuong an du phong neu
	// resolveJoinRole loi (vd loi tai lop).
	role, roleErr := s.resolveJoinRole(ctx, targetID, session)
	if roleErr != nil {
		role = participant.Role
	}
	_, canSubscribe, canPublishData := participantGrant(role, session.Settings.WhiteboardLocked)
	updateReq := dto.UpdateParticipantDTO{
		CanPublish:     ptrBool(false),
		CanSubscribe:   ptrBool(canSubscribe),
		CanPublishData: ptrBool(canPublishData),
	}
	_, err = s.livekitSvc.UpdateParticipant(ctx, session.RoomName, targetID.String(), updateReq)
	return err
}

func (s *LivestreamService) KickParticipant(ctx context.Context, actorID uuid.UUID, isAdmin bool, sessionID, targetID uuid.UUID) error {
	// V3-6 (issue #58): kick la thao tac quan tri — actorID (nguoi goi) phai la host/GV lop/
	// instructor khoa/admin; targetID la nguoi bi da ra.
	session, err := s.getManageableSession(ctx, actorID, isAdmin, sessionID)
	if err != nil {
		return err
	}

	// V3-7 (issue #58): chan tu-da HOST ra khoi phong cua chinh minh.
	if targetID == session.HostID {
		return ErrCannotKickHost
	}

	if err := s.livekitSvc.RemoveParticipant(ctx, session.RoomName, targetID.String()); err != nil {
		return err
	}

	// F-5 (issue #58 review vong 2): ghi BEN trang thai bi kick — truoc day chi goi
	// RemoveParticipant (ngat ket noi realtime), khong doi gi trong DB, nen nguoi bi kick goi lai
	// POST /:id/join la vao lai duoc ngay (resolveJoinRole van thay dung quan he lop).
	participant, _ := s.participantRepo.GetBySessionAndUser(ctx, sessionID, targetID)
	if participant != nil {
		_ = s.participantRepo.MarkKicked(ctx, participant.ID)
	}

	return nil
}

func (s *LivestreamService) LockWhiteboard(ctx context.Context, actorID uuid.UUID, isAdmin bool, sessionID uuid.UUID, locked bool) error {
	// V3-6 (issue #58): khoa/mo bang la thao tac quan tri.
	session, err := s.getManageableSession(ctx, actorID, isAdmin, sessionID)
	if err != nil {
		return err
	}

	session.Settings.WhiteboardLocked = locked
	settingsJSON, _ := json.Marshal(session.Settings)
	updateReq := dto.UpdateRoomMetadataDTO{
		Metadata: string(settingsJSON),
	}
	if _, err := s.livekitSvc.UpdateRoomMetadata(ctx, session.RoomName, updateReq); err != nil {
		return err
	}

	if err := s.repo.Update(ctx, session); err != nil {
		return err
	}

	// D3 (issue #58 review vong 2): khoa/mo bang phai tat/bat lai CanPublishData cua MOI
	// participant khong phai nguoi quan tri dang trong phong — truoc day chi doi Settings (anh
	// huong SaveSnapshot qua EnsureSessionMember/F-4) va broadcast qua data channel (van bi khoa
	// o BroadcastEvent), nhung KHONG doi grant LiveKit cua ai — hoc sinh van publish thang len
	// topic "whiteboard" qua LiveKit duoc, di vong hoan toan qua server (F-2).
	//
	// R2-1/R2-11 (issue #58 review vong 3): truoc day vong lap nay LUON dat CanPublish=false cho
	// MOI non-manager, bat ke ho co dang duoc duyet chia se man hinh hay khong (R2-11) — vo tinh
	// thu lai quyen do moi lan GV khoa/mo bang. Va vi khong truyen CanSubscribe, no bi LiveKit dat
	// ve false theo mac dinh cu (R2-1, BLOCKER) — CA LOP mat kha nang nhan hinh/tieng khi bang bi
	// khoa hoac mo. Sua: doc lai CanPublish/CanPublishSources HIEN TAI cua tung participant tu
	// chinh ket qua ListParticipants (p.Permission), truyen nguyen ven lai, CHI doi CanPublishData
	// theo trang thai khoa — khong con dua doan nao vao gia tri mac dinh nua.
	participants, err := s.livekitSvc.ListParticipants(ctx, session.RoomName)
	if err != nil {
		// Khong chan thao tac khoa bang chi vi khong lay duoc danh sach realtime — Settings va
		// SaveSnapshot van duoc bao ve du LiveKit tam thoi khong dong bo duoc.
		return nil
	}
	for _, p := range participants {
		identity, parseErr := uuid.Parse(p.Identity)
		if parseErr != nil {
			continue
		}
		if identity == session.HostID {
			continue
		}
		role, roleErr := s.resolveJoinRole(ctx, identity, session)
		if roleErr == nil && role == model.ParticipantRoleTeacher {
			continue
		}
		canPublish := false
		var sources []string
		if p.Permission != nil {
			canPublish = p.Permission.CanPublish
			sources = trackSourceStrings(p.Permission.CanPublishSources)
		}
		_, _ = s.livekitSvc.UpdateParticipant(ctx, session.RoomName, p.Identity, dto.UpdateParticipantDTO{
			CanPublish:        ptrBool(canPublish),
			CanPublishSources: sources,
			CanPublishData:    ptrBool(!locked),
		})
	}

	return nil
}

// screenShareSources (D5, issue #58 review vong 3): duyet chia se man hinh CHI duoc phep publish
// hinh + tieng CUA MAN HINH — khong phai la cach "mo lai" camera/microphone cho hoc sinh. Hang so
// dung chung cho ca Start (cap) va nhac lai trong comment cua Stop (thu).
var screenShareSources = []string{"screen_share", "screen_share_audio"}

func (s *LivestreamService) StartScreenShare(ctx context.Context, actorID uuid.UUID, isAdmin bool, sessionID, targetID uuid.UUID) error {
	// D3 (issue #58 review vong 2): host/GV lop/instructor khoa/admin DUYET chia se man hinh cho
	// targetID (rong o tang handler = actorID, tuc host tu chia se). Truoc day StartScreenShare
	// chi kiem quyen NGUOI GOI ma khong lam gi ca — hoc sinh van khong publish duoc vi
	// CanPublish mac dinh (D3) la false, nen "duyet" chi la kiem quyen suong, khong cap gi.
	session, err := s.getManageableSession(ctx, actorID, isAdmin, sessionID)
	if err != nil {
		return err
	}
	// D5 (issue #58 review vong 3): duyet chia se man hinh KHONG duoc mo kem camera/microphone —
	// CanPublishSources gioi han dung 2 nguon man hinh. CanPublishData tinh lai theo vai tro/khoa
	// bang HIEN TAI (R2-1) thay vi bo trong de mac dinh co the vo tinh mo lai kenh du lieu dang
	// bi khoa.
	role, roleErr := s.resolveJoinRole(ctx, targetID, session)
	if roleErr != nil {
		role = model.ParticipantRoleStudent // fallback an toan: khong ro vai tro thi coi quyen thap nhat
	}
	_, _, canPublishData := participantGrant(role, session.Settings.WhiteboardLocked)
	_, err = s.livekitSvc.UpdateParticipant(ctx, session.RoomName, targetID.String(), dto.UpdateParticipantDTO{
		CanPublish:        ptrBool(true),
		CanPublishSources: screenShareSources,
		CanPublishData:    ptrBool(canPublishData),
	})
	return err
}

func (s *LivestreamService) StopScreenShare(ctx context.Context, actorID uuid.UUID, isAdmin bool, sessionID, targetID uuid.UUID) error {
	session, err := s.getManageableSession(ctx, actorID, isAdmin, sessionID)
	if err != nil {
		return err
	}
	// Khong ha CanPublish cua giao vien/host — baseline cua ho luon day du (D3), StopScreenShare
	// chi thu lai quyen da CAP RIENG cho hoc sinh qua StartScreenShare.
	role, roleErr := s.resolveJoinRole(ctx, targetID, session)
	if roleErr == nil && role == model.ParticipantRoleTeacher {
		return nil
	}
	if roleErr != nil {
		role = model.ParticipantRoleStudent
	}
	_, _, canPublishData := participantGrant(role, session.Settings.WhiteboardLocked)
	_, err = s.livekitSvc.UpdateParticipant(ctx, session.RoomName, targetID.String(), dto.UpdateParticipantDTO{
		CanPublish:     ptrBool(false),
		CanPublishData: ptrBool(canPublishData),
	})
	return err
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
		LessonContentID: session.LessonContentID,
		RoomName:        session.RoomName,
		Status:          string(session.Status),
		StartedAt:       startedAt,
		EndedAt:         endedAt,
		ScheduledAt:     scheduledAt,
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

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
	// Create: hostID la nguoi goi THAT SU (lay tu access token o tang handler), khong nam trong
	// req — xem comment tai dto.CreateLivestreamDTO.
	Create(ctx context.Context, hostID uuid.UUID, req dto.CreateLivestreamDTO) (*model.LivestreamSession, error)
	GetByID(ctx context.Context, id uuid.UUID) (*dto.LivestreamDetailDTO, error)
	// GetAll: lessonContentID (N10, review vòng 2) lọc phiên theo lesson_content_id, nil = không lọc.
	GetAll(ctx context.Context, page, pageSize int, status string, hostID *uuid.UUID, lessonContentID *uuid.UUID) (*dto.LivestreamListDTO, error)
	// Update/Delete/Start/End: userID la nguoi goi THAT SU (access token), kiem quyen quan tri phien
	// truoc khi lam bat cu thu gi — xem canManageSession (finding review V3-6/V3-7, issue #58).
	Update(ctx context.Context, userID, id uuid.UUID, req dto.UpdateLivestreamDTO) (*model.LivestreamSession, error)
	Delete(ctx context.Context, userID, id uuid.UUID) error
	Start(ctx context.Context, userID, id uuid.UUID) (*model.LivestreamSession, error)
	End(ctx context.Context, userID, id uuid.UUID) (*model.LivestreamSession, error)
	// Join: userID la nguoi tham gia THAT SU (access token) — xem dto.JoinLivestreamDTO.
	Join(ctx context.Context, sessionID, userID uuid.UUID, req dto.JoinLivestreamDTO) (*dto.ParticipantResponseDTO, error)
	// Leave: userID la nguoi roi phong THAT SU (access token), khong con nam trong DTO.
	Leave(ctx context.Context, sessionID, userID uuid.UUID) error
	GetParticipants(ctx context.Context, sessionID uuid.UUID, page, pageSize int) ([]model.Participant, int64, error)
	// MuteParticipant/KickParticipant: actorID la nguoi goi, targetID la doi tuong bi tac dong.
	MuteParticipant(ctx context.Context, actorID, sessionID, targetID uuid.UUID) error
	KickParticipant(ctx context.Context, actorID, sessionID, targetID uuid.UUID) error
	// LockWhiteboard: actorID la nguoi goi (chi host/GV lop/instructor khoa moi duoc khoa bang).
	LockWhiteboard(ctx context.Context, actorID, sessionID uuid.UUID, locked bool) error
	// StartScreenShare/StopScreenShare: actorID la nguoi goi.
	StartScreenShare(ctx context.Context, actorID, sessionID uuid.UUID) error
	StopScreenShare(ctx context.Context, actorID, sessionID uuid.UUID) error
}

type LivestreamService struct {
	repo            repository.LivestreamRepositoryInterface
	participantRepo repository.ParticipantRepositoryInterface
	analyticsRepo   repository.AnalyticsRepositoryInterface
	classRepo       repository.ClassRepositoryInterface
	courseRepo      repository.CourseRepositoryInterface
	// enrollmentRepo (finding V3-6, issue #58): Join phai kiem nguoi tham gia co quan he that voi
	// phien khong (hoc sinh da enroll khoa cua phien, hoac la GV lop/instructor khoa) — truoc day
	// bat ky user dang nhap nao cung join duoc bat ky phien nao. enrollmentRepo.GetByUserAndCourse
	// tra (nil, nil) khi khong tim thay, du de phan biet "khong enroll" voi loi ha tang.
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

// IsForbiddenErr (finding review V3-6/V3-7, issue #58): gom moi sentinel UY QUYEN cua nhom
// livestream ve MOT cho, de tang handler khong phai liet ke lai tung sentinel (va khong the quen
// mot cai khi them sau nay).
func IsForbiddenErr(err error) bool {
	return errors.Is(err, ErrNotClassTeacher) || errors.Is(err, ErrNotSessionMember) || errors.Is(err, ErrCannotKickHost)
}

// isClassTeacherOrInstructor (finding review V3-6/V3-7, issue #58) — NGUON SU THAT DUY NHAT cho
// cau hoi "user nay co vai tro giao vien voi lop nay khong" = giao vien cua lop HOAC instructor
// cua khoa chua lop. Truoc day phep kiem chi ton tai copy trong Create (fix N1); moi handler quan
// tri khac (Update/Delete/Start/End/mute/kick/khoa bang/chia se man hinh) deu KHONG kiem gi ca,
// nen bat ky user dang nhap nao cung ket thuc duoc phien cua nguoi khac hoac da nguoi ra khoi
// phong. `class` duoc truyen vao (da load) vi ca hai caller deu can no cho muc dich khac nua.
func (s *LivestreamService) isClassTeacherOrInstructor(ctx context.Context, userID uuid.UUID, class *model.Class) (bool, error) {
	if class == nil {
		return false, errors.New("class not found")
	}
	isTeacher, err := s.classRepo.TeacherClassExists(ctx, class.ID, userID)
	if err != nil {
		return false, fmt.Errorf("failed to verify class teacher: %w", err)
	}
	if isTeacher {
		return true, nil
	}
	if class.CourseID != nil {
		course, err := s.courseRepo.GetByID(ctx, *class.CourseID)
		if err != nil {
			return false, fmt.Errorf("failed to load course: %w", err)
		}
		if course != nil && course.InstructorID == userID {
			return true, nil
		}
	}
	return false, nil
}

// canManageClass tra ErrNotClassTeacher khi userID khong duoc quan tri lop. Luon fail-closed: moi
// loi doc du lieu deu thanh loi tra ve, khong bao gio thanh "cho phep".
func (s *LivestreamService) canManageClass(ctx context.Context, userID, classID uuid.UUID) error {
	class, err := s.classRepo.GetByID(ctx, classID)
	if err != nil {
		return fmt.Errorf("failed to load class: %w", err)
	}
	ok, err := s.isClassTeacherOrInstructor(ctx, userID, class)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotClassTeacher
	}
	return nil
}

// canManageSession = host cua phien HOAC nguoi quan tri duoc lop cua phien (canManageClass).
func (s *LivestreamService) canManageSession(ctx context.Context, userID uuid.UUID, session *model.LivestreamSession) error {
	if session == nil {
		return errors.New("session not found")
	}
	if userID == session.HostID {
		return nil
	}
	return s.canManageClass(ctx, userID, session.ClassID)
}

// getManageableSession tai phien va kiem quyen quan tri trong MOT buoc, de moi handler quan tri
// khong the vo tinh bo qua mot trong hai. Xem canManageSession.
func (s *LivestreamService) getManageableSession(ctx context.Context, userID, sessionID uuid.UUID) (*model.LivestreamSession, error) {
	session, err := s.repo.GetByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, errors.New("session not found")
	}
	if err := s.canManageSession(ctx, userID, session); err != nil {
		return nil, err
	}
	return session, nil
}

func (s *LivestreamService) Create(ctx context.Context, hostID uuid.UUID, req dto.CreateLivestreamDTO) (*model.LivestreamSession, error) {
	classID, err := uuid.Parse(req.ClassID)
	if err != nil {
		return nil, errors.New("invalid class_id")
	}

	// N1 (review vong 2, 260915): fix host_id (vong truoc) chi chan MAO DANH — phien khong con
	// mang ten nguoi khac duoc nua — nhung khong chan UY QUYEN: bat ky user dang nhap nao (ke ca
	// hoc sinh) van tao duoc phien gan vao MOT LOP BAT KY, va khi co scheduled_at, Create con
	// enqueue reminder BAN THONG BAO TOI CA LOP do. Kiem hostID phai la giao vien cua class_id
	// HOAC instructor cua khoa hoc chua lop do, truoc khi tao bat cu thu gi — dat truoc moi thao
	// tac ghi/enqueue trong ham nay nen tu dong bao ve ca duong goi noi bo
	// (ClassLessonContentService.createLivestreamSession cung truyen hostID = userID xac thuc,
	// khong co gi de bypass). Dung lai sentinel ErrNotClassTeacher da co san (grade_service.go,
	// C-13 audit 260909) thay vi khai bao ban sao — cung y nghia "khong phai giao vien lop nay".
	//
	// V3-6/V3-7 (issue #58): phep kiem nay da duoc rut thanh canManageClass de moi handler quan
	// tri khac dung lai dung mot dinh nghia — xem canManageClass. Host o day chinh la nguoi goi
	// nen khong can nhanh "userID == session.HostID".
	if err := s.canManageClass(ctx, hostID, classID); err != nil {
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

func (s *LivestreamService) GetAll(ctx context.Context, page, pageSize int, status string, hostID *uuid.UUID, lessonContentID *uuid.UUID) (*dto.LivestreamListDTO, error) {
	sessions, total, err := s.repo.GetAll(ctx, page, pageSize, status, hostID, lessonContentID)
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

func (s *LivestreamService) Update(ctx context.Context, userID, id uuid.UUID, req dto.UpdateLivestreamDTO) (*model.LivestreamSession, error) {
	// V3-6 (issue #58): truoc day bat ky user dang nhap nao cung doi duoc title/max_viewers cua
	// phien nguoi khac — keo theo ca viec ha max_viewers de chan nguoi khac vao phong.
	session, err := s.getManageableSession(ctx, userID, id)
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

func (s *LivestreamService) Delete(ctx context.Context, userID, id uuid.UUID) error {
	// V3-6 (issue #58): xoa phien la thao tac pha huy (xoa ca room LiveKit) — truoc day khong
	// kiem quyen, bat ky user dang nhap nao cung xoa duoc phien cua nguoi khac.
	session, err := s.getManageableSession(ctx, userID, id)
	if err != nil {
		return err
	}

	_ = s.livekitSvc.DeleteRoom(ctx, session.RoomName)
	return s.repo.Delete(ctx, id)
}

func (s *LivestreamService) Start(ctx context.Context, userID, id uuid.UUID) (*model.LivestreamSession, error) {
	// V3-6 (issue #58): mo phong hoc la thao tac quan tri — chi host/GV lop/instructor khoa.
	session, err := s.getManageableSession(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	return s.startSession(ctx, session)
}

// StartAsSystem mo phien cho task auto-start chay NEN (asynq TaskAutoStartLivestream) — task nay
// khong co nguoi goi, khong co access token, va khong den tu HTTP: no duoc len lich tu luc tao
// phien. Actor cua no chinh la host cua phien (nguoi da duoc kiem quyen ngay tai Create), nen day
// KHONG phai duong vong quyen: khong co tham so nao den tu client. Ten ham co chu "AsSystem" de
// bat ky ai doc code cung thay ngay day la duong noi bo, khong duoc goi tu handler.
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

func (s *LivestreamService) End(ctx context.Context, userID, id uuid.UUID) (*model.LivestreamSession, error) {
	// V3-6 (issue #58): ket thuc phien la thao tac quan tri — truoc day bat ky user dang nhap nao
	// cung ket thuc duoc phien dang dien ra (ngat live cua ca lop).
	session, err := s.getManageableSession(ctx, userID, id)
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

// resolveJoinRole (finding review V3-6, issue #58) quyet dinh vai tro cua nguoi tham gia phien,
// dua tren QUAN HE THAT trong DB — khong bao gio dua tren gia tri client gui len:
//
//	teacher  : host phien, GV cua lop, hoac instructor cua khoa chua lop
//	student  : hoc sinh da enroll lop cua phien, hoac da enroll khoa cua phien
//	ErrNotSessionMember: khong co quan he nao o tren -> khong duoc vao phong
//
// Truoc day vai tro lay tu `req.Role` (client tu khai) => bat ky ai cung tu phong minh len teacher,
// va `IsHost` cua LiveKit token duoc set theo role do.
func (s *LivestreamService) resolveJoinRole(ctx context.Context, userID uuid.UUID, session *model.LivestreamSession) (model.ParticipantRole, error) {
	if userID == session.HostID {
		return model.ParticipantRoleTeacher, nil
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

	// Hoc sinh cua lop (StudentClass) — duong enroll pho bien nhat cua phien gan lop.
	isStudent, err := s.classRepo.StudentClassExists(ctx, session.ClassID, userID)
	if err != nil {
		return "", fmt.Errorf("failed to verify class enrollment: %w", err)
	}
	if isStudent {
		return model.ParticipantRoleStudent, nil
	}

	// Duong enroll theo khoa (Enrollment). Uu tien khoa cua lop, dung khoa cua phien lam du phong
	// khi lop khong gan khoa nao. GetByUserAndCourse tra (nil, nil) khi khong tim thay.
	courseID := session.CourseID
	if class.CourseID != nil {
		courseID = class.CourseID
	}
	if courseID != nil {
		enrollment, err := s.enrollmentRepo.GetByUserAndCourse(ctx, userID, *courseID)
		if err != nil {
			return "", fmt.Errorf("failed to verify course enrollment: %w", err)
		}
		if enrollment != nil {
			return model.ParticipantRoleStudent, nil
		}
	}

	return "", ErrNotSessionMember
}

func (s *LivestreamService) Join(ctx context.Context, sessionID, userID uuid.UUID, req dto.JoinLivestreamDTO) (*dto.ParticipantResponseDTO, error) {
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

	// V3-6 (issue #58): chi nguoi co quan he that voi phien moi vao duoc, va vai tro do SERVER
	// suy ra tu quan he do (khong nhan tu body). Dat sau cac buoc kiem trang thai/suc chua de giu
	// nguyen thu tu thong bao loi cu cho nguoi dung hop le, va truoc moi thao tac ghi.
	role, err := s.resolveJoinRole(ctx, userID, session)
	if err != nil {
		return nil, err
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
		// V3-6 (issue #58): Identity PHAI la userID that (da xac thuc tu access token). Truoc day
		// lay req.UserID tu body nen token mang danh tinh cua nguoi khac neu client khai vay.
		token, err := s.livekitSvc.CreateJoinToken(ctx, session.RoomName, dto.JoinTokenDTO{
			Identity: userID.String(),
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
		Identity: userID.String(),
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

func (s *LivestreamService) GetParticipants(ctx context.Context, sessionID uuid.UUID, page, pageSize int) ([]model.Participant, int64, error) {
	return s.participantRepo.GetBySession(ctx, sessionID, page, pageSize)
}

func (s *LivestreamService) MuteParticipant(ctx context.Context, actorID, sessionID, targetID uuid.UUID) error {
	// V3-6 (issue #58): mute la thao tac quan tri — actorID (nguoi goi) phai la host/GV lop/
	// instructor khoa; targetID la nguoi bi mute. Truoc day khong kiem gi.
	session, err := s.getManageableSession(ctx, actorID, sessionID)
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
	updateReq := dto.UpdateParticipantDTO{
		CanPublish: ptrBool(false),
	}
	_, err = s.livekitSvc.UpdateParticipant(ctx, session.RoomName, targetID.String(), updateReq)
	return err
}

func (s *LivestreamService) KickParticipant(ctx context.Context, actorID, sessionID, targetID uuid.UUID) error {
	// V3-6 (issue #58): kick la thao tac quan tri — actorID (nguoi goi) phai la host/GV lop/
	// instructor khoa; targetID la nguoi bi da ra. Truoc day khong kiem gi.
	session, err := s.getManageableSession(ctx, actorID, sessionID)
	if err != nil {
		return err
	}

	// V3-7 (issue #58): chan tu-da HOST ra khoi phong cua chinh minh. Khong co chan nay thi mot
	// GV lop (khong phai host) — hoac chinh host tu bam nham — co the da host ra khoi phong, va
	// host la nguoi duy nhat con quyen quan tri phien: mat host = phien khong con ai quan tri.
	if targetID == session.HostID {
		return ErrCannotKickHost
	}

	if err := s.livekitSvc.RemoveParticipant(ctx, session.RoomName, targetID.String()); err != nil {
		return err
	}

	participant, _ := s.participantRepo.GetBySessionAndUser(ctx, sessionID, targetID)
	if participant != nil {
		_ = s.participantRepo.SetLeft(ctx, participant.ID)
	}

	return nil
}

func (s *LivestreamService) LockWhiteboard(ctx context.Context, actorID, sessionID uuid.UUID, locked bool) error {
	// V3-6 (issue #58): khoa/mo bang la thao tac quan tri.
	session, err := s.getManageableSession(ctx, actorID, sessionID)
	if err != nil {
		return err
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

func (s *LivestreamService) StartScreenShare(ctx context.Context, actorID, sessionID uuid.UUID) error {
	// V3-6 (issue #58): bat dau chia se man hinh la thao tac quan tri.
	if _, err := s.getManageableSession(ctx, actorID, sessionID); err != nil {
		return err
	}
	return nil
}

func (s *LivestreamService) StopScreenShare(ctx context.Context, actorID, sessionID uuid.UUID) error {
	// V3-6 (issue #58): dung chia se man hinh la thao tac quan tri.
	if _, err := s.getManageableSession(ctx, actorID, sessionID); err != nil {
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

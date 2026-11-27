package service

// QA8 review PR #157 (Postgres THẬT): M1 OAuth liên kết lời mời bằng email đã chuẩn hoá; M2 hai thao tác đồng thời cùng tên
// đăng nhập chỉ một thành công (pg_advisory_xact_lock); L4 tên tổ chức được cắt khoảng trắng. Mỗi ca ĐỎ khi bỏ phần sửa.

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"study.com/v1/internal/dto"
	"study.com/v1/internal/model"
	"study.com/v1/internal/repository"
	"study.com/v1/internal/thirdparty/oauth"
)

// Đỏ khi createOAuthUser lại truyền *userInfo.Email (chữ hoa của provider) cho LinkInvitationToNewUser.
func TestQA8Review_M1_OAuthSignup_MixedCaseEmail_LinksInvitation(t *testing.T) {
	f := newParentLinkFixture(t)
	child := f.user("child", "STUDENT")
	email := "qa8-oauth-" + uuid.NewString()[:8] + "@40study.test"
	inv := f.emailInvitation(child, email, time.Now())
	svc := &OAuthService{userRepo: repository.NewUserRepository(f.db), oauthRepo: repository.NewOAuthProviderRepository(f.db),
		parentInvitationSvc: f.invitationService()}
	providerEmail, name := "Qa8-OAuth-"+strings.TrimPrefix(email, "qa8-oauth-"), "Phụ Huynh"
	providerEmail = strings.Replace(providerEmail, "@40study.test", "@40Study.TEST", 1)
	u, err := svc.createOAuthUser(f.ctx, "google", &oauth.OAuthUserInfo{ProviderUserID: "g-" + uuid.NewString(), Email: &providerEmail, Name: &name})
	if err != nil {
		t.Fatalf("createOAuthUser: %v", err)
	}
	var got model.ParentInvitation
	if err := f.db.First(&got, "id = ?", inv.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.InviteeUserID == nil || *got.InviteeUserID != u.ID || got.Status != model.ParentInvitationStatusPending {
		t.Fatalf("lời mời chưa được liên kết: invitee=%v status=%s, muốn %s pending", got.InviteeUserID, got.Status, u.ID)
	}
}

// barrierUserRepo: sau khi ĐỌC UserNameExists, mỗi goroutine chờ goroutine kia cũng đọc xong (tối đa 1s) rồi mới trả kết quả, để
// ép cả hai cùng "thấy tên còn trống" nếu không có khoá. Có khoá thì goroutine thứ hai bị chặn ở pg_advisory_xact_lock trước
// khi đọc, goroutine đầu hết giờ chờ, ghi và commit; goroutine thứ hai đọc sau đó và thấy tên đã bị lấy.
type barrierUserRepo struct {
	repository.UserRepositoryInterface
	b *nameBarrier
}

type nameBarrier struct {
	armed   atomic.Bool
	counted atomic.Int32
}

func (r *barrierUserRepo) UserNameExists(ctx context.Context, name string) (bool, error) {
	taken, err := r.UserRepositoryInterface.UserNameExists(ctx, name)
	if r.b.armed.Load() {
		r.b.counted.Add(1)
		for deadline := time.Now().Add(time.Second); r.b.counted.Load() < 2 && time.Now().Before(deadline); {
			time.Sleep(2 * time.Millisecond)
		}
	}
	return taken, err
}

func (r *barrierUserRepo) WithUserNameLock(ctx context.Context, name string, fn func(repository.UserRepositoryInterface) error) error {
	return r.UserRepositoryInterface.WithUserNameLock(ctx, name, func(tx repository.UserRepositoryInterface) error {
		return fn(&barrierUserRepo{UserRepositoryInterface: tx, b: r.b})
	})
}

func newQA8BarrierEnv(t *testing.T) (*qa8Env, *nameBarrier) {
	e := newQA8Env(t)
	b := &nameBarrier{}
	usr := &lastLoginFakeUserSystemRoleRepo{}
	e.svc = NewAuthService(nil, &barrierUserRepo{UserRepositoryInterface: repository.NewUserRepository(e.db), b: b}, nil, nil, usr, nil, usr, e.rdb)
	return e, b
}

func runPair(fn func(i int) error) []error {
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); errs[i] = fn(i) }()
	}
	wg.Wait()
	return errs
}

func assertOneWinnerOneTaken(t *testing.T, errs []error) {
	t.Helper()
	ok, taken := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrUserNameTaken):
			taken++
		default:
			t.Fatalf("lỗi lạ: %v", err)
		}
	}
	if ok != 1 || taken != 1 {
		t.Fatalf("thành công=%d bị chặn=%d (errs=%v), muốn đúng 1 và 1", ok, taken, errs)
	}
}

// Đỏ khi Register kiểm tra tên ngoài WithUserNameLock (cả hai cùng tạo tài khoản trùng tên).
func TestQA8Review_M2_ConcurrentRegisterSameUserName_OnlyOneWins(t *testing.T) {
	e, b := newQA8BarrierEnv(t)
	ctx := context.Background()
	name := "dua-" + qa8Uniq()
	emails := []string{"a-" + qa8Uniq() + "@demo.com", "b-" + qa8Uniq() + "@demo.com"}
	otps := make([]string, 2)
	for i, em := range emails {
		if err := e.svc.RequestRegister(ctx, qa8RegisterReq(em, name)); err != nil {
			t.Fatal(err)
		}
		otps[i] = e.nextOTP(t)
	}
	b.armed.Store(true)
	errs := runPair(func(i int) error {
		_, err := e.svc.Register(ctx, dto.VerifyOtpRequestDto{Email: emails[i], OTP: otps[i]})
		return err
	})
	assertOneWinnerOneTaken(t, errs)
	var n int64
	e.db.Model(&model.User{}).Where("lower(user_name) = ?", name).Count(&n)
	if n != 1 {
		t.Fatalf("có %d tài khoản tên %q, muốn 1", n, name)
	}
}

// Đỏ khi UpdateMe kiểm tra tên ngoài WithUserNameLock.
func TestQA8Review_M2_ConcurrentUpdateMeSameUserName_OnlyOneWins(t *testing.T) {
	e, b := newQA8BarrierEnv(t)
	ctx := context.Background()
	users := []model.User{
		e.legacyUser(t, "u1-"+qa8Uniq()+"@demo.com", "u1-"+qa8Uniq(), true),
		e.legacyUser(t, "u2-"+qa8Uniq()+"@demo.com", "u2-"+qa8Uniq(), true),
	}
	name := "doiten-" + qa8Uniq()
	b.armed.Store(true)
	errs := runPair(func(i int) error {
		_, err := e.svc.UpdateMe(ctx, users[i].ID, dto.UpdateMeRequestDto{Username: &name})
		return err
	})
	assertOneWinnerOneTaken(t, errs)
	var n int64
	e.db.Model(&model.User{}).Where("lower(user_name) = ?", name).Count(&n)
	if n != 1 {
		t.Fatalf("có %d tài khoản đổi sang %q, muốn 1", n, name)
	}
}

// Đỏ khi bỏ trimOrgName: tên kèm khoảng trắng tạo được tổ chức thứ hai, tên toàn khoảng trắng lọt qua.
func TestQA8Review_L4_OrgName_TrimmedBeforeUniquenessAndStored(t *testing.T) {
	db := isolatedAPISchema(t)
	ctx := context.Background()
	owner := model.User{Email: "own-" + qa8Uniq() + "@demo.com", PasswordHash: "x", UserName: "own-" + qa8Uniq()}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewOrganizationService(repository.NewOrganizationRepository(db))
	base := "Trung tâm " + qa8Uniq()
	created, err := svc.CreateOrganization(ctx, owner.ID, dto.CreateOrganizationDTO{Name: "  " + base + " "})
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != base {
		t.Fatalf("tên lưu %q, muốn %q (đã cắt khoảng trắng)", created.Name, base)
	}
	if _, err := svc.CreateOrganization(ctx, owner.ID, dto.CreateOrganizationDTO{Name: base + "  "}); !errors.Is(err, ErrOrgNameExists) {
		t.Fatalf("tên kèm khoảng trắng: err=%v, muốn ErrOrgNameExists", err)
	}
	if _, err := svc.CreateOrganization(ctx, owner.ID, dto.CreateOrganizationDTO{Name: "   "}); !errors.Is(err, ErrOrgNameTooShort) {
		t.Fatalf("tên toàn khoảng trắng: err=%v, muốn ErrOrgNameTooShort", err)
	}
}

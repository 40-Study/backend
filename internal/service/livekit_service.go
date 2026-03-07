package service

import (
	"github.com/livekit/protocol/auth"
)

type LivekitServiceInterface interface {
	CreateToken(ctx context.Context, req dto.CreateLiveTokenDTO) (string, error)
}

type LivekitService struct {
	livekitRepository LivekitRepositoryInterface
}

func NewLivekitService(livekitRepository LivekitRepositoryInterface) *LivekitService {
	return &LivekitService{livekitRepository: livekitRepository}
}

func (s *LivekitService) CreateToken(ctx context.Context, req dto.CreateLiveTokenDTO) (string, error) {
	apiKey := os.Getenv("LIVEKIT_API_KEY")
	apiSecret := os.Getenv("LIVEKIT_API_SECRET")
	at := auth.NewAccessToken(apiKey, apiSecret)
	at.SetIdentity(req.Identity)
	at.AddGrant(&auth.VideoGrant{
		RoomJoin: true,
		Room:     req.RoomName,
	})
	return at.ToJWT()
}

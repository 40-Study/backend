package main

import (
	"fmt"

	"github.com/livekit/protocol/auth"
)

func main() {

	apiKey := os.Getenv("LIVEKIT_API_KEY")
	apiSecret := os.Getenv("LIVEKIT_API_SECRET")

	at := auth.NewAccessToken(apiKey, apiSecret)
	at.SetIdentity("user3")

	at.AddGrant(&auth.VideoGrant{
		RoomJoin: true,
		Room:     "test-room",
	})

	token, err := at.ToJWT()
	if err != nil {
		panic(err)
	}

	fmt.Println(token)
}

package main

import (
	"fmt"

	"github.com/livekit/protocol/auth"
)

func main() {

	apiKey := "api_tttt_key"
	apiSecret := "api_tttt_secret"

	at := auth.NewAccessToken(apiKey, apiSecret)
	at.SetIdentity("user1")

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

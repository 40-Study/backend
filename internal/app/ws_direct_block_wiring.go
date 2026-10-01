package app

// wireDirectBlockRealtime nối hai nửa của "chặn trong DM 1-1" cho realtime (L3):
//   - chặn/bỏ chặn phát `conversation_blocked_changed` tới cả hai người của DM (Friendship -> Conversation);
//   - "đang gõ" trong DM bị chặn không được chuyển tiếp (authorizer WS -> Conversation.IsDirectBlocked).
//
// Tách khỏi app.New/InitServices để test dựng đúng dây nối thật (dm_block_ws_live_test.go): bỏ dòng nào ở đây
// thì test ĐỎ, thay vì tính năng im lặng không chạy ở production.
func wireDirectBlockRealtime(s *Services, authz *wsChannelAuthorizer) {
	s.Friendship.SetDirectBlockPublisher(s.Conversation)
	authz.SetDirectBlockChecker(s.Conversation)
}

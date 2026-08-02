package telegram

import (
	"context"
	"strconv"
	"strings"

	"github.com/gotd/td/tg"
	"go.uber.org/zap"
)

func (bm *BotManager) registerMainHandlers(dispatcher *tg.UpdateDispatcher) {
	// 1. New Message updates
	dispatcher.OnNewMessage(func(ctx context.Context, entities tg.Entities, u *tg.UpdateNewMessage) error {
		msg, ok := u.Message.(*tg.Message)
		if !ok {
			return nil
		}

		peerUser, ok := msg.PeerID.(*tg.PeerUser)
		if !ok {
			return nil
		}
		userID := peerUser.UserID

		// Ignore self/bots
		if msg.Out {
			return nil
		}

		// Dynamically register admin command scope with verified AccessHash
		var senderUser *tg.User
		if user, exists := entities.Users[userID]; exists {
			senderUser = user
			go bm.SetupAdminCommands(ctx, userID, user.AccessHash)
		}

		// Handle State Listeners (client.listen equivalent)
		listenerVal := msg.Message
		if fwd, ok := msg.GetFwdFrom(); ok && fwd.ChannelPost != 0 {
			listenerVal = strconv.Itoa(fwd.ChannelPost)
		}

		if bm.state.Push(userID, listenerVal, msg.ID) {
			return nil // consumed by listener
		}

		// Handle normal commands
		if strings.HasPrefix(msg.Message, "/") {
			args := strings.Fields(msg.Message)
			cmd := args[0]
			return bm.handleMainCommand(ctx, userID, senderUser, cmd, args[1:], msg)
		}

		// Non-command text message
		return bm.handleMainTextMessage(ctx, userID, msg)
	})

	// 2. Callback Queries
	dispatcher.OnBotCallbackQuery(func(ctx context.Context, entities tg.Entities, u *tg.UpdateBotCallbackQuery) error {
		// Answer callback query first to stop loading animation
		api := bm.primary.API()
		_, _ = api.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{
			QueryID: u.QueryID,
		})

		return bm.handleMainCallback(ctx, u)
	})

	// 3. Channel Join Request
	dispatcher.OnBotChatInviteRequester(func(ctx context.Context, entities tg.Entities, u *tg.UpdateBotChatInviteRequester) error {
		peerID := bm.getPeerID(u.Peer)
		// Dynamic approval sub logic
		bm.logger.Info("Received join request", zap.Int64("user_id", u.UserID), zap.Int64("chat_id", peerID))
		
		// If request is monitored, approve it
		for _, ch := range bm.fsubChannels {
			if ch.ID == peerID && ch.RequestEnabled {
				// Accept join request via MessagesHideChatJoinRequest
				inputPeer := peerToInputPeer(u.Peer)
				if inputPeer != nil {
					_, err := bm.primary.API().MessagesHideChatJoinRequest(ctx, &tg.MessagesHideChatJoinRequestRequest{
						Approved: true,
						Peer:     inputPeer,
						UserID:   &tg.InputUser{UserID: u.UserID},
					})
					if err != nil {
						bm.logger.Warn("Failed to approve invite request", zap.String("error", err.Error()))
					} else {
						bm.logger.Info("Approved join request", zap.Int64("user_id", u.UserID))
					}
				}
				break
			}
		}
		return nil
	})
}

func (bm *BotManager) registerCloneHandlers(dispatcher *tg.UpdateDispatcher, token string) {
	// 1. New Message updates for Cloned Bots
	dispatcher.OnNewMessage(func(ctx context.Context, entities tg.Entities, u *tg.UpdateNewMessage) error {
		msg, ok := u.Message.(*tg.Message)
		if !ok {
			return nil
		}

		peerUser, ok := msg.PeerID.(*tg.PeerUser)
		if !ok {
			return nil
		}
		userID := peerUser.UserID

		if msg.Out {
			return nil
		}

		// Dynamically register clone owner commands with verified AccessHash
		if user, exists := entities.Users[userID]; exists {
			if client, exists := bm.clones[token]; exists {
				botDoc, err := bm.mongo.GetClonedBot(ctx, token)
				if err == nil && botDoc.UserID == userID {
					go bm.SetupCloneCommands(ctx, client, userID, user.AccessHash)
				}
			}
		}

		// Handle State Listeners (client.listen equivalent)
		if bm.state.Push(userID, msg.Message, msg.ID) {
			return nil // consumed by listener
		}

		if strings.HasPrefix(msg.Message, "/") {
			args := strings.Fields(msg.Message)
			cmd := args[0]
			return bm.handleCloneCommand(ctx, token, userID, cmd, args[1:], msg)
		}

		return nil
	})

	// 2. Callback Queries for Cloned Bots
	dispatcher.OnBotCallbackQuery(func(ctx context.Context, entities tg.Entities, u *tg.UpdateBotCallbackQuery) error {
		client, exists := bm.clones[token]
		if !exists {
			return nil
		}
		_, _ = client.API().MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{
			QueryID: u.QueryID,
		})

		return bm.handleCloneCallback(ctx, token, u)
	})
}

// Get object ID from Peer
func (p *BotManager) getPeerID(peer tg.PeerClass) int64 {
	switch ch := peer.(type) {
	case *tg.PeerUser:
		return ch.UserID
	case *tg.PeerChat:
		return ch.ChatID
	case *tg.PeerChannel:
		return ch.ChannelID
	}
	return 0
}

func peerToInputPeer(peer tg.PeerClass) tg.InputPeerClass {
	switch p := peer.(type) {
	case *tg.PeerChannel:
		return &tg.InputPeerChannel{ChannelID: p.ChannelID}
	case *tg.PeerUser:
		return &tg.InputPeerUser{UserID: p.UserID}
	case *tg.PeerChat:
		return &tg.InputPeerChat{ChatID: p.ChatID}
	}
	return nil
}

package telegram

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"filestore/pkg/crypto"
	"filestore/pkg/db"
	"filestore/pkg/shortener"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"go.mongodb.org/mongo-driver/bson"
	"go.uber.org/zap"
)

func (bm *BotManager) handleCloneCommand(ctx context.Context, token string, userID int64, cmd string, args []string, msg *tg.Message) error {
	client, exists := bm.clones[token]
	if !exists {
		return nil
	}

	peer := &tg.InputPeerUser{UserID: userID}

	// Fetch clone bot metadata
	var botDoc db.ClonedBotDoc
	encToken, _ := crypto.Encrypt(token, bm.mongo.TokenCryptKey())
	err := bm.mongo.DB.Collection("bots").FindOne(ctx, bson.M{
		"$or": []bson.M{
			{"token": encToken},
			{"token": token},
		},
	}).Decode(&botDoc)
	if err != nil {
		bm.logger.Warn("Failed to find clone bot info in database", zap.Error(err))
		return nil
	}

	// Helper to restrict to clone bot owner
	isOwner := userID == botDoc.UserID

	switch cmd {
	case "/start":
		return bm.handleCloneStart(ctx, client, botDoc, userID, args, msg)

	case "/settings":
		if !isOwner {
			return bm.sendCloneText(ctx, client, peer, "Unauthorized.")
		}
		return bm.sendCloneSettingsPanel(ctx, client, peer, botDoc, 0)

	case "/mysettings":
		return bm.sendCloneMySettingsPanel(ctx, client, peer, userID, 0)

	case "/api":
		if !isOwner {
			return bm.sendCloneText(ctx, client, peer, "Unauthorized.")
		}
		if len(args) == 0 {
			return bm.sendCloneText(ctx, client, peer, fmt.Sprintf("Usage: /api <your_shortener_api_key>\n\nCurrent: %s", botDoc.ShortenerAPI))
		}
		newAPI := args[0]
		_ = bm.updateCloneField(ctx, token, "shortener_api", newAPI)
		return bm.sendCloneText(ctx, client, peer, "Shortener API key updated successfully!")

	case "/base_site":
		if !isOwner {
			return bm.sendCloneText(ctx, client, peer, "Unauthorized.")
		}
		if len(args) == 0 {
			return bm.sendCloneText(ctx, client, peer, fmt.Sprintf("Usage: /base_site <shortener_domain>\nExample: /base_site linkshortify.com\n\nCurrent: %s", botDoc.BaseSite))
		}
		domain := args[0]
		_ = bm.updateCloneField(ctx, token, "base_site", domain)
		return bm.sendCloneText(ctx, client, peer, "Shortener base site domain updated successfully!")

	case "/set_caption":
		if !isOwner {
			return bm.sendCloneText(ctx, client, peer, "Unauthorized.")
		}
		if len(args) == 0 {
			return bm.sendCloneText(ctx, client, peer, "Usage: /set_caption <your_custom_caption_text>")
		}
		caption := strings.Join(args, " ")
		_ = bm.updateCloneField(ctx, token, "custom_caption", caption)
		return bm.sendCloneText(ctx, client, peer, "Custom file caption updated successfully!")

	case "/premium":
		upi := botDoc.UPIID
		qr := botDoc.QRPic
		plans := botDoc.PlansDetails
		if upi == "" {
			upi = "No UPI ID set by clone bot owner."
		}
		if plans == "" {
			plans = "Plan details: Contact the bot owner."
		}
		msgText := fmt.Sprintf("<b>✦ %s ✦</b>\n\n%s\n\n<b>UPI ID:</b> <code>%s</code>", ToSmallCaps("Premium Plans"), plans, upi)
		if qr != "" {
			msgText += fmt.Sprintf("\n<b>QR Code:</b> <a href=\"%s\">Link</a>", qr)
		}
		return bm.sendCloneText(ctx, client, peer, msgText)

	case "/broadcast":
		if !isOwner {
			return bm.sendCloneText(ctx, client, peer, "Unauthorized.")
		}
		var replyToMsgID int
		if msg.ReplyTo != nil {
			if header, ok := msg.ReplyTo.(*tg.MessageReplyHeader); ok {
				replyToMsgID = header.ReplyToMsgID
			}
		}
		if replyToMsgID == 0 && len(args) == 0 {
			return bm.sendCloneText(ctx, client, peer, "Usage: Reply to a message with /broadcast, or use /broadcast <message_text>")
		}
		go bm.handleCloneBroadcast(ctx, client, botDoc.BotID, strings.Join(args, " "), replyToMsgID, peer)
		return bm.sendCloneText(ctx, client, peer, "Broadcast started in background...")

	case "/users":
		if !isOwner {
			return bm.sendCloneText(ctx, client, peer, "Unauthorized.")
		}
		count, _ := bm.mongo.CloneUserCount(ctx, botDoc.BotID)
		return bm.sendCloneText(ctx, client, peer, fmt.Sprintf("<b>Total Users:</b> <code>%d</code>", count))

	case "/ban":
		if !isOwner {
			return bm.sendCloneText(ctx, client, peer, "Unauthorized.")
		}
		if len(args) == 0 {
			return bm.sendCloneText(ctx, client, peer, "Usage: /ban <user_id>")
		}
		targetID, _ := strconv.ParseInt(args[0], 10, 64)
		if targetID == 0 {
			return bm.sendCloneText(ctx, client, peer, "Invalid user ID.")
		}
		_ = bm.mongo.BanCloneUser(ctx, botDoc.BotID, targetID)
		return bm.sendCloneText(ctx, client, peer, fmt.Sprintf("\u2705 User <code>%d</code> has been banned.", targetID))

	case "/unban":
		if !isOwner {
			return bm.sendCloneText(ctx, client, peer, "Unauthorized.")
		}
		if len(args) == 0 {
			return bm.sendCloneText(ctx, client, peer, "Usage: /unban <user_id>")
		}
		targetID, _ := strconv.ParseInt(args[0], 10, 64)
		if targetID == 0 {
			return bm.sendCloneText(ctx, client, peer, "Invalid user ID.")
		}
		_ = bm.mongo.UnbanCloneUser(ctx, botDoc.BotID, targetID)
		return bm.sendCloneText(ctx, client, peer, fmt.Sprintf("\u2705 User <code>%d</code> has been unbanned.", targetID))

	case "/addpremium":
		if !isOwner {
			return bm.sendCloneText(ctx, client, peer, "Unauthorized.")
		}
		if len(args) < 1 {
			return bm.sendCloneText(ctx, client, peer, "Usage: /addpremium <user_id> [days]\nOmit days for permanent.")
		}
		targetID, _ := strconv.ParseInt(args[0], 10, 64)
		if targetID == 0 {
			return bm.sendCloneText(ctx, client, peer, "Invalid user ID.")
		}
		var expiry *time.Time
		if len(args) >= 2 {
			days, _ := strconv.Atoi(args[1])
			if days > 0 {
				e := time.Now().Add(time.Duration(days) * 24 * time.Hour)
				expiry = &e
			}
		}
		_ = bm.mongo.AddClonePro(ctx, botDoc.BotID, targetID, expiry)
		if expiry != nil {
			return bm.sendCloneText(ctx, client, peer, fmt.Sprintf("\u2705 Premium granted to <code>%d</code> until %s", targetID, expiry.Format("2006-01-02")))
		}
		return bm.sendCloneText(ctx, client, peer, fmt.Sprintf("\u2705 Permanent premium granted to <code>%d</code>", targetID))

	case "/delpremium":
		if !isOwner {
			return bm.sendCloneText(ctx, client, peer, "Unauthorized.")
		}
		if len(args) == 0 {
			return bm.sendCloneText(ctx, client, peer, "Usage: /delpremium <user_id>")
		}
		targetID, _ := strconv.ParseInt(args[0], 10, 64)
		_ = bm.mongo.RemoveClonePro(ctx, botDoc.BotID, targetID)
		return bm.sendCloneText(ctx, client, peer, fmt.Sprintf("\u2705 Premium revoked from <code>%d</code>", targetID))

	case "/premiumusers":
		if !isOwner {
			return bm.sendCloneText(ctx, client, peer, "Unauthorized.")
		}
		pros, err := bm.mongo.GetCloneProsList(ctx, botDoc.BotID)
		if err != nil || len(pros) == 0 {
			return bm.sendCloneText(ctx, client, peer, "No premium users found.")
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("<b>Premium Users (%d):</b>\n", len(pros)))
		for _, id := range pros {
			sb.WriteString(fmt.Sprintf("\u2022 <code>%d</code>\n", id))
		}
		return bm.sendCloneText(ctx, client, peer, sb.String())

	case "/profile":
		if !isOwner {
			return bm.sendCloneText(ctx, client, peer, "Unauthorized.")
		}
		if len(args) == 0 {
			return bm.sendCloneText(ctx, client, peer, "Usage: /profile <user_id>")
		}
		targetID, _ := strconv.ParseInt(args[0], 10, 64)
		banned, _ := bm.mongo.IsCloneUserBanned(ctx, botDoc.BotID, targetID)
		isPro, _ := bm.mongo.IsClonePro(ctx, botDoc.BotID, targetID)
		profileMsg := fmt.Sprintf("<blockquote>\u2726 %s \u2726</blockquote>\n\n\u203a\u203a <b>User ID:</b> <code>%d</code>\n\u203a\u203a <b>Banned:</b> %t\n\u203a\u203a <b>Premium:</b> %t",
			ToSmallCaps("User Profile"), targetID, banned, isPro)
		return bm.sendCloneText(ctx, client, peer, profileMsg)
	}

	return nil
}

func (bm *BotManager) handleCloneStart(ctx context.Context, client *telegram.Client, botDoc db.ClonedBotDoc, userID int64, args []string, msg *tg.Message) error {
	peer := &tg.InputPeerUser{UserID: userID}

	// Clone-scoped ban check
	cloneBanned, _ := bm.mongo.IsCloneUserBanned(ctx, botDoc.BotID, userID)
	if cloneBanned {
		return bm.sendCloneText(ctx, client, peer, "<b>You have been banned from using this bot!</b>")
	}

	// Add user record
	_ = bm.mongo.AddUser(ctx, userID)

	// Add to clone user base for broadcasts
	_ = bm.mongo.AddCloneUser(ctx, botDoc.BotID, userID)

	if len(args) == 0 {
		startText := botDoc.StartText
		if startText == "" {
			startText = fmt.Sprintf("<b>Hello! Welcome to @%s.</b>\n\nI am a cloned File Store bot. You can search or request files using start links.", botDoc.Username)
		}
		return bm.sendCloneText(ctx, client, peer, startText)
	}

	payload := args[0]
	isShortLink := false
	if strings.HasPrefix(payload, "yu3elk") {
		payload = payload[6 : len(payload)-1]
		isShortLink = true
	}

	// Check if user is premium (clone-scoped)
	isPremium, _ := bm.mongo.IsClonePro(ctx, botDoc.BotID, userID)

	// If not premium/owner AND clone bot has shortener configured, redirect
	if !isPremium && userID != botDoc.UserID && !isShortLink && botDoc.ShortenerAPI != "" && botDoc.BaseSite != "" {
		shortLink, err := shortener.GetShortlink(botDoc.BaseSite, botDoc.ShortenerAPI, fmt.Sprintf("https://t.me/%s?start=yu3elk%s7", botDoc.Username, payload))
		if err == nil {
			caption := fmt.Sprintf("<b>⌯ %s.</b>", ToSmallCaps("Here is your download link. Solve the shortener to continue"))
			rows := [][]tg.KeyboardButtonClass{
				{
					NewURLButtonWithStyle(ToSmallCaps("Download"), shortLink, styleGreen),
				},
				{
					NewCallbackButtonWithStyle(ToSmallCaps("Buy Premium"), "premium", styleGreen),
				},
			}
			return bm.sendCloneTextWithMarkup(ctx, client, peer, caption, NewInlineMarkup(rows))
		}
	}

	// Force subscription check for clone bot
	if botDoc.FSubChannelID != 0 && userID != botDoc.UserID {
		api := client.API()
		_, err := api.ChannelsGetParticipant(ctx, &tg.ChannelsGetParticipantRequest{
			Channel: &tg.InputChannel{ChannelID: botDoc.FSubChannelID},
			Participant: peer,
		})
		if err != nil {
			fsubText := botDoc.FSubText
			if fsubText == "" {
				fsubText = "<b>⚠️ You must join our channel to use this bot!</b>"
			}
			inviteLink := fmt.Sprintf("https://t.me/c/%d/1", botDoc.FSubChannelID)
			rows := [][]tg.KeyboardButtonClass{
				{
					NewURLButtonWithStyle(ToSmallCaps("Join Channel"), inviteLink, styleGreen),
				},
			}
			return bm.sendCloneTextWithMarkup(ctx, client, peer, fsubText, NewInlineMarkup(rows))
		}
	}

	// Retrieve files
	decodedStr, err := base64Decode(payload)
	if err != nil {
		return bm.sendCloneText(ctx, client, peer, "⚠️ Invalid or expired link.")
	}

	var fileIDs []int
	var targetChannelID int64 = bm.primaryDBID
	if botDoc.DBChannelID != 0 {
		targetChannelID = botDoc.DBChannelID
	}

	if strings.Contains(decodedStr, "_") {
		// New Fail-Safe format: get_<channelID>_<msgID> or get_<channelID>_<startID>_<endID>
		parts := strings.Split(decodedStr, "_")
		if len(parts) >= 3 && parts[0] == "get" {
			parsedChID, _ := strconv.ParseInt(parts[1], 10, 64)
			if parsedChID != 0 {
				targetChannelID = parsedChID
			}

			if len(parts) == 3 {
				msgID, _ := strconv.Atoi(parts[2])
				if msgID > 0 {
					fileIDs = []int{msgID}
				}
			} else if len(parts) == 4 {
				startID, _ := strconv.Atoi(parts[2])
				endID, _ := strconv.Atoi(parts[3])
				if startID > 0 && endID > 0 {
					if startID <= endID {
						for i := startID; i <= endID; i++ {
							fileIDs = append(fileIDs, i)
						}
					} else {
						for i := startID; i >= endID; i-- {
							fileIDs = append(fileIDs, i)
						}
					}
				}
			}
		}
	} else {
		// Legacy multiplier format
		parts := strings.Split(decodedStr, "-")
		if len(parts) == 3 {
			startEncoded, _ := strconv.Atoi(parts[1])
			endEncoded, _ := strconv.Atoi(parts[2])

			mult := int(math.Abs(float64(targetChannelID)))
			if mult > 0 && startEncoded%mult == 0 && endEncoded%mult == 0 {
				startID := startEncoded / mult
				endID := endEncoded / mult
				if startID <= endID {
					for i := startID; i <= endID; i++ {
						fileIDs = append(fileIDs, i)
					}
				} else {
					for i := startID; i >= endID; i-- {
						fileIDs = append(fileIDs, i)
					}
				}
			}
		} else if len(parts) == 2 {
			encodedID, _ := strconv.Atoi(parts[1])
			mult := int(math.Abs(float64(targetChannelID)))
			if mult > 0 && encodedID%mult == 0 {
				fileIDs = []int{encodedID / mult}
			}
		}
	}

	if len(fileIDs) == 0 {
		return bm.sendCloneText(ctx, client, peer, "⚠️ Files not found.")
	}

	api := client.API()
	inputChannel := &tg.InputPeerChannel{ChannelID: targetChannelID}

	// Personal settings
	userPrefs, _ := bm.mongo.GetUserSettings(ctx, userID)
	protectContent := bm.config.Protect
	if userPrefs.Protect {
		protectContent = true
	}
	autoDelDelay := bm.config.AutoDel
	if userPrefs.AutoDel > 0 {
		autoDelDelay = userPrefs.AutoDel
	}

	for _, msgID := range fileIDs {
		updates, err := api.MessagesForwardMessages(ctx, &tg.MessagesForwardMessagesRequest{
			FromPeer:    inputChannel,
			ID:          []int{msgID},
			ToPeer:      peer,
			WithMyScore: false,
			DropAuthor:  true, // makes copy
			Noforwards:  protectContent,
		})
		if err != nil {
			bm.logger.Warn("Clone bot failed to forward message", zap.Int("msg_id", msgID), zap.Error(err))
		} else if autoDelDelay > 0 {
			sentMsgIDs := getSentMsgIDs(updates)
			if len(sentMsgIDs) > 0 {
				go func(msgIDs []int, delay int) {
					time.Sleep(time.Duration(delay) * time.Second)
					_, _ = api.MessagesDeleteMessages(ctx, &tg.MessagesDeleteMessagesRequest{
						ID:     msgIDs,
						Revoke: true,
					})
				}(sentMsgIDs, autoDelDelay)
			}
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Send auto-delete notification
	if autoDelDelay > 0 {
		delMinutes := autoDelDelay / 60
		if delMinutes == 0 {
			delMinutes = 1
		}
		notifMsg := fmt.Sprintf("\u26a0\ufe0f <b>These files will be auto-deleted in %d minute(s).</b>\nPlease save them before that.", delMinutes)
		_ = bm.sendCloneText(ctx, client, peer, notifMsg)
	}

	return nil
}

func (bm *BotManager) handleCloneCallback(ctx context.Context, token string, u *tg.UpdateBotCallbackQuery) error {
	client, exists := bm.clones[token]
	if !exists {
		return nil
	}
	peer := &tg.InputPeerUser{UserID: u.UserID}
	data := string(u.Data)

	// Fetch clone bot metadata
	var botDoc db.ClonedBotDoc
	encToken, _ := crypto.Encrypt(token, bm.mongo.TokenCryptKey())
	_ = bm.mongo.DB.Collection("bots").FindOne(ctx, bson.M{
		"$or": []bson.M{
			{"token": encToken},
			{"token": token},
		},
	}).Decode(&botDoc)

	// Settings route
	switch data {
	case "premium":
		upi := botDoc.UPIID
		qr := botDoc.QRPic
		plans := botDoc.PlansDetails
		if upi == "" {
			upi = "No UPI ID set by clone bot owner."
		}
		if plans == "" {
			plans = "Plan details: Contact the bot owner."
		}
		msgText := fmt.Sprintf("<b>✦ %s ✦</b>\n\n%s\n\n<b>UPI ID:</b> <code>%s</code>", ToSmallCaps("Premium Plans"), plans, upi)
		if qr != "" {
			msgText += fmt.Sprintf("\n<b>QR Code:</b> <a href=\"%s\">Link</a>", qr)
		}
		return bm.sendCloneText(ctx, client, peer, msgText)

	// Toggle user preferences (for standard clone users)
	case "my_clone_toggle_autodel":
		userPrefs, _ := bm.mongo.GetUserSettings(ctx, u.UserID)
		nextDel := 0
		switch userPrefs.AutoDel {
		case 0:
			nextDel = 60
		case 60:
			nextDel = 300
		case 300:
			nextDel = 900
		case 900:
			nextDel = 1800
		default:
			nextDel = 0
		}
		_ = bm.mongo.UpdateUserSettings(ctx, u.UserID, nextDel, userPrefs.Protect)
		return bm.sendCloneMySettingsPanel(ctx, client, peer, u.UserID, u.MsgID)

	case "my_clone_toggle_protect":
		userPrefs, _ := bm.mongo.GetUserSettings(ctx, u.UserID)
		_ = bm.mongo.UpdateUserSettings(ctx, u.UserID, userPrefs.AutoDel, !userPrefs.Protect)
		return bm.sendCloneMySettingsPanel(ctx, client, peer, u.UserID, u.MsgID)

	// Clone settings callbacks (for owner)
	case "clone_home":
		return bm.sendCloneSettingsPanel(ctx, client, peer, botDoc, u.MsgID)
	case "clone_api_menu":
		return bm.sendCloneShortenerMenu(ctx, client, peer, botDoc, u.MsgID)
	case "clone_txt_menu":
		return bm.sendCloneTextsMenu(ctx, client, peer, botDoc, u.MsgID)
	case "clone_pics_menu":
		return bm.sendClonePicsMenu(ctx, client, peer, botDoc, u.MsgID)
	case "clone_ch_menu":
		return bm.sendCloneChannelsMenu(ctx, client, peer, botDoc, u.MsgID)

	// Prompt triggers
	case "clone_add_api":
		go bm.promptCloneAPI(ctx, client, u.UserID, token)
	case "clone_add_site":
		go bm.promptCloneSite(ctx, client, u.UserID, token)
	case "clone_add_caption":
		go bm.promptCloneCaption(ctx, client, u.UserID, token)
	case "clone_add_start_txt":
		go bm.promptCloneStartTxt(ctx, client, u.UserID, token)
	case "clone_add_fsub_txt":
		go bm.promptCloneFSubTxt(ctx, client, u.UserID, token)
	case "clone_add_upi":
		go bm.promptCloneUPI(ctx, client, u.UserID, token)
	case "clone_add_plans":
		go bm.promptClonePlans(ctx, client, u.UserID, token)
	case "clone_add_fsub_ch":
		go bm.promptCloneFSubCh(ctx, client, u.UserID, token)
	case "clone_add_db_ch":
		go bm.promptCloneDBCh(ctx, client, u.UserID, token)

	case "clone_close":
		_, err := client.API().MessagesDeleteMessages(ctx, &tg.MessagesDeleteMessagesRequest{
			ID:     []int{u.MsgID},
			Revoke: true,
		})
		return err
	}
	return nil
}

// Clone Settings Dashboard layout

func (bm *BotManager) sendCloneSettingsPanel(ctx context.Context, client *telegram.Client, peer tg.InputPeerClass, botDoc db.ClonedBotDoc, editMsgID int) error {
	msg := fmt.Sprintf("<blockquote>✦ %s ✦</blockquote>\nConfigure your clone bot options below:", ToSmallCaps("Clone Settings Dashboard"))

	rows := [][]tg.KeyboardButtonClass{
		{
			NewCallbackButtonWithStyle(ToSmallCaps("Shortener"), "clone_api_menu", styleGreen),
			NewCallbackButtonWithStyle(ToSmallCaps("Texts Config"), "clone_txt_menu", styleGreen),
		},
		{
			NewCallbackButtonWithStyle(ToSmallCaps("Banner & Pics"), "clone_pics_menu", styleGreen),
			NewCallbackButtonWithStyle(ToSmallCaps("Channels Setup"), "clone_ch_menu", styleGreen),
		},
		{
			NewCallbackButtonWithStyle(ToSmallCaps("Close Panel"), "clone_close", styleRed),
		},
	}

	if editMsgID > 0 {
		_, err := client.API().MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
			Peer:        peer,
			ID:          editMsgID,
			Message:     msg,
			ReplyMarkup: NewInlineMarkup(rows),
			NoWebpage:   true,
		})
		return err
	}

	_, err := client.API().MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
		Peer:        peer,
		Message:     msg,
		ReplyMarkup: NewInlineMarkup(rows),
		NoWebpage:   true,
		RandomID:    getRandomID(),
	})
	return err
}

func (bm *BotManager) sendCloneShortenerMenu(ctx context.Context, client *telegram.Client, peer tg.InputPeerClass, botDoc db.ClonedBotDoc, msgID int) error {
	msg := fmt.Sprintf("<blockquote>✦ %s ✦</blockquote>\n›› **Domain:** %s\n›› **API Key:** %s", ToSmallCaps("Clone Shortener Settings"), botDoc.BaseSite, botDoc.ShortenerAPI)
	rows := [][]tg.KeyboardButtonClass{
		{
			NewCallbackButtonWithStyle(ToSmallCaps("Set API Key"), "clone_add_api", styleGreen),
			NewCallbackButtonWithStyle(ToSmallCaps("Set Domain"), "clone_add_site", styleGreen),
		},
		{
			NewCallbackButtonWithStyle(ToSmallCaps("Back"), "clone_home", styleBlue),
		},
	}
	_, err := client.API().MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
		Peer:        peer,
		ID:          msgID,
		Message:     msg,
		ReplyMarkup: NewInlineMarkup(rows),
		NoWebpage:   true,
	})
	return err
}

func (bm *BotManager) sendCloneTextsMenu(ctx context.Context, client *telegram.Client, peer tg.InputPeerClass, botDoc db.ClonedBotDoc, msgID int) error {
	msg := fmt.Sprintf("<blockquote>✦ %s ✦</blockquote>\nConfigure start link welcome message and captions:", ToSmallCaps("Clone Texts Customization"))
	rows := [][]tg.KeyboardButtonClass{
		{
			NewCallbackButtonWithStyle(ToSmallCaps("Set Caption"), "clone_add_caption", styleGreen),
			NewCallbackButtonWithStyle(ToSmallCaps("Set Welcome"), "clone_add_start_txt", styleGreen),
		},
		{
			NewCallbackButtonWithStyle(ToSmallCaps("Set FSub Text"), "clone_add_fsub_txt", styleGreen),
		},
		{
			NewCallbackButtonWithStyle(ToSmallCaps("Back"), "clone_home", styleBlue),
		},
	}
	_, err := client.API().MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
		Peer:        peer,
		ID:          msgID,
		Message:     msg,
		ReplyMarkup: NewInlineMarkup(rows),
		NoWebpage:   true,
	})
	return err
}

func (bm *BotManager) sendClonePicsMenu(ctx context.Context, client *telegram.Client, peer tg.InputPeerClass, botDoc db.ClonedBotDoc, msgID int) error {
	msg := fmt.Sprintf("<blockquote>✦ %s ✦</blockquote>\nConfigure monetization channels and UPI coordinates:", ToSmallCaps("Clone Checkout Details"))
	rows := [][]tg.KeyboardButtonClass{
		{
			NewCallbackButtonWithStyle(ToSmallCaps("Set UPI ID"), "clone_add_upi", styleGreen),
			NewCallbackButtonWithStyle(ToSmallCaps("Set Plans"), "clone_add_plans", styleGreen),
		},
		{
			NewCallbackButtonWithStyle(ToSmallCaps("Back"), "clone_home", styleBlue),
		},
	}
	_, err := client.API().MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
		Peer:        peer,
		ID:          msgID,
		Message:     msg,
		ReplyMarkup: NewInlineMarkup(rows),
		NoWebpage:   true,
	})
	return err
}

func (bm *BotManager) sendCloneChannelsMenu(ctx context.Context, client *telegram.Client, peer tg.InputPeerClass, botDoc db.ClonedBotDoc, msgID int) error {
	msg := fmt.Sprintf("<blockquote>✦ %s ✦</blockquote>\n›› **FSub Channel ID:** %d\n›› **DB Storage ID:** %d", ToSmallCaps("Clone Channels Connection"), botDoc.FSubChannelID, botDoc.DBChannelID)
	rows := [][]tg.KeyboardButtonClass{
		{
			NewCallbackButtonWithStyle(ToSmallCaps("Set FSub Channel"), "clone_add_fsub_ch", styleGreen),
			NewCallbackButtonWithStyle(ToSmallCaps("Set DB Channel"), "clone_add_db_ch", styleGreen),
		},
		{
			NewCallbackButtonWithStyle(ToSmallCaps("Back"), "clone_home", styleBlue),
		},
	}
	_, err := client.API().MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
		Peer:        peer,
		ID:          msgID,
		Message:     msg,
		ReplyMarkup: NewInlineMarkup(rows),
		NoWebpage:   true,
	})
	return err
}

func (bm *BotManager) sendCloneMySettingsPanel(ctx context.Context, client *telegram.Client, peer tg.InputPeerClass, userID int64, editMsgID int) error {
	userPrefs, _ := bm.mongo.GetUserSettings(ctx, userID)

	autoDelStr := "Disabled ❌"
	if userPrefs.AutoDel > 0 {
		autoDelStr = fmt.Sprintf("%dm 🟢", userPrefs.AutoDel/60)
	}

	protectStr := "Disabled ❌"
	if userPrefs.Protect {
		protectStr = "Enabled 🟢"
	}

	msg := fmt.Sprintf("<blockquote>✦ %s ✦</blockquote>\n\nConfigure your personal downloader preferences:\n\n›› **Auto Delete:** %s\n›› **Protect Content:** %s", ToSmallCaps("My Settings"), autoDelStr, protectStr)

	rows := [][]tg.KeyboardButtonClass{
		{
			NewCallbackButtonWithStyle(ToSmallCaps("Toggle Auto Delete"), "my_clone_toggle_autodel", styleGreen),
			NewCallbackButtonWithStyle(ToSmallCaps("Toggle Protect"), "my_clone_toggle_protect", styleGreen),
		},
		{
			NewCallbackButtonWithStyle(ToSmallCaps("Close"), "clone_close", styleRed),
		},
	}

	if editMsgID > 0 {
		_, err := client.API().MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
			Peer:        peer,
			ID:          editMsgID,
			Message:     msg,
			ReplyMarkup: NewInlineMarkup(rows),
			NoWebpage:   true,
		})
		return err
	}
	_, err := client.API().MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
		Peer:        peer,
		Message:     msg,
		ReplyMarkup: NewInlineMarkup(rows),
		NoWebpage:   true,
		RandomID:    getRandomID(),
	})
	return err
}

// Background prompt input handlers for clone settings

func (bm *BotManager) promptCloneAPI(ctx context.Context, client *telegram.Client, userID int64, token string) {
	peer := &tg.InputPeerUser{UserID: userID}
	_ = bm.sendCloneText(ctx, client, peer, "<b>Send your new URL Shortener API key in the next 60 seconds:</b>")
	val, err := bm.state.Listen(ctx, userID, 60*time.Second)
	if err == nil && val != "/cancel" {
		_ = bm.updateCloneField(ctx, token, "shortener_api", strings.TrimSpace(val))
		_ = bm.sendCloneText(ctx, client, peer, "API key updated successfully!")
	}
}

func (bm *BotManager) promptCloneSite(ctx context.Context, client *telegram.Client, userID int64, token string) {
	peer := &tg.InputPeerUser{UserID: userID}
	_ = bm.sendCloneText(ctx, client, peer, "<b>Send your URL Shortener domain in the next 60 seconds:</b>\nExample: `linkshortify.com`")
	val, err := bm.state.Listen(ctx, userID, 60*time.Second)
	if err == nil && val != "/cancel" {
		_ = bm.updateCloneField(ctx, token, "base_site", strings.TrimSpace(val))
		_ = bm.sendCloneText(ctx, client, peer, "Shortener domain updated successfully!")
	}
}

func (bm *BotManager) promptCloneCaption(ctx context.Context, client *telegram.Client, userID int64, token string) {
	peer := &tg.InputPeerUser{UserID: userID}
	_ = bm.sendCloneText(ctx, client, peer, "<b>Send your custom file download caption text:</b>")
	val, err := bm.state.Listen(ctx, userID, 60*time.Second)
	if err == nil && val != "/cancel" {
		_ = bm.updateCloneField(ctx, token, "custom_caption", val)
		_ = bm.sendCloneText(ctx, client, peer, "Custom file caption updated successfully!")
	}
}

func (bm *BotManager) promptCloneStartTxt(ctx context.Context, client *telegram.Client, userID int64, token string) {
	peer := &tg.InputPeerUser{UserID: userID}
	_ = bm.sendCloneText(ctx, client, peer, "<b>Send your custom start greeting text:</b>")
	val, err := bm.state.Listen(ctx, userID, 60*time.Second)
	if err == nil && val != "/cancel" {
		_ = bm.updateCloneField(ctx, token, "start_text", val)
		_ = bm.sendCloneText(ctx, client, peer, "Start greeting message updated!")
	}
}

func (bm *BotManager) promptCloneFSubTxt(ctx context.Context, client *telegram.Client, userID int64, token string) {
	peer := &tg.InputPeerUser{UserID: userID}
	_ = bm.sendCloneText(ctx, client, peer, "<b>Send your custom Force Subscribe prompt text:</b>")
	val, err := bm.state.Listen(ctx, userID, 60*time.Second)
	if err == nil && val != "/cancel" {
		_ = bm.updateCloneField(ctx, token, "fsub_text", val)
		_ = bm.sendCloneText(ctx, client, peer, "Force Subscribe prompt message updated!")
	}
}

func (bm *BotManager) promptCloneUPI(ctx context.Context, client *telegram.Client, userID int64, token string) {
	peer := &tg.InputPeerUser{UserID: userID}
	_ = bm.sendCloneText(ctx, client, peer, "<b>Send your premium checkout UPI ID in the next 60 seconds:</b>")
	val, err := bm.state.Listen(ctx, userID, 60*time.Second)
	if err == nil && val != "/cancel" {
		_ = bm.updateCloneField(ctx, token, "upi_id", strings.TrimSpace(val))
		_ = bm.sendCloneText(ctx, client, peer, "Payment UPI ID updated!")
	}
}

func (bm *BotManager) promptClonePlans(ctx context.Context, client *telegram.Client, userID int64, token string) {
	peer := &tg.InputPeerUser{UserID: userID}
	_ = bm.sendCloneText(ctx, client, peer, "<b>Send your custom plans pricing description:</b>")
	val, err := bm.state.Listen(ctx, userID, 60*time.Second)
	if err == nil && val != "/cancel" {
		_ = bm.updateCloneField(ctx, token, "plans_details", val)
		_ = bm.sendCloneText(ctx, client, peer, "Checkout plans configuration updated!")
	}
}

func (bm *BotManager) promptCloneFSubCh(ctx context.Context, client *telegram.Client, userID int64, token string) {
	peer := &tg.InputPeerUser{UserID: userID}
	_ = bm.sendCloneText(ctx, client, peer, "<b>Send your FSub Channel ID and request approval boolean (yes/no) separated by a space:</b>\nExample: `-100123456789 yes`")
	val, err := bm.state.Listen(ctx, userID, 60*time.Second)
	if err == nil && val != "/cancel" {
		parts := strings.Fields(val)
		if len(parts) < 2 {
			_ = bm.sendCloneText(ctx, client, peer, "Invalid format.")
			return
		}
		chID, _ := strconv.ParseInt(parts[0], 10, 64)
		req := parts[1] == "yes" || parts[1] == "true"
		_ = bm.updateCloneField(ctx, token, "fsub_channel_id", chID)
		_ = bm.updateCloneField(ctx, token, "fsub_channel_req", req)
		_ = bm.sendCloneText(ctx, client, peer, "FSub channel updated successfully!")
	}
}

func (bm *BotManager) promptCloneDBCh(ctx context.Context, client *telegram.Client, userID int64, token string) {
	peer := &tg.InputPeerUser{UserID: userID}
	_ = bm.sendCloneText(ctx, client, peer, "<b>Send your dedicated database storage channel ID:</b>\nExample: `-100123456789`")
	val, err := bm.state.Listen(ctx, userID, 60*time.Second)
	if err == nil && val != "/cancel" {
		chID, _ := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
		_ = bm.updateCloneField(ctx, token, "db_channel_id", chID)
		_ = bm.sendCloneText(ctx, client, peer, "Database storage channel updated successfully!")
	}
}

func (bm *BotManager) updateCloneField(ctx context.Context, token, field string, value interface{}) error {
	encToken, _ := crypto.Encrypt(token, bm.mongo.TokenCryptKey())
	_, err := bm.mongo.DB.Collection("bots").UpdateOne(ctx,
		bson.M{
			"$or": []bson.M{
				{"token": encToken},
				{"token": token},
			},
		},
		bson.M{"$set": bson.M{field: value}},
	)
	return err
}

// Cloned Bot messengers
func (bm *BotManager) sendCloneText(ctx context.Context, client *telegram.Client, peer tg.InputPeerClass, text string) error {
	_, err := client.API().MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
		Peer:      peer,
		Message:   text,
		NoWebpage: true,
		RandomID:  getRandomID(),
	})
	return err
}

func (bm *BotManager) sendCloneTextWithMarkup(ctx context.Context, client *telegram.Client, peer tg.InputPeerClass, text string, markup tg.ReplyMarkupClass) error {
	_, err := client.API().MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
		Peer:        peer,
		Message:     text,
		ReplyMarkup: markup,
		NoWebpage:   true,
		RandomID:    getRandomID(),
	})
	return err
}

func (bm *BotManager) handleCloneBroadcast(ctx context.Context, client *telegram.Client, botID int64, message string, replyToMsgID int, ownerPeer tg.InputPeerClass) {
	users, err := bm.mongo.GetCloneUserbase(ctx, botID)
	if err != nil {
		bm.logger.Warn("Failed to fetch clone user base for broadcast", zap.Int64("bot_id", botID), zap.Error(err))
		return
	}

	total := len(users)
	if total == 0 {
		_ = bm.sendCloneText(ctx, client, ownerPeer, "Userbase is empty.")
		return
	}

	// Send initial status message to owner
	statusMsg, err := client.API().MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
		Peer:     ownerPeer,
		Message:  fmt.Sprintf("✦ %s ✦\n\nStarting broadcast...", ToSmallCaps("Broadcast Status")),
		RandomID: getRandomID(),
	})
	var statusMsgID int
	if updates, ok := statusMsg.(*tg.Updates); ok {
		for _, upd := range updates.Updates {
			if ev, ok := upd.(*tg.UpdateNewMessage); ok {
				if msg, ok := ev.Message.(*tg.Message); ok {
					statusMsgID = msg.ID
				}
			}
		}
	}

	success := 0
	fail := 0
	lastUpdate := time.Now()
	api := client.API()

	for idx, uID := range users {
		peer := &tg.InputPeerUser{UserID: uID}

		var sendErr error
		if replyToMsgID > 0 {
			_, sendErr = api.MessagesForwardMessages(ctx, &tg.MessagesForwardMessagesRequest{
				FromPeer:   ownerPeer,
				ID:         []int{replyToMsgID},
				ToPeer:     peer,
				DropAuthor: true, // Copy message including inline keyboards
			})
		} else {
			sendErr = bm.sendCloneText(ctx, client, peer, message)
		}

		if sendErr != nil {
			if d, ok := tgerr.AsFloodWait(sendErr); ok {
				if statusMsgID > 0 {
					_, _ = api.MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
						Peer:    ownerPeer,
						ID:      statusMsgID,
						Message: fmt.Sprintf("✦ %s ✦\n\n⚠️ **FLOOD WAIT**: Sleeping for %v\n\n%s\n›› **Success:** %d\n›› **Failed:** %d", ToSmallCaps("Broadcast Paused"), d, getProgressBar(idx, total), success, fail),
					})
				}
				time.Sleep(d)
				if replyToMsgID > 0 {
					_, sendErr = api.MessagesForwardMessages(ctx, &tg.MessagesForwardMessagesRequest{
						FromPeer:   ownerPeer,
						ID:         []int{replyToMsgID},
						ToPeer:     peer,
						DropAuthor: true,
					})
				} else {
					sendErr = bm.sendCloneText(ctx, client, peer, message)
				}
			}
		}

		if sendErr == nil {
			success++
		} else {
			fail++
		}

		if time.Since(lastUpdate) > 2*time.Second || idx == total-1 {
			if statusMsgID > 0 {
				_, _ = api.MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
					Peer:    ownerPeer,
					ID:      statusMsgID,
					Message: fmt.Sprintf("✦ %s ✦\n\n%s\n›› **Total Users:** %d\n›› **Success:** %d\n›› **Failed:** %d", ToSmallCaps("Broadcast Progress"), getProgressBar(idx+1, total), total, success, fail),
				})
			}
			lastUpdate = time.Now()
		}

		time.Sleep(100 * time.Millisecond) // avoid flood
	}

	if statusMsgID > 0 {
		_, _ = api.MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
			Peer:    ownerPeer,
			ID:      statusMsgID,
			Message: fmt.Sprintf("✦ %s ✦\n\n%s\n\n›› **Success:** %d\n›› **Failed:** %d", ToSmallCaps("Broadcast Complete"), getProgressBar(total, total), success, fail),
		})
	}
}

package telegram

import (
	"context"
	"encoding/base64"
	"fmt"
	"math"
	"runtime"
	"strconv"
	"strings"
	"time"

	"filestore/pkg/db"
	"filestore/pkg/shortener"
	"filestore/pkg/update"

	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"go.uber.org/zap"
)

var (
	styleGreen interface{} = nil
	styleRed   interface{} = nil
	styleBlue  interface{} = nil
)

func (bm *BotManager) handleMainCommand(ctx context.Context, userID int64, cmd string, args []string, msg *tg.Message) error {
	peer := &tg.InputPeerUser{UserID: userID}

	switch cmd {
	case "/start":
		return bm.handleStart(ctx, userID, args, msg)
	case "/settings":
		if !bm.isAdmin(userID) {
			return bm.sendText(ctx, peer, "Bakka! You are not my Senpai!")
		}
		return bm.sendSettingsPanel(ctx, peer)
	case "/mysettings":
		return bm.sendMySettingsPanel(ctx, peer, userID, 0)
	case "/clone":
		if !bm.config.CloneAllow {
			return bm.sendText(ctx, peer, "⚠️ Bot cloning is disabled by the administrator.")
		}
		go bm.handleClonePrompt(ctx, userID)
		return nil
	case "/deletecloned":
		if !bm.config.CloneAllow {
			return bm.sendText(ctx, peer, "⚠️ Bot cloning is disabled by the administrator.")
		}
		go bm.handleDeleteClonedPrompt(ctx, userID)
		return nil
	case "/stats":
		if !bm.isAdmin(userID) {
			return bm.sendText(ctx, peer, "Unauthorized.")
		}
		return bm.sendStats(ctx, peer)
	case "/users":
		if !bm.isAdmin(userID) {
			return bm.sendText(ctx, peer, "Unauthorized.")
		}
		count, _ := bm.mongo.UserCount(ctx)
		return bm.sendText(ctx, peer, fmt.Sprintf("<b>Total Users:</b> <code>%d</code>", count))
	case "/genlink":
		if !bm.isAdmin(userID) {
			return bm.sendText(ctx, peer, "Unauthorized.")
		}
		go bm.handleGenLink(ctx, userID)
		return nil
	case "/batch":
		if !bm.isAdmin(userID) {
			return bm.sendText(ctx, peer, "Unauthorized.")
		}
		go bm.handleBatchLink(ctx, userID)
		return nil
	case "/ban":
		if !bm.isAdmin(userID) {
			return bm.sendText(ctx, peer, "Unauthorized.")
		}
		if len(args) == 0 {
			return bm.sendText(ctx, peer, "Usage: /ban <user_id>")
		}
		targetID, _ := strconv.ParseInt(args[0], 10, 64)
		if targetID == 0 {
			return bm.sendText(ctx, peer, "Invalid user ID.")
		}
		_ = bm.mongo.BanUser(ctx, targetID)
		return bm.sendText(ctx, peer, fmt.Sprintf("✅ User <code>%d</code> has been banned.", targetID))
	case "/unban":
		if !bm.isAdmin(userID) {
			return bm.sendText(ctx, peer, "Unauthorized.")
		}
		if len(args) == 0 {
			return bm.sendText(ctx, peer, "Usage: /unban <user_id>")
		}
		targetID, _ := strconv.ParseInt(args[0], 10, 64)
		if targetID == 0 {
			return bm.sendText(ctx, peer, "Invalid user ID.")
		}
		_ = bm.mongo.UnbanUser(ctx, targetID)
		return bm.sendText(ctx, peer, fmt.Sprintf("✅ User <code>%d</code> has been unbanned.", targetID))
	case "/addpremium":
		if !bm.isAdmin(userID) {
			return bm.sendText(ctx, peer, "Unauthorized.")
		}
		if len(args) < 1 {
			return bm.sendText(ctx, peer, "Usage: /addpremium <user_id> [days]\nOmit days for permanent.")
		}
		targetID, _ := strconv.ParseInt(args[0], 10, 64)
		if targetID == 0 {
			return bm.sendText(ctx, peer, "Invalid user ID.")
		}
		var expiry *time.Time
		if len(args) >= 2 {
			days, _ := strconv.Atoi(args[1])
			if days > 0 {
				e := time.Now().Add(time.Duration(days) * 24 * time.Hour)
				expiry = &e
			}
		}
		_ = bm.mongo.AddPro(ctx, targetID, expiry)
		if expiry != nil {
			return bm.sendText(ctx, peer, fmt.Sprintf("✅ Premium granted to <code>%d</code> until %s", targetID, expiry.Format("2006-01-02")))
		}
		return bm.sendText(ctx, peer, fmt.Sprintf("✅ Permanent premium granted to <code>%d</code>", targetID))
	case "/delpremium":
		if !bm.isAdmin(userID) {
			return bm.sendText(ctx, peer, "Unauthorized.")
		}
		if len(args) == 0 {
			return bm.sendText(ctx, peer, "Usage: /delpremium <user_id>")
		}
		targetID, _ := strconv.ParseInt(args[0], 10, 64)
		_ = bm.mongo.RemovePro(ctx, targetID)
		return bm.sendText(ctx, peer, fmt.Sprintf("✅ Premium revoked from <code>%d</code>", targetID))
	case "/premiumusers":
		if !bm.isAdmin(userID) {
			return bm.sendText(ctx, peer, "Unauthorized.")
		}
		pros, err := bm.mongo.GetProsList(ctx)
		if err != nil || len(pros) == 0 {
			return bm.sendText(ctx, peer, "No premium users found.")
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("<b>Premium Users (%d):</b>\n", len(pros)))
		for _, p := range pros {
			expiryStr := "Permanent"
			if p.ExpiryDate != nil {
				expiryStr = p.ExpiryDate.Format("2006-01-02")
			}
			sb.WriteString(fmt.Sprintf("• <code>%d</code> — Expires: %s\n", p.ID, expiryStr))
		}
		return bm.sendText(ctx, peer, sb.String())
	case "/profile":
		if !bm.isAdmin(userID) {
			return bm.sendText(ctx, peer, "Unauthorized.")
		}
		if len(args) == 0 {
			return bm.sendText(ctx, peer, "Usage: /profile <user_id>")
		}
		targetID, _ := strconv.ParseInt(args[0], 10, 64)
		banned, _ := bm.mongo.IsBanned(ctx, targetID)
		isPro, _ := bm.mongo.IsPro(ctx, targetID)
		expiryDate, _ := bm.mongo.GetExpiryDate(ctx, targetID)
		expiryStr := "N/A"
		if expiryDate != nil {
			expiryStr = expiryDate.Format("2006-01-02 15:04")
		} else if isPro {
			expiryStr = "Permanent"
		}
		profileMsg := fmt.Sprintf("<blockquote>✦ %s ✦</blockquote>\n\n›› <b>User ID:</b> <code>%d</code>\n›› <b>Banned:</b> %t\n›› <b>Premium:</b> %t\n›› <b>Expiry:</b> %s",
			ToSmallCaps("User Profile"), targetID, banned, isPro, expiryStr)
		return bm.sendText(ctx, peer, profileMsg)
	case "/broadcast":
		if !bm.isAdmin(userID) {
			return bm.sendText(ctx, peer, "Unauthorized.")
		}
		var replyToMsgID int
		if msg.ReplyTo != nil {
			if header, ok := msg.ReplyTo.(*tg.MessageReplyHeader); ok {
				replyToMsgID = header.ReplyToMsgID
			}
		}
		if replyToMsgID == 0 && len(args) == 0 {
			return bm.sendText(ctx, peer, "Usage: Reply to a message with /broadcast, or use /broadcast <message_text>")
		}
		go bm.handleMainBroadcast(ctx, strings.Join(args, " "), replyToMsgID, peer)
		return bm.sendText(ctx, peer, "Broadcast started in background...")
	case "/update":
		if !bm.isAdmin(userID) {
			return bm.sendText(ctx, peer, "Unauthorized.")
		}
		_ = bm.sendText(ctx, peer, "🔄 <b>Checking for updates from upstream repository...</b>")

		res, err := update.PullUpstream(bm.config.UpstreamRepo, bm.config.UpstreamBranch, bm.config.GithubToken)
		if err != nil {
			return bm.sendText(ctx, peer, fmt.Sprintf("❌ <b>Update failed:</b> %s", err.Error()))
		}

		updateMsg := fmt.Sprintf("✅ <b>Update successful!</b>\n\n📌 <b>Commit ID:</b> <code>%s</code>\n💬 <b>Commit Message:</b> %s\n\n🔄 <i>Restarting bot process...</i>", res.CommitHash, res.CommitMessage)
		_ = bm.sendText(ctx, peer, updateMsg)
		time.Sleep(2 * time.Second)
		update.RestartProcess()
		return nil
	case "/restart":
		if !bm.isAdmin(userID) {
			return bm.sendText(ctx, peer, "Unauthorized.")
		}
		_ = bm.sendText(ctx, peer, "🔄 <b>Restarting bot process...</b>")
		time.Sleep(1 * time.Second)
		update.RestartProcess()
		return nil
	}
	return nil
}

func (bm *BotManager) handleStart(ctx context.Context, userID int64, args []string, msg *tg.Message) error {
	peer := &tg.InputPeerUser{UserID: userID}

	// Check if banned
	banned, err := bm.mongo.IsBanned(ctx, userID)
	if err == nil && banned {
		return bm.sendText(ctx, peer, "**You have been banned from using this bot!**")
	}

	// Add user if not exists
	_ = bm.mongo.AddUser(ctx, userID)

	// Normal start message if no argument
	if len(args) == 0 {
		startMsg := fmt.Sprintf("<b>Hey %d!</b>\n\n<blockquote>I am File Store Bot. I can store files in private channels and share download links.</blockquote>", userID)
		
		var rows [][]tg.KeyboardButtonClass
		if bm.isAdmin(userID) {
			rows = append(rows, []tg.KeyboardButtonClass{
				NewCallbackButtonWithStyle("⛩️ Settings ⛩️", "settings", styleGreen),
			})
		}
		rows = append(rows, []tg.KeyboardButtonClass{
			NewCallbackButtonWithStyle("Help", "help", styleGreen),
			NewCallbackButtonWithStyle("Close", "close", styleRed),
		})

		return bm.sendTextWithMarkup(ctx, peer, startMsg, NewInlineMarkup(rows))
	}

	// Start link decoding
	payload := args[0]
	isShortLink := false
	if strings.HasPrefix(payload, "yu3elk") {
		payload = payload[6 : len(payload)-1]
		isShortLink = true
	}

	// Check premium bypass
	isPremium, _ := bm.mongo.IsPro(ctx, userID)

	// If not premium AND shortener is active, generate shortlink
	if !isPremium && userID != bm.config.OwnerID && !isShortLink {
		shortDomain := "linkshortify.com"
		shortAPI := ""

		shortLink, err := shortener.GetShortlink(shortDomain, shortAPI, fmt.Sprintf("https://t.me/your_bot?start=yu3elk%s7", payload))
		if err == nil {
			caption := "<b>⌯ Here is Your Download Link. Must solve the shortener to continue.</b>"
			rows := [][]tg.KeyboardButtonClass{
				{
					NewURLButtonWithStyle("Download", shortLink, styleGreen),
				},
				{
					NewCallbackButtonWithStyle("Buy Premium", "premium", styleGreen),
				},
			}
			return bm.sendTextWithMarkup(ctx, peer, caption, NewInlineMarkup(rows))
		}
	}

	// Retrieve actual files
	decodedStr, err := base64Decode(payload)
	if err != nil {
		return bm.sendText(ctx, peer, "⚠️ Invalid or expired link.")
	}

	var fileIDs []int
	var targetChannelID int64 = bm.primaryDBID

	if strings.Contains(decodedStr, "_") {
		// New Fail-Safe format: get_<channelID>_<msgID> or get_<channelID>_<startID>_<endID>
		parts := strings.Split(decodedStr, "_")
		if len(parts) >= 3 && parts[0] == "get" {
			parsedChID, _ := strconv.ParseInt(parts[1], 10, 64)
			if parsedChID != 0 {
				targetChannelID = parsedChID
			}

			if len(parts) == 3 {
				// Single file
				msgID, _ := strconv.Atoi(parts[2])
				if msgID > 0 {
					fileIDs = []int{msgID}
				}
			} else if len(parts) == 4 {
				// Range batch
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
		// Legacy multiplier format: get-<startEncoded>-<endEncoded> or get-<encodedID>
		parts := strings.Split(decodedStr, "-")
		if len(parts) == 3 {
			startEncoded, _ := strconv.Atoi(parts[1])
			endEncoded, _ := strconv.Atoi(parts[2])

			mult := int(math.Abs(float64(bm.primaryDBID)))
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
			mult := int(math.Abs(float64(bm.primaryDBID)))
			if mult > 0 && encodedID%mult == 0 {
				fileIDs = []int{encodedID / mult}
			}
		}
	}

	if len(fileIDs) == 0 {
		return bm.sendText(ctx, peer, "⚠️ Files not found.")
	}

	// Copy messages to user
	api := bm.primary.API()
	inputChannel := &tg.InputPeerChannel{ChannelID: targetChannelID}

	// Fetch personal preferences
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
			bm.logger.Warn("Failed to forward/copy message", zap.Int("msg_id", msgID), zap.Error(err))
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
		time.Sleep(200 * time.Millisecond) // avoid flood
	}

	// Send auto-delete notification
	if autoDelDelay > 0 {
		delMinutes := autoDelDelay / 60
		if delMinutes == 0 {
			delMinutes = 1
		}
		notifMsg := fmt.Sprintf("⚠️ <b>These files will be auto-deleted in %d minute(s).</b>\nPlease save them before that.", delMinutes)
		_ = bm.sendText(ctx, peer, notifMsg)
	}

	return nil
}

func (bm *BotManager) handleClonePrompt(ctx context.Context, userID int64) error {
	peer := &tg.InputPeerUser{UserID: userID}

	// Step 1: Bot Token
	_ = bm.sendText(ctx, peer, "<b>[Step 1/3] Send your Bot Token created via @BotFather:</b>\n(or send /cancel to abort)")
	token, err := bm.state.Listen(ctx, userID, 60*time.Second)
	if err != nil || token == "/cancel" {
		return bm.sendText(ctx, peer, "Process cancelled.")
	}
	token = strings.TrimSpace(token)

	// Step 2: MongoDB URI
	_ = bm.sendText(ctx, peer, "<b>[Step 2/3] Send your MongoDB URI connection string:</b>\nSend <code>default</code> to use the default database, or send your custom MongoDB connection string (or /cancel).")
	mongoURI, err := bm.state.Listen(ctx, userID, 60*time.Second)
	if err != nil || mongoURI == "/cancel" {
		return bm.sendText(ctx, peer, "Process cancelled.")
	}
	mongoURI = strings.TrimSpace(mongoURI)
	if strings.ToLower(mongoURI) == "default" {
		mongoURI = ""
	}

	// Step 3: DB Storage Channel ID
	_ = bm.sendText(ctx, peer, "<b>[Step 3/3] Send your DB Storage Channel ID (e.g., <code>-1001234567890</code>):</b>\n⚠️ <i>Make sure you have added your clone bot as an Admin in this channel first!</i>")
	chIDStr, err := bm.state.Listen(ctx, userID, 60*time.Second)
	if err != nil || chIDStr == "/cancel" {
		return bm.sendText(ctx, peer, "Process cancelled.")
	}
	dbChannelID, _ := strconv.ParseInt(strings.TrimSpace(chIDStr), 10, 64)
	if dbChannelID == 0 {
		return bm.sendText(ctx, peer, "⚠️ Invalid Channel ID. Clone creation failed.")
	}

	_ = bm.sendText(ctx, peer, "⏳ <b>Verifying bot token & channel admin rights...</b>")

	username, err := bm.RegisterClone(ctx, token, mongoURI, dbChannelID, userID)
	if err != nil {
		return bm.sendText(ctx, peer, fmt.Sprintf("⚠️ <b>Clone setup failed:</b> %s", err.Error()))
	}

	return bm.sendText(ctx, peer, fmt.Sprintf("✅ <b>Successfully cloned bot: @%s!</b>\n\nYour bot is now live and linked to DB Channel <code>%d</code>.", username, dbChannelID))
}

func (bm *BotManager) handleDeleteClonedPrompt(ctx context.Context, userID int64) error {
	peer := &tg.InputPeerUser{UserID: userID}

	_ = bm.sendText(ctx, peer, "<b>Send the Bot Token of the clone you wish to delete.</b>")
	token, err := bm.state.Listen(ctx, userID, 60*time.Second)
	if err != nil {
		return bm.sendText(ctx, peer, "Timeout or cancelled.")
	}

	err = bm.DeregisterClone(ctx, token)
	if err != nil {
		return bm.sendText(ctx, peer, fmt.Sprintf("⚠️ Failed to delete bot: %s", err.Error()))
	}

	return bm.sendText(ctx, peer, "<b>Cloned bot deleted successfully from database and stopped.</b>")
}

func (bm *BotManager) handleGenLink(ctx context.Context, userID int64) error {
	peer := &tg.InputPeerUser{UserID: userID}

	_ = bm.sendText(ctx, peer, "Send the ID of the file inside your database channel:")
	idStr, err := bm.state.Listen(ctx, userID, 60*time.Second)
	if err != nil {
		return err
	}

	msgID, _ := strconv.Atoi(idStr)
	if msgID <= 0 {
		return bm.sendText(ctx, peer, "Invalid message ID.")
	}

	payload := base64Encode(fmt.Sprintf("get_%d_%d", bm.primaryDBID, msgID))
	username := bm.BotUsername()
	if username == "" {
		username = "your_bot"
	}
	link := fmt.Sprintf("https://t.me/%s?start=%s", username, payload)

	return bm.sendText(ctx, peer, fmt.Sprintf("<b>Here is your file link:</b>\n\n<code>%s</code>", link))
}

func (bm *BotManager) handleBatchLink(ctx context.Context, userID int64) error {
	peer := &tg.InputPeerUser{UserID: userID}

	_ = bm.sendText(ctx, peer, "Send First Message ID:")
	startStr, err := bm.state.Listen(ctx, userID, 60*time.Second)
	if err != nil {
		return err
	}
	startID, _ := strconv.Atoi(startStr)

	_ = bm.sendText(ctx, peer, "Send Last Message ID:")
	endStr, err := bm.state.Listen(ctx, userID, 60*time.Second)
	if err != nil {
		return err
	}
	endID, _ := strconv.Atoi(endStr)

	if startID <= 0 || endID <= 0 {
		return bm.sendText(ctx, peer, "Invalid message range IDs.")
	}

	payload := base64Encode(fmt.Sprintf("get_%d_%d_%d", bm.primaryDBID, startID, endID))
	username := bm.BotUsername()
	if username == "" {
		username = "your_bot"
	}
	link := fmt.Sprintf("https://t.me/%s?start=%s", username, payload)

	return bm.sendText(ctx, peer, fmt.Sprintf("<b>Here is your range batch link:</b>\n\n<code>%s</code>", link))
}

func (bm *BotManager) sendSettingsPanel(ctx context.Context, peer tg.InputPeerClass) error {
	msg := fmt.Sprintf("<blockquote>✦ %s ✦</blockquote>\n", ToSmallCaps("Settings Dashboard")) +
		"›› **FSub Channels:** " + strconv.Itoa(len(bm.fsubChannels)) + "\n" +
		"›› **DB Channels:** " + strconv.Itoa(len(bm.dbChannels)) + "\n" +
		"›› **Primary DB ID:** " + strconv.FormatInt(bm.primaryDBID, 10) + "\n"

	if bm.config.CloneAllow {
		msg += "›› **Clone Mode:** " + fmt.Sprintf("%t", bm.cloneMode) + "\n"
	}
	msg += "›› **Auto Delete:** " + strconv.Itoa(bm.config.AutoDel) + "s"

	rows := [][]tg.KeyboardButtonClass{
		{
			NewCallbackButtonWithStyle(ToSmallCaps("FSub Channels"), "set_fsub", styleGreen),
			NewCallbackButtonWithStyle(ToSmallCaps("DB Channels"), "set_db", styleGreen),
		},
		{
			NewCallbackButtonWithStyle(ToSmallCaps("Admins"), "set_admins", styleGreen),
			NewCallbackButtonWithStyle(ToSmallCaps("Auto Delete"), "set_autodel", styleGreen),
		},
	}

	if bm.config.CloneAllow {
		rows = append(rows, []tg.KeyboardButtonClass{
			NewCallbackButtonWithStyle(ToSmallCaps("Toggle Clone"), "toggle_clone", styleGreen),
		})
	}

	rows = append(rows, []tg.KeyboardButtonClass{
		NewCallbackButtonWithStyle(ToSmallCaps("Home"), "settings", styleGreen),
		NewCallbackButtonWithStyle(ToSmallCaps("Next >>"), "settings_page_2", styleGreen),
	})

	return bm.sendTextWithMarkup(ctx, peer, msg, NewInlineMarkup(rows))
}

func (bm *BotManager) sendSettingsPage2(ctx context.Context, peer tg.InputPeerClass, msgID int) error {
	msg := fmt.Sprintf("<blockquote>✦ %s ✦</blockquote>\nConfigure premium messages, layouts, and url shorteners:", ToSmallCaps("Settings Dashboard (Page 2)"))

	rows := [][]tg.KeyboardButtonClass{
		{
			NewCallbackButtonWithStyle(ToSmallCaps("Protect Content"), "set_protect", styleGreen),
			NewCallbackButtonWithStyle(ToSmallCaps("Photos Settings"), "set_photos", styleGreen),
		},
		{
			NewCallbackButtonWithStyle(ToSmallCaps("Texts Settings"), "set_texts", styleGreen),
			NewCallbackButtonWithStyle(ToSmallCaps("Shortener"), "set_shortener", styleGreen),
		},
		{
			NewCallbackButtonWithStyle(ToSmallCaps("<< Prev"), "settings", styleGreen),
			NewCallbackButtonWithStyle(ToSmallCaps("Home"), "settings", styleGreen),
		},
		{
			NewCallbackButtonWithStyle(ToSmallCaps("Close"), "close", styleRed),
		},
	}
	return bm.editMessage(ctx, peer, msgID, msg, NewInlineMarkup(rows))
}

func (bm *BotManager) sendFSubSettings(ctx context.Context, peer tg.InputPeerClass, msgID int) error {
	var listBuilder strings.Builder
	listBuilder.WriteString(fmt.Sprintf("<blockquote>✦ %s ✦</blockquote>\n", ToSmallCaps("FSub Channels Settings")))
	if len(bm.fsubChannels) == 0 {
		listBuilder.WriteString("_No Force Subscription Channels configured._")
	} else {
		for _, ch := range bm.fsubChannels {
			reqStr := "Request: ❌"
			if ch.RequestEnabled {
				reqStr = "Request: ✅"
			}
			listBuilder.WriteString(fmt.Sprintf("• **%s** (`%d`) - %s, Timer: %dm\n", ch.Name, ch.ID, reqStr, ch.TimerMinutes))
		}
	}

	rows := [][]tg.KeyboardButtonClass{
		{
			NewCallbackButtonWithStyle(ToSmallCaps("Add Channel"), "add_fsub", styleGreen),
			NewCallbackButtonWithStyle(ToSmallCaps("Remove Channel"), "rm_fsub", styleGreen),
		},
		{
			NewCallbackButtonWithStyle(ToSmallCaps("Back"), "settings", styleBlue),
		},
	}
	return bm.editMessage(ctx, peer, msgID, listBuilder.String(), NewInlineMarkup(rows))
}

func (bm *BotManager) sendDBSettings(ctx context.Context, peer tg.InputPeerClass, msgID int) error {
	var listBuilder strings.Builder
	listBuilder.WriteString(fmt.Sprintf("<blockquote>✦ %s ✦</blockquote>\n", ToSmallCaps("DB Channels Settings")))
	if len(bm.dbChannels) == 0 {
		listBuilder.WriteString("_No database channels configured._")
	} else {
		for _, ch := range bm.dbChannels {
			primStr := "Secondary"
			if ch.IsPrimary {
				primStr = "Primary ✅"
			}
			actStr := "Active 🟢"
			if !ch.IsActive {
				actStr = "Inactive 🔴"
			}
			listBuilder.WriteString(fmt.Sprintf("• **%s** (`%d`)\n  %s | %s\n\n", ch.Name, ch.ID, primStr, actStr))
		}
	}

	rows := [][]tg.KeyboardButtonClass{
		{
			NewCallbackButtonWithStyle(ToSmallCaps("Add DB Channel"), "add_db_ch", styleGreen),
			NewCallbackButtonWithStyle(ToSmallCaps("Remove DB Channel"), "rm_db_ch", styleGreen),
		},
		{
			NewCallbackButtonWithStyle(ToSmallCaps("Set Primary"), "set_prim_db", styleGreen),
			NewCallbackButtonWithStyle(ToSmallCaps("Toggle Status"), "toggle_db_act", styleGreen),
		},
		{
			NewCallbackButtonWithStyle(ToSmallCaps("Back"), "settings", styleBlue),
		},
	}
	return bm.editMessage(ctx, peer, msgID, listBuilder.String(), NewInlineMarkup(rows))
}

func (bm *BotManager) sendAdminsSettings(ctx context.Context, peer tg.InputPeerClass, msgID int) error {
	var listBuilder strings.Builder
	listBuilder.WriteString(fmt.Sprintf("<blockquote>✦ %s ✦</blockquote>\n", ToSmallCaps("Admins Management")))
	listBuilder.WriteString(fmt.Sprintf("• **Primary Owner:** <code>%d</code>\n", bm.config.OwnerID))

	bm.mu.Lock()
	admins := bm.admins
	bm.mu.Unlock()

	if len(admins) == 0 {
		listBuilder.WriteString("\n_No additional admins configured._")
	} else {
		listBuilder.WriteString("\n<b>Extra Dynamic Admins:</b>\n")
		for _, adminID := range admins {
			listBuilder.WriteString(fmt.Sprintf("• <code>%d</code>\n", adminID))
		}
	}

	rows := [][]tg.KeyboardButtonClass{
		{
			NewCallbackButtonWithStyle(ToSmallCaps("Add Admin"), "add_admin", styleGreen),
			NewCallbackButtonWithStyle(ToSmallCaps("Remove Admin"), "rm_admin", styleGreen),
		},
		{
			NewCallbackButtonWithStyle(ToSmallCaps("Back"), "settings", styleBlue),
		},
	}
	return bm.editMessage(ctx, peer, msgID, listBuilder.String(), NewInlineMarkup(rows))
}

func (bm *BotManager) sendPhotosSettings(ctx context.Context, peer tg.InputPeerClass, msgID int) error {
	msg := fmt.Sprintf("<blockquote>✦ %s ✦</blockquote>\nConfigure banner links for your bot start / force subscribe prompts:", ToSmallCaps("Photos/Media Settings"))

	rows := [][]tg.KeyboardButtonClass{
		{
			NewCallbackButtonWithStyle(ToSmallCaps("Set Start Photo"), "set_start_pic", styleGreen),
			NewCallbackButtonWithStyle(ToSmallCaps("Set FSub Photo"), "set_fsub_pic", styleGreen),
		},
		{
			NewCallbackButtonWithStyle(ToSmallCaps("Remove Start Photo"), "rm_start_pic", styleGreen),
			NewCallbackButtonWithStyle(ToSmallCaps("Remove FSub Photo"), "rm_fsub_pic", styleGreen),
		},
		{
			NewCallbackButtonWithStyle(ToSmallCaps("Back"), "settings_page_2", styleBlue),
		},
	}
	return bm.editMessage(ctx, peer, msgID, msg, NewInlineMarkup(rows))
}

func (bm *BotManager) sendTextsSettings(ctx context.Context, peer tg.InputPeerClass, msgID int) error {
	msg := fmt.Sprintf("<blockquote>✦ %s ✦</blockquote>\nCustomize greeting headers and fallback responses:", ToSmallCaps("Texts Configuration"))

	rows := [][]tg.KeyboardButtonClass{
		{
			NewCallbackButtonWithStyle(ToSmallCaps("Start Text"), "set_start_txt", styleGreen),
			NewCallbackButtonWithStyle(ToSmallCaps("FSub Text"), "set_fsub_txt", styleGreen),
		},
		{
			NewCallbackButtonWithStyle(ToSmallCaps("Reply Text"), "set_reply_txt", styleGreen),
			NewCallbackButtonWithStyle(ToSmallCaps("About Text"), "set_about_txt", styleGreen),
		},
		{
			NewCallbackButtonWithStyle(ToSmallCaps("Back"), "settings_page_2", styleBlue),
		},
	}
	return bm.editMessage(ctx, peer, msgID, msg, NewInlineMarkup(rows))
}

func (bm *BotManager) sendShortenerSettings(ctx context.Context, peer tg.InputPeerClass, msgID int) error {
	msg := fmt.Sprintf("<blockquote>✦ %s ✦</blockquote>\nManage advertising credentials to monetize your start links:", ToSmallCaps("URL Shortener Settings"))

	rows := [][]tg.KeyboardButtonClass{
		{
			NewCallbackButtonWithStyle(ToSmallCaps("Toggle Shortener"), "toggle_sh", styleGreen),
			NewCallbackButtonWithStyle(ToSmallCaps("Add Shortener"), "add_sh", styleGreen),
		},
		{
			NewCallbackButtonWithStyle(ToSmallCaps("Set Tutorial Link"), "set_tut", styleGreen),
			NewCallbackButtonWithStyle(ToSmallCaps("Test Shortener"), "test_sh", styleGreen),
		},
		{
			NewCallbackButtonWithStyle(ToSmallCaps("Back"), "settings_page_2", styleBlue),
		},
	}
	return bm.editMessage(ctx, peer, msgID, msg, NewInlineMarkup(rows))
}

func (bm *BotManager) handleMainCallback(ctx context.Context, u *tg.UpdateBotCallbackQuery) error {
	data := string(u.Data)
	peer := &tg.InputPeerUser{UserID: u.UserID}

	switch data {
	case "my_toggle_autodel":
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
		return bm.sendMySettingsPanel(ctx, peer, u.UserID, u.MsgID)

	case "my_toggle_protect":
		userPrefs, _ := bm.mongo.GetUserSettings(ctx, u.UserID)
		_ = bm.mongo.UpdateUserSettings(ctx, u.UserID, userPrefs.AutoDel, !userPrefs.Protect)
		return bm.sendMySettingsPanel(ctx, peer, u.UserID, u.MsgID)

	case "close":
		_, err := bm.primary.API().MessagesDeleteMessages(ctx, &tg.MessagesDeleteMessagesRequest{
			ID:     []int{u.MsgID},
			Revoke: true,
		})
		return err
	case "settings":
		return bm.sendSettingsPanel(ctx, peer)
	case "settings_page_2":
		return bm.sendSettingsPage2(ctx, peer, u.MsgID)
	case "set_fsub":
		return bm.sendFSubSettings(ctx, peer, u.MsgID)
	case "set_db":
		return bm.sendDBSettings(ctx, peer, u.MsgID)
	case "set_admins":
		return bm.sendAdminsSettings(ctx, peer, u.MsgID)
	case "set_photos":
		return bm.sendPhotosSettings(ctx, peer, u.MsgID)
	case "set_texts":
		return bm.sendTextsSettings(ctx, peer, u.MsgID)
	case "set_shortener":
		return bm.sendShortenerSettings(ctx, peer, u.MsgID)
	case "toggle_clone":
		mode := bm.ToggleCloneMode(ctx)
		return bm.sendText(ctx, peer, fmt.Sprintf("Clone bot activation mode is now: **%t**", mode))

	// Interactive prompts triggers (routed in background)
	case "add_admin":
		go bm.handleAddAdminPrompt(ctx, u.UserID)
	case "rm_admin":
		go bm.handleRemoveAdminPrompt(ctx, u.UserID)
	case "add_fsub":
		go bm.handleAddFSubPrompt(ctx, u.UserID)
	case "rm_fsub":
		go bm.handleRemoveFSubPrompt(ctx, u.UserID)
	case "add_db_ch":
		go bm.handleAddDBChannelPrompt(ctx, u.UserID)
	case "rm_db_ch":
		go bm.handleRemoveDBChannelPrompt(ctx, u.UserID)
	case "set_prim_db":
		go bm.handleSetPrimaryDBPrompt(ctx, u.UserID)
	case "toggle_db_act":
		go bm.handleToggleDBChannelPrompt(ctx, u.UserID)
	case "set_autodel":
		go bm.handleSetAutoDeletePrompt(ctx, u.UserID)
	}
	return nil
}

// Prompt Handlers (client.listen equivalents running in goroutines)

func (bm *BotManager) handleAddFSubPrompt(ctx context.Context, userID int64) {
	peer := &tg.InputPeerUser{UserID: userID}
	_ = bm.sendText(ctx, peer, "<b>Send channel ID, request boolean (yes/no), and timer (minutes) separated by a space:</b>\nExample: `-100123456789 yes 5`")

	resp, err := bm.state.Listen(ctx, userID, 60*time.Second)
	if err != nil {
		_ = bm.sendText(ctx, peer, "Timeout or cancelled.")
		return
	}

	parts := strings.Fields(resp)
	if len(parts) < 3 {
		_ = bm.sendText(ctx, peer, "Invalid format. Must specify ID, request, and timer.")
		return
	}

	chID, _ := strconv.ParseInt(parts[0], 10, 64)
	req := parts[1] == "yes" || parts[1] == "true"
	timer, _ := strconv.Atoi(parts[2])

	ch := db.FSubDoc{
		ID:             chID,
		Name:           fmt.Sprintf("Channel %d", chID),
		InviteLink:     "",
		RequestEnabled: req,
		TimerMinutes:   timer,
	}

	err = bm.mongo.AddFSubChannel(ctx, ch)
	if err != nil {
		_ = bm.sendText(ctx, peer, fmt.Sprintf("Failed to save channel: %s", err.Error()))
		return
	}

	bm.refreshSettings(ctx)
	_ = bm.sendText(ctx, peer, "FSub channel added successfully!")
}

func (bm *BotManager) handleRemoveFSubPrompt(ctx context.Context, userID int64) {
	peer := &tg.InputPeerUser{UserID: userID}
	_ = bm.sendText(ctx, peer, "<b>Send the Channel ID you want to remove:</b>")

	resp, err := bm.state.Listen(ctx, userID, 60*time.Second)
	if err != nil {
		_ = bm.sendText(ctx, peer, "Timeout or cancelled.")
		return
	}

	chID, _ := strconv.ParseInt(strings.TrimSpace(resp), 10, 64)
	err = bm.mongo.RemoveFSubChannel(ctx, chID)
	if err != nil {
		_ = bm.sendText(ctx, peer, fmt.Sprintf("Failed to remove channel: %s", err.Error()))
		return
	}

	bm.refreshSettings(ctx)
	_ = bm.sendText(ctx, peer, "FSub channel removed successfully!")
}

func (bm *BotManager) handleAddDBChannelPrompt(ctx context.Context, userID int64) {
	peer := &tg.InputPeerUser{UserID: userID}
	_ = bm.sendText(ctx, peer, "<b>Send the Database Channel ID you want to add:</b>\nExample: `-100123456789`")

	resp, err := bm.state.Listen(ctx, userID, 60*time.Second)
	if err != nil {
		_ = bm.sendText(ctx, peer, "Timeout or cancelled.")
		return
	}

	chID, _ := strconv.ParseInt(strings.TrimSpace(resp), 10, 64)
	ch := db.DBChannelDoc{
		ID:        chID,
		Name:      fmt.Sprintf("DB Storage %d", chID),
		IsPrimary: false,
		IsActive:  true,
	}

	err = bm.mongo.AddDBChannel(ctx, ch)
	if err != nil {
		_ = bm.sendText(ctx, peer, fmt.Sprintf("Failed to save DB channel: %s", err.Error()))
		return
	}

	bm.refreshSettings(ctx)
	_ = bm.sendText(ctx, peer, "Database channel added successfully!")
}

func (bm *BotManager) handleRemoveDBChannelPrompt(ctx context.Context, userID int64) {
	peer := &tg.InputPeerUser{UserID: userID}
	_ = bm.sendText(ctx, peer, "<b>Send the DB Channel ID you want to remove:</b>")

	resp, err := bm.state.Listen(ctx, userID, 60*time.Second)
	if err != nil {
		_ = bm.sendText(ctx, peer, "Timeout or cancelled.")
		return
	}

	chID, _ := strconv.ParseInt(strings.TrimSpace(resp), 10, 64)
	err = bm.mongo.RemoveDBChannel(ctx, chID)
	if err != nil {
		_ = bm.sendText(ctx, peer, fmt.Sprintf("Failed to remove DB channel: %s", err.Error()))
		return
	}

	bm.refreshSettings(ctx)
	_ = bm.sendText(ctx, peer, "Database channel removed successfully!")
}

func (bm *BotManager) handleSetPrimaryDBPrompt(ctx context.Context, userID int64) {
	peer := &tg.InputPeerUser{UserID: userID}
	_ = bm.sendText(ctx, peer, "<b>Send the DB Channel ID you want to set as primary:</b>")

	resp, err := bm.state.Listen(ctx, userID, 60*time.Second)
	if err != nil {
		_ = bm.sendText(ctx, peer, "Timeout or cancelled.")
		return
	}

	chID, _ := strconv.ParseInt(strings.TrimSpace(resp), 10, 64)
	err = bm.mongo.SetPrimaryDBChannel(ctx, chID)
	if err != nil {
		_ = bm.sendText(ctx, peer, fmt.Sprintf("Failed to set primary DB channel: %s", err.Error()))
		return
	}

	bm.refreshSettings(ctx)
	_ = bm.sendText(ctx, peer, "Primary database channel updated successfully!")
}

func (bm *BotManager) handleToggleDBChannelPrompt(ctx context.Context, userID int64) {
	peer := &tg.InputPeerUser{UserID: userID}
	_ = bm.sendText(ctx, peer, "<b>Send the DB Channel ID you want to toggle Active/Inactive:</b>")

	resp, err := bm.state.Listen(ctx, userID, 60*time.Second)
	if err != nil {
		_ = bm.sendText(ctx, peer, "Timeout or cancelled.")
		return
	}

	chID, _ := strconv.ParseInt(strings.TrimSpace(resp), 10, 64)
	newStatus, err := bm.mongo.ToggleDBChannelStatus(ctx, chID)
	if err != nil {
		_ = bm.sendText(ctx, peer, fmt.Sprintf("Failed to toggle DB channel status: %s", err.Error()))
		return
	}

	bm.refreshSettings(ctx)
	_ = bm.sendText(ctx, peer, fmt.Sprintf("Database channel status toggled! New status active = **%t**", newStatus))
}

func (bm *BotManager) handleSetAutoDeletePrompt(ctx context.Context, userID int64) {
	peer := &tg.InputPeerUser{UserID: userID}
	_ = bm.sendText(ctx, peer, "<b>Send the auto delete delay in seconds:</b>")

	resp, err := bm.state.Listen(ctx, userID, 60*time.Second)
	if err != nil {
		_ = bm.sendText(ctx, peer, "Timeout or cancelled.")
		return
	}

	val, err := strconv.Atoi(strings.TrimSpace(resp))
	if err != nil || val < 0 {
		_ = bm.sendText(ctx, peer, "Invalid number.")
		return
	}

	bm.config.AutoDel = val
	_ = bm.sendText(ctx, peer, fmt.Sprintf("Auto delete delay successfully set to **%d** seconds!", val))
}

func (bm *BotManager) handleMainTextMessage(ctx context.Context, userID int64, msg *tg.Message) error {
	peer := &tg.InputPeerUser{UserID: userID}

	// If admin sends/forwards a file, auto-generate download link
	if bm.isAdmin(userID) && msg.Media != nil {
		// Forward the file to the primary DB channel first
		if bm.primaryDBID != 0 {
			dbPeer := &tg.InputPeerChannel{ChannelID: bm.primaryDBID}
			updates, err := bm.primary.API().MessagesForwardMessages(ctx, &tg.MessagesForwardMessagesRequest{
				FromPeer: peer,
				ID:       []int{msg.ID},
				ToPeer:   dbPeer,
			})
			if err == nil {
				sentIDs := getSentMsgIDs(updates)
				if len(sentIDs) > 0 {
					mult := int(math.Abs(float64(bm.primaryDBID)))
					payload := base64Encode(fmt.Sprintf("get-%d", sentIDs[0]*mult))
					username := bm.BotUsername()
					if username == "" {
						username = "your_bot"
					}
					link := fmt.Sprintf("https://t.me/%s?start=%s", username, payload)
					return bm.sendText(ctx, peer, fmt.Sprintf("<b>File stored! Here is your link:</b>\n\n<code>%s</code>", link))
				}
			} else {
				bm.logger.Warn("Failed to forward file to DB channel", zap.Error(err))
			}
		}
	}

	return bm.sendText(ctx, peer, "Please start the bot via shared link to get files.")
}

func (bm *BotManager) isAdmin(userID int64) bool {
	if userID == bm.config.OwnerID {
		return true
	}
	bm.mu.Lock()
	defer bm.mu.Unlock()
	for _, id := range bm.admins {
		if id == userID {
			return true
		}
	}
	return false
}

func (bm *BotManager) handleAddAdminPrompt(ctx context.Context, userID int64) {
	peer := &tg.InputPeerUser{UserID: userID}
	_ = bm.sendText(ctx, peer, "<b>Send the User ID of the new Admin to add:</b>\n(e.g., <code>123456789</code>)")

	resp, err := bm.state.Listen(ctx, userID, 60*time.Second)
	if err != nil || resp == "/cancel" {
		_ = bm.sendText(ctx, peer, "Process cancelled.")
		return
	}

	newAdminID, _ := strconv.ParseInt(strings.TrimSpace(resp), 10, 64)
	if newAdminID == 0 {
		_ = bm.sendText(ctx, peer, "Invalid User ID.")
		return
	}

	currentAdmins, _ := bm.mongo.GetAdminsList(ctx)
	for _, id := range currentAdmins {
		if id == newAdminID {
			_ = bm.sendText(ctx, peer, fmt.Sprintf("User <code>%d</code> is already an Admin.", newAdminID))
			return
		}
	}

	newAdmins := append(currentAdmins, newAdminID)
	_ = bm.mongo.SetAdminsList(ctx, newAdmins)
	bm.refreshSettings(ctx)
	_ = bm.sendText(ctx, peer, fmt.Sprintf("✅ User <code>%d</code> added as an Admin successfully!", newAdminID))
}

func (bm *BotManager) handleRemoveAdminPrompt(ctx context.Context, userID int64) {
	peer := &tg.InputPeerUser{UserID: userID}
	_ = bm.sendText(ctx, peer, "<b>Send the User ID of the Admin to remove:</b>")

	resp, err := bm.state.Listen(ctx, userID, 60*time.Second)
	if err != nil || resp == "/cancel" {
		_ = bm.sendText(ctx, peer, "Process cancelled.")
		return
	}

	targetID, _ := strconv.ParseInt(strings.TrimSpace(resp), 10, 64)
	if targetID == 0 {
		_ = bm.sendText(ctx, peer, "Invalid User ID.")
		return
	}

	currentAdmins, _ := bm.mongo.GetAdminsList(ctx)
	var updated []int64
	found := false
	for _, id := range currentAdmins {
		if id == targetID {
			found = true
		} else {
			updated = append(updated, id)
		}
	}

	if !found {
		_ = bm.sendText(ctx, peer, fmt.Sprintf("User <code>%d</code> is not in the extra admins list.", targetID))
		return
	}

	_ = bm.mongo.SetAdminsList(ctx, updated)
	bm.refreshSettings(ctx)
	_ = bm.sendText(ctx, peer, fmt.Sprintf("✅ User <code>%d</code> removed from extra admins list.", targetID))
}

// Inline messengers
func (bm *BotManager) sendText(ctx context.Context, peer tg.InputPeerClass, text string) error {
	_, err := bm.primary.API().MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
		Peer:      peer,
		Message:   text,
		NoWebpage: true,
		RandomID:  getRandomID(),
	})
	return err
}

func (bm *BotManager) sendTextWithMarkup(ctx context.Context, peer tg.InputPeerClass, text string, markup tg.ReplyMarkupClass) error {
	_, err := bm.primary.API().MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
		Peer:        peer,
		Message:     text,
		ReplyMarkup: markup,
		NoWebpage:   true,
		RandomID:    getRandomID(),
	})
	return err
}

func (bm *BotManager) editMessage(ctx context.Context, peer tg.InputPeerClass, msgID int, text string, markup tg.ReplyMarkupClass) error {
	_, err := bm.primary.API().MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
		Peer:        peer,
		ID:          msgID,
		Message:     text,
		ReplyMarkup: markup,
		NoWebpage:   true,
	})
	return err
}

func (bm *BotManager) sendStats(ctx context.Context, peer tg.InputPeerClass) error {
	userCount, _ := bm.mongo.UserCount(ctx)
	cloneBots, _ := bm.mongo.GetClonedBots(ctx)
	prosList, _ := bm.mongo.GetProsList(ctx)
	uptime := time.Since(bm.uptime)
	hours := int(uptime.Hours())
	mins := int(uptime.Minutes()) % 60

	msg := fmt.Sprintf("<blockquote>✦ %s ✦</blockquote>\n\n"+
		"›› <b>Total Users:</b> <code>%d</code>\n"+
		"›› <b>Premium Users:</b> <code>%d</code>\n"+
		"›› <b>Active Clones:</b> <code>%d</code>\n"+
		"›› <b>FSub Channels:</b> <code>%d</code>\n"+
		"›› <b>DB Channels:</b> <code>%d</code>\n"+
		"›› <b>Uptime:</b> <code>%dh %dm</code>\n"+
		"›› <b>Go Version:</b> <code>%s</code>",
		ToSmallCaps("System Stats"),
		userCount, len(prosList), len(cloneBots),
		len(bm.fsubChannels), len(bm.dbChannels),
		hours, mins,
		runtime.Version())
	return bm.sendText(ctx, peer, msg)
}

// Helper Encoders
func base64Encode(s string) string {
	b := base64.URLEncoding.EncodeToString([]byte(s))
	return strings.TrimRight(b, "=")
}

func base64Decode(s string) (string, error) {
	if m := len(s) % 4; m != 0 {
		s += strings.Repeat("=", 4-m)
	}
	b, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (bm *BotManager) sendMySettingsPanel(ctx context.Context, peer tg.InputPeerClass, userID int64, editMsgID int) error {
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
			NewCallbackButtonWithStyle(ToSmallCaps("Toggle Auto Delete"), "my_toggle_autodel", styleGreen),
			NewCallbackButtonWithStyle(ToSmallCaps("Toggle Protect"), "my_toggle_protect", styleGreen),
		},
		{
			NewCallbackButtonWithStyle(ToSmallCaps("Close"), "close", styleRed),
		},
	}

	if editMsgID > 0 {
		return bm.editMessage(ctx, peer, editMsgID, msg, NewInlineMarkup(rows))
	}
	return bm.sendTextWithMarkup(ctx, peer, msg, NewInlineMarkup(rows))
}

func getSentMsgIDs(updates tg.UpdatesClass) []int {
	var ids []int
	switch u := updates.(type) {
	case *tg.Updates:
		for _, upd := range u.Updates {
			switch ev := upd.(type) {
			case *tg.UpdateNewMessage:
				if msg, ok := ev.Message.(*tg.Message); ok {
					ids = append(ids, msg.ID)
				}
			}
		}
	case *tg.UpdatesCombined:
		for _, upd := range u.Updates {
			switch ev := upd.(type) {
			case *tg.UpdateNewMessage:
				if msg, ok := ev.Message.(*tg.Message); ok {
					ids = append(ids, msg.ID)
				}
			}
		}
	case *tg.UpdateShortSentMessage:
		ids = append(ids, u.ID)
	}
	return ids
}

func (bm *BotManager) handleMainBroadcast(ctx context.Context, message string, replyToMsgID int, ownerPeer tg.InputPeerClass) {
	users, err := bm.mongo.FullUserbase(ctx)
	if err != nil {
		bm.logger.Warn("Failed to fetch user base for broadcast", zap.Error(err))
		return
	}

	total := len(users)
	if total == 0 {
		_ = bm.sendText(ctx, ownerPeer, "Userbase is empty.")
		return
	}

	// Send initial status message to owner
	statusMsg, err := bm.primary.API().MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
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
	api := bm.primary.API()

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
			sendErr = bm.sendText(ctx, peer, message)
		}

		if sendErr != nil {
			if d, ok := tgerr.AsFloodWait(sendErr); ok {
				if statusMsgID > 0 {
					_ = bm.editMessage(ctx, ownerPeer, statusMsgID, fmt.Sprintf("✦ %s ✦\n\n⚠️ **FLOOD WAIT**: Sleeping for %v\n\n%s\n›› **Success:** %d\n›› **Failed:** %d", ToSmallCaps("Broadcast Paused"), d, getProgressBar(idx, total), success, fail), nil)
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
					sendErr = bm.sendText(ctx, peer, message)
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
				_ = bm.editMessage(ctx, ownerPeer, statusMsgID, fmt.Sprintf("✦ %s ✦\n\n%s\n›› **Total Users:** %d\n›› **Success:** %d\n›› **Failed:** %d", ToSmallCaps("Broadcast Progress"), getProgressBar(idx+1, total), total, success, fail), nil)
			}
			lastUpdate = time.Now()
		}

		time.Sleep(100 * time.Millisecond) // avoid flood
	}

	if statusMsgID > 0 {
		_ = bm.editMessage(ctx, ownerPeer, statusMsgID, fmt.Sprintf("✦ %s ✦\n\n%s\n\n›› **Success:** %d\n›› **Failed:** %d", ToSmallCaps("Broadcast Complete"), getProgressBar(total, total), success, fail), nil)
	}
}

func getProgressBar(current, total int) string {
	if total == 0 {
		return "[░░░░░░░░░░] 0%"
	}
	pct := (current * 100) / total
	filled := pct / 10
	var sb strings.Builder
	sb.WriteString("[")
	for i := 0; i < 10; i++ {
		if i < filled {
			sb.WriteString("█")
		} else {
			sb.WriteString("░")
		}
	}
	sb.WriteString(fmt.Sprintf("] %d%%", pct))
	return sb.String()
}

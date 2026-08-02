package telegram

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"filestore/pkg/config"
	"filestore/pkg/db"

	"github.com/gotd/log/logzap"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"go.uber.org/zap"
)

type BotManager struct {
	mu           sync.Mutex
	config       *config.Config
	mongo        *db.MongoDB
	state        *StateRegistry
	primary      *telegram.Client
	clones       map[string]*telegram.Client
	cloneCancel  map[string]context.CancelFunc
	cloneMode    bool
	logger       *zap.Logger
	fsubChannels []db.FSubDoc
	dbChannels   []db.DBChannelDoc
	primaryDBID  int64
	botUsername  string
	uptime       time.Time
	admins       []int64
	channelHashes map[int64]int64
}

func NewBotManager(cfg *config.Config, mongoDB *db.MongoDB, logger *zap.Logger) *BotManager {
	return &BotManager{
		config:        cfg,
		mongo:         mongoDB,
		state:         NewStateRegistry(),
		clones:        make(map[string]*telegram.Client),
		cloneCancel:   make(map[string]context.CancelFunc),
		cloneMode:     true, // default on
		logger:        logger,
		uptime:        time.Now(),
		channelHashes: make(map[int64]int64),
	}
}

func (bm *BotManager) BotUsername() string {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	return bm.botUsername
}

func (bm *BotManager) Start(ctx context.Context) error {
	// Create sessions folder
	if err := os.MkdirAll(".sessions", 0700); err != nil {
		return err
	}

	// Load dynamic settings from DB
	bm.refreshSettings(ctx)

	// Start Primary Bot
	go func() {
		err := bm.runBot(ctx, bm.config.BotToken, false)
		if err != nil {
			bm.logger.Error("Primary bot crashed", zap.String("error", err.Error()))
		}
	}()

	// Start Cloned Bots from DB
	bots, err := bm.mongo.GetClonedBots(ctx)
	if err == nil {
		for _, b := range bots {
			bm.logger.Info("Restarting cloned bot", zap.String("username", b.Username))
			go func(token string) {
				if err := bm.runBot(ctx, token, true); err != nil {
					bm.logger.Warn("Failed to start cloned bot", zap.String("token", token), zap.String("error", err.Error()))
				}
			}(b.Token)
		}
	}

	return nil
}

func (bm *BotManager) runBot(ctx context.Context, token string, isClone bool) error {
	sessionPath := filepath.Join(".sessions", fmt.Sprintf("session_%s.json", hashToken(token)))
	sessionStorage := &telegram.FileSessionStorage{
		Path: sessionPath,
	}

	// Create update handler
	dispatcher := bm.createDispatcher(token, isClone)

	client := telegram.NewClient(bm.config.APIID, bm.config.APIHash, telegram.Options{
		SessionStorage: sessionStorage,
		UpdateHandler:  dispatcher,
		Logger:         logzap.New(bm.logger.Named(fmt.Sprintf("tg_client_%t", isClone)).WithOptions(zap.IncreaseLevel(zap.WarnLevel))),
	})

	bm.mu.Lock()
	if isClone {
		bm.clones[token] = client
	} else {
		bm.primary = client
	}
	bm.mu.Unlock()

	return client.Run(ctx, func(ctx context.Context) error {
		// Log in as a bot
		status, err := client.Auth().Status(ctx)
		if err != nil {
			return err
		}
		if !status.Authorized {
			if _, err := client.Auth().Bot(ctx, token); err != nil {
				return err
			}
		}

		// Keep connection alive and receive updates
		bm.logger.Info("Bot client logged in successfully", zap.Bool("is_clone", isClone))

		// Fetch and store bot username for non-clones
		if !isClone {
			u, err := client.Self(ctx)
			if err == nil {
				bm.mu.Lock()
				bm.botUsername = u.Username
				bm.mu.Unlock()
				bm.logger.Info("Main bot username resolved", zap.String("username", u.Username))
			}
		}

		// Automatically configure bot commands on startup
		if isClone {
			go func() {
				time.Sleep(3 * time.Second)
				_ = bm.setCloneBotCommands(ctx, client, 0)
			}()
		} else {
			go func() {
				time.Sleep(3 * time.Second)
				_ = bm.setMainBotCommands(ctx, client)
			}()
		}

		return telegram.RunUntilCanceled(ctx, client)
	})
}

func (bm *BotManager) RegisterClone(ctx context.Context, token, mongoURI string, dbChannelID int64, ownerID int64) (string, error) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if !bm.cloneMode {
		return "", fmt.Errorf("clone mode is disabled")
	}

	// Restrict clone bot limit dynamically from config
	count, err := bm.mongo.CountUserClonedBots(ctx, ownerID)
	if err == nil && count >= int64(bm.config.CloneLimit) {
		return "", fmt.Errorf("you have reached the limit of %d cloned bots", bm.config.CloneLimit)
	}

	if _, exists := bm.clones[token]; exists {
		return "", fmt.Errorf("bot is already running")
	}

	// Verify custom MongoDB connection if provided
	if mongoURI != "" {
		testCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		testDB, err := db.NewMongoDB(mongoURI, "filestore", bm.mongo.TokenCryptKey())
		if err != nil {
			return "", fmt.Errorf("invalid MongoDB connection string: %w", err)
		}
		if err := testDB.DB.Client().Ping(testCtx, nil); err != nil {
			return "", fmt.Errorf("cannot connect to custom MongoDB: %w", err)
		}
	}

	// Test the client connection and get details
	sessionPath := filepath.Join(".sessions", fmt.Sprintf("session_%s.json", hashToken(token)))
	sessionStorage := &telegram.FileSessionStorage{
		Path: sessionPath,
	}

	// Temp client to verify and fetch info
	verifyClient := telegram.NewClient(bm.config.APIID, bm.config.APIHash, telegram.Options{
		SessionStorage: sessionStorage,
		Logger:         logzap.New(bm.logger.Named("verify_clone").WithOptions(zap.IncreaseLevel(zap.WarnLevel))),
	})

	var botUser *tg.User
	err = verifyClient.Run(ctx, func(ctx context.Context) error {
		status, err := verifyClient.Auth().Status(ctx)
		if err != nil {
			return err
		}
		if !status.Authorized {
			if _, err := verifyClient.Auth().Bot(ctx, token); err != nil {
				return err
			}
		}
		u, err := verifyClient.Self(ctx)
		if err != nil {
			return err
		}
		botUser = u

		// Verify channel admin status
		if dbChannelID != 0 {
			api := verifyClient.API()
			inputChannel := &tg.InputChannel{ChannelID: dbChannelID}
			participant, err := api.ChannelsGetParticipant(ctx, &tg.ChannelsGetParticipantRequest{
				Channel:     inputChannel,
				Participant: &tg.InputPeerUser{UserID: botUser.ID},
			})
			if err != nil {
				return fmt.Errorf("cannot access channel %d (error: %v). Make sure the bot is added as admin", dbChannelID, err)
			}
			switch p := participant.Participant.(type) {
			case *tg.ChannelParticipantAdmin, *tg.ChannelParticipantCreator:
				// Bot is verified Admin or Creator!
			default:
				return fmt.Errorf("bot @%s is NOT an Admin in channel %d (status: %T). Please grant admin rights", botUser.Username, dbChannelID, p)
			}
		}
		return nil
	})

	if err != nil {
		return "", err
	}

	if botUser == nil {
		return "", fmt.Errorf("failed to fetch bot details")
	}

	// Persist in DB
	botDoc := db.ClonedBotDoc{
		BotID:       botUser.ID,
		Name:        botUser.FirstName,
		Token:       token,
		Username:    botUser.Username,
		UserID:      ownerID,
		MongoURI:    mongoURI,
		DBChannelID: dbChannelID,
	}
	if err := bm.mongo.AddClonedBot(ctx, botDoc); err != nil {
		return "", err
	}

	// Start bot in goroutine
	cloneCtx, cancel := context.WithCancel(ctx)
	bm.cloneCancel[token] = cancel

	go func() {
		if err := bm.runBot(cloneCtx, token, true); err != nil {
			bm.logger.Warn("Cloned bot stopped running", zap.String("token", token), zap.String("error", err.Error()))
		}
	}()

	return botUser.Username, nil
}

func (bm *BotManager) DeregisterClone(ctx context.Context, token string) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	// Cancel context
	if cancel, exists := bm.cloneCancel[token]; exists {
		cancel()
		delete(bm.cloneCancel, token)
	}

	delete(bm.clones, token)

	// Remove from DB
	return bm.mongo.RemoveClonedBot(ctx, token)
}

func (bm *BotManager) ToggleCloneMode(ctx context.Context) bool {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	bm.cloneMode = !bm.cloneMode
	return bm.cloneMode
}

func (bm *BotManager) refreshSettings(ctx context.Context) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	// Load DB channels
	dbChs, err := bm.mongo.GetDBChannels(ctx)
	if err == nil {
		bm.dbChannels = dbChs
		for _, ch := range dbChs {
			if ch.IsPrimary {
				bm.primaryDBID = ch.ID
			}
		}
	}

	// Load FSub channels
	fsubs, err := bm.mongo.GetFSubChannels(ctx)
	if err == nil {
		bm.fsubChannels = fsubs
	}

	// Load Dynamic Admins from DB
	dbAdmins, err := bm.mongo.GetAdminsList(ctx)
	if err == nil {
		bm.admins = dbAdmins
	}
}

func hashToken(token string) string {
	// Simple hashing for session filename security
	var hash uint32 = 5381
	for _, c := range token {
		hash = ((hash << 5) + hash) + uint32(c)
	}
	return fmt.Sprintf("%x", hash)
}

func (bm *BotManager) createDispatcher(token string, isClone bool) tg.UpdateDispatcher {
	dispatcher := tg.NewUpdateDispatcher()

	if isClone {
		bm.registerCloneHandlers(&dispatcher, token)
	} else {
		bm.registerMainHandlers(&dispatcher)
	}

	return dispatcher
}

func (bm *BotManager) setMainBotCommands(ctx context.Context, client *telegram.Client) error {
	api := client.API()

	// 1. Default commands (for normal users)
	defaultCmds := []tg.BotCommand{
		{Command: "start", Description: "Start the bot"},
		{Command: "mysettings", Description: "Downloader preferences"},
	}
	_, _ = api.BotsSetBotCommands(ctx, &tg.BotsSetBotCommandsRequest{
		Scope:    &tg.BotCommandScopeDefault{},
		LangCode: "en",
		Commands: defaultCmds,
	})
	return nil
}

func (bm *BotManager) SetupAdminCommands(ctx context.Context, userID, accessHash int64) {
	if !bm.isAdmin(userID) {
		return
	}
	api := bm.primary.API()

	adminCmds := []tg.BotCommand{
		{Command: "start", Description: "Start the bot"},
		{Command: "settings", Description: "Admin control panel"},
		{Command: "mysettings", Description: "Downloader preferences"},
		{Command: "stats", Description: "Check bot system metrics"},
		{Command: "users", Description: "Show total user count"},
		{Command: "genlink", Description: "Generate file sharing link"},
		{Command: "batch", Description: "Generate range batch link"},
		{Command: "broadcast", Description: "Broadcast message to all users"},
		{Command: "ban", Description: "Ban a user by ID"},
		{Command: "unban", Description: "Unban a user by ID"},
		{Command: "addpremium", Description: "Grant premium to a user"},
		{Command: "delpremium", Description: "Revoke premium from a user"},
		{Command: "premiumusers", Description: "List all premium users"},
		{Command: "profile", Description: "View user profile details"},
		{Command: "update", Description: "Update bot from upstream repo"},
		{Command: "restart", Description: "Restart the bot process"},
	}
	if bm.config.CloneAllow {
		adminCmds = append(adminCmds,
			tg.BotCommand{Command: "clone", Description: "Create clone bot instance"},
			tg.BotCommand{Command: "deletecloned", Description: "Delete clone bot instance"},
		)
	}

	adminPeer := &tg.InputPeerUser{UserID: userID, AccessHash: accessHash}
	_, _ = api.BotsSetBotCommands(ctx, &tg.BotsSetBotCommandsRequest{
		Scope:    &tg.BotCommandScopePeer{Peer: adminPeer},
		LangCode: "en",
		Commands: adminCmds,
	})
}

func (bm *BotManager) setCloneBotCommands(ctx context.Context, client *telegram.Client, _ int64) error {
	api := client.API()

	// 1. Default commands (for standard clone users)
	defaultCmds := []tg.BotCommand{
		{Command: "start", Description: "Start the bot"},
		{Command: "mysettings", Description: "Downloader preferences"},
		{Command: "premium", Description: "Check premium checkout details"},
	}
	_, _ = api.BotsSetBotCommands(ctx, &tg.BotsSetBotCommandsRequest{
		Scope:    &tg.BotCommandScopeDefault{},
		LangCode: "en",
		Commands: defaultCmds,
	})
	return nil
}

func (bm *BotManager) SetupCloneCommands(ctx context.Context, client *telegram.Client, ownerID, accessHash int64) {
	api := client.API()

	ownerCmds := []tg.BotCommand{
		{Command: "start", Description: "Start the bot"},
		{Command: "settings", Description: "Clone Settings Dashboard"},
		{Command: "mysettings", Description: "Downloader preferences"},
		{Command: "premium", Description: "Check premium checkout details"},
		{Command: "broadcast", Description: "Broadcast message to your user base"},
		{Command: "users", Description: "Show total user count"},
		{Command: "ban", Description: "Ban a user by ID"},
		{Command: "unban", Description: "Unban a user by ID"},
		{Command: "addpremium", Description: "Grant premium to a user"},
		{Command: "delpremium", Description: "Revoke premium from a user"},
		{Command: "premiumusers", Description: "List all premium users"},
		{Command: "profile", Description: "View user profile details"},
	}
	ownerPeer := &tg.InputPeerUser{UserID: ownerID, AccessHash: accessHash}
	_, _ = api.BotsSetBotCommands(ctx, &tg.BotsSetBotCommandsRequest{
		Scope:    &tg.BotCommandScopePeer{Peer: ownerPeer},
		LangCode: "en",
		Commands: ownerCmds,
	})
}

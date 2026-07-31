package db

import (
	"context"
	"time"

	"filestore/pkg/crypto"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type UserDoc struct {
	ID      int64 `bson:"_id"`
	Ban     bool  `bson:"ban"`
	AutoDel int   `bson:"auto_del,omitempty"` // User custom auto-delete in seconds
	Protect bool  `bson:"protect,omitempty"`  // User custom protect setting
}

type PremiumUser struct {
	ID         int64     `bson:"_id"`
	ExpiryDate *time.Time `bson:"expiry_date,omitempty"`
}

type ClonedBotDoc struct {
	BotID          int64  `bson:"bot_id"`
	Name           string `bson:"name"`
	Token          string `bson:"token"`
	Username       string `bson:"username"`
	UserID         int64  `bson:"user_id"` // Owner of this cloned bot
	MongoURI       string `bson:"mongo_uri,omitempty"`
	ShortenerAPI   string `bson:"shortener_api"`
	BaseSite       string `bson:"base_site"`
	CustomCaption  string `bson:"custom_caption"`
	StartText      string `bson:"start_text"`
	FSubText       string `bson:"fsub_text"`
	AboutText      string `bson:"about_text"`
	ReplyText      string `bson:"reply_text"`
	StartPhoto     string `bson:"start_photo"`
	FSubPhoto      string `bson:"fsub_photo"`
	UPIID          string `bson:"upi_id"`
	QRPic          string `bson:"qr_pic"`
	PlansDetails   string `bson:"plans_details"`
	FSubChannelID  int64  `bson:"fsub_channel_id"`
	FSubChannelReq bool   `bson:"fsub_channel_req"`
	DBChannelID    int64  `bson:"db_channel_id"`
}

type MongoDB struct {
	client        *mongo.Client
	DB            *mongo.Database
	users         *mongo.Collection
	premiumUsers  *mongo.Collection
	bots          *mongo.Collection
	fsubChannels  *mongo.Collection
	dbChannels    *mongo.Collection
	cloneUsers    *mongo.Collection
	tokenCryptKey string
}

func NewMongoDB(uri, dbName, tokenCryptKey string) (*MongoDB, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}

	dbObj := client.Database(dbName)
	return &MongoDB{
		client:        client,
		DB:            dbObj,
		users:         dbObj.Collection("users"),
		premiumUsers:  dbObj.Collection("pros"),
		bots:          dbObj.Collection("bots"),
		fsubChannels:  dbObj.Collection("fsub_channels"),
		dbChannels:    dbObj.Collection("db_channels"),
		cloneUsers:    dbObj.Collection("clone_users"),
		tokenCryptKey: tokenCryptKey,
	}, nil
}

// User Actions
func (m *MongoDB) AddUser(ctx context.Context, userID int64) error {
	_, err := m.users.UpdateOne(ctx,
		bson.M{"_id": userID},
		bson.M{"$setOnInsert": bson.M{"ban": false, "auto_del": 0, "protect": false}},
		options.Update().SetUpsert(true),
	)
	return err
}

func (m *MongoDB) UpdateUserSettings(ctx context.Context, userID int64, autoDel int, protect bool) error {
	_, err := m.users.UpdateOne(ctx,
		bson.M{"_id": userID},
		bson.M{"$set": bson.M{"auto_del": autoDel, "protect": protect}},
		options.Update().SetUpsert(true),
	)
	return err
}

func (m *MongoDB) GetUserSettings(ctx context.Context, userID int64) (UserDoc, error) {
	var user UserDoc
	err := m.users.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
	return user, err
}

func (m *MongoDB) CountUserClonedBots(ctx context.Context, userID int64) (int64, error) {
	return m.bots.CountDocuments(ctx, bson.M{"user_id": userID})
}

func (m *MongoDB) PresentUser(ctx context.Context, userID int64) (bool, error) {
	var user UserDoc
	err := m.users.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
	if err == mongo.ErrNoDocuments {
		return false, nil
	}
	return err == nil, err
}

func (m *MongoDB) BanUser(ctx context.Context, userID int64) error {
	_, err := m.users.UpdateOne(ctx,
		bson.M{"_id": userID},
		bson.M{"$set": bson.M{"ban": true}},
		options.Update().SetUpsert(true),
	)
	return err
}

func (m *MongoDB) UnbanUser(ctx context.Context, userID int64) error {
	_, err := m.users.UpdateOne(ctx,
		bson.M{"_id": userID},
		bson.M{"$set": bson.M{"ban": false}},
	)
	return err
}

func (m *MongoDB) IsBanned(ctx context.Context, userID int64) (bool, error) {
	var user UserDoc
	err := m.users.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
	if err == mongo.ErrNoDocuments {
		return false, nil
	}
	return user.Ban, err
}

func (m *MongoDB) FullUserbase(ctx context.Context) ([]int64, error) {
	cursor, err := m.users.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var users []int64
	for cursor.Next(ctx) {
		var doc struct {
			ID int64 `bson:"_id"`
		}
		if err := cursor.Decode(&doc); err == nil {
			users = append(users, doc.ID)
		}
	}
	return users, nil
}

// Premium Actions
func (m *MongoDB) AddPro(ctx context.Context, userID int64, expiry *time.Time) error {
	_, err := m.premiumUsers.UpdateOne(ctx,
		bson.M{"_id": userID},
		bson.M{"$set": bson.M{"expiry_date": expiry}},
		options.Update().SetUpsert(true),
	)
	return err
}

func (m *MongoDB) RemovePro(ctx context.Context, userID int64) error {
	_, err := m.premiumUsers.DeleteOne(ctx, bson.M{"_id": userID})
	return err
}

func (m *MongoDB) IsPro(ctx context.Context, userID int64) (bool, error) {
	var p PremiumUser
	err := m.premiumUsers.FindOne(ctx, bson.M{"_id": userID}).Decode(&p)
	if err == mongo.ErrNoDocuments {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if p.ExpiryDate == nil {
		return true, nil // Permanent premium
	}
	return p.ExpiryDate.After(time.Now()), nil
}

func (m *MongoDB) GetProsList(ctx context.Context) ([]PremiumUser, error) {
	cursor, err := m.premiumUsers.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var pros []PremiumUser
	if err := cursor.All(ctx, &pros); err != nil {
		return nil, err
	}
	return pros, nil
}

// Cloned Bots
func (m *MongoDB) AddClonedBot(ctx context.Context, bot ClonedBotDoc) error {
	encToken, err := crypto.Encrypt(bot.Token, m.tokenCryptKey)
	if err != nil {
		return err
	}
	bot.Token = encToken

	_, err = m.bots.UpdateOne(ctx,
		bson.M{"token": bot.Token},
		bson.M{"$set": bot},
		options.Update().SetUpsert(true),
	)
	return err
}

func (m *MongoDB) RemoveClonedBot(ctx context.Context, token string) error {
	encToken, _ := crypto.Encrypt(token, m.tokenCryptKey)
	// Try deleting either encrypted or raw token
	_, err := m.bots.DeleteMany(ctx, bson.M{
		"$or": []bson.M{
			{"token": encToken},
			{"token": token},
		},
	})
	return err
}

func (m *MongoDB) GetClonedBots(ctx context.Context) ([]ClonedBotDoc, error) {
	cursor, err := m.bots.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var bots []ClonedBotDoc
	if err := cursor.All(ctx, &bots); err != nil {
		return nil, err
	}

	for i, b := range bots {
		decToken, err := crypto.Decrypt(b.Token, m.tokenCryptKey)
		if err == nil {
			bots[i].Token = decToken
		}
	}
	return bots, nil
}

func (m *MongoDB) UpdateClonedBotShortener(ctx context.Context, token, api, baseSite string) error {
	encToken, _ := crypto.Encrypt(token, m.tokenCryptKey)
	_, err := m.bots.UpdateOne(ctx,
		bson.M{
			"$or": []bson.M{
				{"token": encToken},
				{"token": token},
			},
		},
		bson.M{"$set": bson.M{"shortener_api": api, "base_site": baseSite}},
	)
	return err
}

// FSub Channel Configuration
type FSubDoc struct {
	ID             int64  `bson:"_id"`
	Name           string `bson:"name"`
	InviteLink     string `bson:"invite_link"`
	RequestEnabled bool   `bson:"request_enabled"`
	TimerMinutes   int    `bson:"timer_minutes"`
}

func (m *MongoDB) AddFSubChannel(ctx context.Context, ch FSubDoc) error {
	_, err := m.fsubChannels.UpdateOne(ctx,
		bson.M{"_id": ch.ID},
		bson.M{"$set": ch},
		options.Update().SetUpsert(true),
	)
	return err
}

func (m *MongoDB) RemoveFSubChannel(ctx context.Context, channelID int64) error {
	_, err := m.fsubChannels.DeleteOne(ctx, bson.M{"_id": channelID})
	return err
}

func (m *MongoDB) GetFSubChannels(ctx context.Context) ([]FSubDoc, error) {
	cursor, err := m.fsubChannels.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var channels []FSubDoc
	if err := cursor.All(ctx, &channels); err != nil {
		return nil, err
	}
	return channels, nil
}

// Database Channels Configuration
type DBChannelDoc struct {
	ID        int64  `bson:"_id"`
	Name      string `bson:"name"`
	IsPrimary bool   `bson:"is_primary"`
	IsActive  bool   `bson:"is_active"`
}

func (m *MongoDB) AddDBChannel(ctx context.Context, ch DBChannelDoc) error {
	_, err := m.dbChannels.UpdateOne(ctx,
		bson.M{"_id": ch.ID},
		bson.M{"$set": ch},
		options.Update().SetUpsert(true),
	)
	return err
}

func (m *MongoDB) RemoveDBChannel(ctx context.Context, channelID int64) error {
	_, err := m.dbChannels.DeleteOne(ctx, bson.M{"_id": channelID})
	return err
}

func (m *MongoDB) GetDBChannels(ctx context.Context) ([]DBChannelDoc, error) {
	cursor, err := m.dbChannels.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var channels []DBChannelDoc
	if err := cursor.All(ctx, &channels); err != nil {
		return nil, err
	}
	return channels, nil
}

func (m *MongoDB) SetPrimaryDBChannel(ctx context.Context, channelID int64) error {
	// First set all channels to non-primary
	_, err := m.dbChannels.UpdateMany(ctx, bson.M{}, bson.M{"$set": bson.M{"is_primary": false}})
	if err != nil {
		return err
	}
	// Set the selected channel to primary
	_, err = m.dbChannels.UpdateOne(ctx, bson.M{"_id": channelID}, bson.M{"$set": bson.M{"is_primary": true}})
	return err
}

func (m *MongoDB) ToggleDBChannelStatus(ctx context.Context, channelID int64) (bool, error) {
	var ch DBChannelDoc
	err := m.dbChannels.FindOne(ctx, bson.M{"_id": channelID}).Decode(&ch)
	if err != nil {
		return false, err
	}
	newStatus := !ch.IsActive
	_, err = m.dbChannels.UpdateOne(ctx, bson.M{"_id": channelID}, bson.M{"$set": bson.M{"is_active": newStatus}})
	return newStatus, err
}

type CloneUserDoc struct {
	BotID  int64 `bson:"bot_id"`
	UserID int64 `bson:"user_id"`
}

func (m *MongoDB) AddCloneUser(ctx context.Context, botID, userID int64) error {
	_, err := m.cloneUsers.UpdateOne(ctx,
		bson.M{"bot_id": botID, "user_id": userID},
		bson.M{"$set": bson.M{"bot_id": botID, "user_id": userID}},
		options.Update().SetUpsert(true),
	)
	return err
}

func (m *MongoDB) GetCloneUserbase(ctx context.Context, botID int64) ([]int64, error) {
	cursor, err := m.cloneUsers.Find(ctx, bson.M{"bot_id": botID})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var users []int64
	for cursor.Next(ctx) {
		var doc CloneUserDoc
		if err := cursor.Decode(&doc); err == nil {
			users = append(users, doc.UserID)
		}
	}
	return users, nil
}

// User Count
func (m *MongoDB) UserCount(ctx context.Context) (int64, error) {
	return m.users.CountDocuments(ctx, bson.M{})
}

// Premium Expiry
func (m *MongoDB) GetExpiryDate(ctx context.Context, userID int64) (*time.Time, error) {
	var p PremiumUser
	err := m.premiumUsers.FindOne(ctx, bson.M{"_id": userID}).Decode(&p)
	if err != nil {
		return nil, err
	}
	return p.ExpiryDate, nil
}

// Admins List (stored in DB for persistence)
func (m *MongoDB) GetAdminsList(ctx context.Context) ([]int64, error) {
	var doc struct {
		Admins []int64 `bson:"admins"`
	}
	err := m.users.FindOne(ctx, bson.M{"_id": "admins_list"}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	return doc.Admins, err
}

func (m *MongoDB) SetAdminsList(ctx context.Context, admins []int64) error {
	_, err := m.users.UpdateOne(ctx,
		bson.M{"_id": "admins_list"},
		bson.M{"$set": bson.M{"admins": admins}},
		options.Update().SetUpsert(true),
	)
	return err
}

// Clone-scoped user ban/unban (each clone bot tracks bans in its own sub-collection)
func (m *MongoDB) BanCloneUser(ctx context.Context, botID, userID int64) error {
	_, err := m.cloneUsers.UpdateOne(ctx,
		bson.M{"bot_id": botID, "user_id": userID},
		bson.M{"$set": bson.M{"ban": true}},
		options.Update().SetUpsert(true),
	)
	return err
}

func (m *MongoDB) UnbanCloneUser(ctx context.Context, botID, userID int64) error {
	_, err := m.cloneUsers.UpdateOne(ctx,
		bson.M{"bot_id": botID, "user_id": userID},
		bson.M{"$set": bson.M{"ban": false}},
	)
	return err
}

func (m *MongoDB) IsCloneUserBanned(ctx context.Context, botID, userID int64) (bool, error) {
	var doc struct {
		Ban bool `bson:"ban"`
	}
	err := m.cloneUsers.FindOne(ctx, bson.M{"bot_id": botID, "user_id": userID}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return false, nil
	}
	return doc.Ban, err
}

// Clone-scoped premium (stored under clone_users collection with premium fields)
func (m *MongoDB) AddClonePro(ctx context.Context, botID, userID int64, expiry *time.Time) error {
	_, err := m.cloneUsers.UpdateOne(ctx,
		bson.M{"bot_id": botID, "user_id": userID},
		bson.M{"$set": bson.M{"pro": true, "pro_expiry": expiry}},
		options.Update().SetUpsert(true),
	)
	return err
}

func (m *MongoDB) RemoveClonePro(ctx context.Context, botID, userID int64) error {
	_, err := m.cloneUsers.UpdateOne(ctx,
		bson.M{"bot_id": botID, "user_id": userID},
		bson.M{"$set": bson.M{"pro": false, "pro_expiry": nil}},
	)
	return err
}

func (m *MongoDB) IsClonePro(ctx context.Context, botID, userID int64) (bool, error) {
	var doc struct {
		Pro       bool       `bson:"pro"`
		ProExpiry *time.Time `bson:"pro_expiry"`
	}
	err := m.cloneUsers.FindOne(ctx, bson.M{"bot_id": botID, "user_id": userID}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !doc.Pro {
		return false, nil
	}
	if doc.ProExpiry == nil {
		return true, nil // Permanent
	}
	return doc.ProExpiry.After(time.Now()), nil
}

func (m *MongoDB) GetCloneProsList(ctx context.Context, botID int64) ([]int64, error) {
	cursor, err := m.cloneUsers.Find(ctx, bson.M{"bot_id": botID, "pro": true})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var users []int64
	for cursor.Next(ctx) {
		var doc CloneUserDoc
		if err := cursor.Decode(&doc); err == nil {
			users = append(users, doc.UserID)
		}
	}
	return users, nil
}

func (m *MongoDB) CloneUserCount(ctx context.Context, botID int64) (int64, error) {
	return m.cloneUsers.CountDocuments(ctx, bson.M{"bot_id": botID})
}

// Export tokenCryptKey for bot.go
func (m *MongoDB) TokenCryptKey() string {
	return m.tokenCryptKey
}


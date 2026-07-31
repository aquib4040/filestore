package config

import (
	"os"
	"strconv"
)

type Config struct {
	APIID          int
	APIHash        string
	BotToken       string
	MongoURI       string
	DBName         string
	OwnerID        int64
	Port           string
	AutoDel        int
	DisableBtn     bool
	Protect        bool
	TokenCryptKey  string
	CloneLimit     int
	CloneAllow     bool
	FQDN           string
	UpstreamRepo   string
	UpstreamBranch string
	GithubToken    string
}

func LoadConfig() (*Config, error) {
	apiID, _ := strconv.Atoi(getEnv("API_ID", "0"))
	ownerID, _ := strconv.ParseInt(getEnv("OWNER_ID", "0"), 10, 64)
	autoDel, _ := strconv.Atoi(getEnv("AUTO_DEL", "300"))
	disableBtn, _ := strconv.ParseBool(getEnv("DISABLE_BTN", "true"))
	protect, _ := strconv.ParseBool(getEnv("PROTECT", "true"))
	cloneLimit, _ := strconv.Atoi(getEnv("CLONE_LIMIT", "3"))
	cloneAllow, _ := strconv.ParseBool(getEnv("CLONE_ALLOW", "true"))

	return &Config{
		APIID:          apiID,
		APIHash:        getEnv("API_HASH", ""),
		BotToken:       getEnv("BOT_TOKEN", ""),
		MongoURI:       getEnv("MONGO_URI", ""),
		DBName:         getEnv("DB_NAME", "filestore"),
		OwnerID:        ownerID,
		Port:           getEnv("PORT", "8080"),
		AutoDel:        autoDel,
		DisableBtn:     disableBtn,
		Protect:        protect,
		TokenCryptKey:  getEnv("TOKEN_ENCRYPTION_KEY", ""),
		CloneLimit:     cloneLimit,
		CloneAllow:     cloneAllow,
		FQDN:           getEnv("FQDN", ""),
		UpstreamRepo:   getEnv("UPSTREAM_REPO", "https://github.com/aquib4040/filestore.git"),
		UpstreamBranch: getEnv("UPSTREAM_BRANCH", "main"),
		GithubToken:    getEnv("GITHUB_TOKEN", ""),
	}, nil
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

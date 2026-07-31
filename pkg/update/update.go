package update

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type UpdateResult struct {
	CommitHash    string
	CommitMessage string
	Log           string
}

// PullUpstream pulls the latest code from upstream repo branch
func PullUpstream(repo, branch, token string) (*UpdateResult, error) {
	if repo == "" {
		repo = "https://github.com/aquib4040/filestore.git"
	}
	if branch == "" {
		branch = "main"
	}

	cleanRepo := strings.TrimSpace(repo)
	if strings.Contains(cleanRepo, "@github.com") {
		parts := strings.Split(cleanRepo, "@github.com/")
		if len(parts) >= 2 {
			cleanRepo = "https://github.com/" + parts[1]
		}
	}

	authRepo := cleanRepo
	if token != "" && strings.HasPrefix(cleanRepo, "https://") {
		authRepo = strings.Replace(cleanRepo, "https://", fmt.Sprintf("https://x-access-token:%s@", token), 1)
	}

	// Remove existing .git if present to ensure clean fetch
	_ = os.RemoveAll(".git")

	// Execute git commands
	initCmd := exec.Command("sh", "-c", fmt.Sprintf(
		"git init -q && "+
			"git config --global user.email 'mdaquinjawed1106@gmail.com' && "+
			"git config --global user.name 'aquib4040' && "+
			"git add . && git commit -sm 'update' -q && "+
			"git remote add origin %s && "+
			"git fetch %s %s -q && "+
			"git reset --hard FETCH_HEAD -q",
		cleanRepo, authRepo, branch,
	))

	var errBuf bytes.Buffer
	initCmd.Stderr = &errBuf
	if err := initCmd.Run(); err != nil {
		// Fallback for Windows cmd environment if sh is not available
		winCmd := exec.Command("cmd", "/c", fmt.Sprintf(
			"git init -q && "+
				"git config --global user.email \"filestore@bot.local\" && "+
				"git config --global user.name \"filestore-bot\" && "+
				"git add . && git commit -sm \"update\" -q && "+
				"git remote add origin %s && "+
				"git fetch %s %s -q && "+
				"git reset --hard FETCH_HEAD -q",
			cleanRepo, authRepo, branch,
		))
		winCmd.Stderr = &errBuf
		if errWin := winCmd.Run(); errWin != nil {
			return nil, fmt.Errorf("git update failed: %s", errBuf.String())
		}
	}

	// Get latest commit hash and message
	hashCmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	hashOut, err := hashCmd.Output()
	commitHash := strings.TrimSpace(string(hashOut))
	if err != nil || commitHash == "" {
		commitHash = "Unknown"
	}

	msgCmd := exec.Command("git", "log", "-1", "--pretty=%B")
	msgOut, _ := msgCmd.Output()
	commitMsg := strings.TrimSpace(string(msgOut))
	if lines := strings.Split(commitMsg, "\n"); len(lines) > 0 {
		commitMsg = lines[0]
	}

	return &UpdateResult{
		CommitHash:    commitHash,
		CommitMessage: commitMsg,
		Log:           fmt.Sprintf("UPSTREAM_REPO: %s | UPSTREAM_BRANCH: %s", cleanRepo, branch),
	}, nil
}

// RestartProcess terminates current process to allow container auto-restart
func RestartProcess() {
	execPath, err := os.Executable()
	if err == nil {
		_ = exec.Command(execPath, os.Args[1:]...).Start()
	}
	os.Exit(0)
}

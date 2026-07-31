package shortener

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type ShortenerResponse struct {
	Status       string `json:"status"`
	ShortenedUrl string `json:"shortenedUrl"`
	Message      string `json:"message"`
}

func GetShortlink(shortDomain, shortAPI, longURL string) (string, error) {
	if shortDomain == "" || shortAPI == "" {
		return longURL, nil
	}

	apiURL := fmt.Sprintf("https://%s/api?api=%s&url=%s", shortDomain, shortAPI, url.QueryEscape(longURL))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		return longURL, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return longURL, fmt.Errorf("bad status code: %d", resp.StatusCode)
	}

	var res ShortenerResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return longURL, err
	}

	if res.Status == "success" && res.ShortenedUrl != "" {
		return res.ShortenedUrl, nil
	}

	return longURL, fmt.Errorf("API error: %s", res.Message)
}

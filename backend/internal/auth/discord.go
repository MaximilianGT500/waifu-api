package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

type DiscordClient struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	HTTP         *http.Client

	TokenURL string
	UserURL  string

	once   sync.Once
	client *http.Client
}

type DiscordUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
}

type DiscordOAuthError struct {
	StatusCode int
}

func (e *DiscordOAuthError) Error() string {
	return fmt.Sprintf("Discord token exchange failed with status %d.", e.StatusCode)
}

var ErrDiscordProfile = errors.New("Failed to retrieve the Discord user profile.")

var (
	snowflakeRe  = regexp.MustCompile(`^[0-9]{5,25}$`)
	avatarHashRe = regexp.MustCompile(`^(a_)?[0-9a-fA-F]{8,64}$`)
)

func (c *DiscordClient) ExchangeCode(ctx context.Context, code string) (string, error) {
	form := url.Values{}
	form.Set("client_id", c.ClientID)
	form.Set("client_secret", c.ClientSecret)
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", c.RedirectURI)

	tokenURL := c.TokenURL
	if tokenURL == "" {
		tokenURL = "https://discord.com/api/oauth2/token"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http().Do(req)
	if err != nil {
		return "", err
	}
	defer drainAndClose(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", &DiscordOAuthError{StatusCode: resp.StatusCode}
	}

	var data struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&data); err != nil {
		return "", err
	}
	if data.AccessToken == "" {
		return "", fmt.Errorf("discord token response missing access_token")
	}
	return data.AccessToken, nil
}

func (c *DiscordClient) GetUserProfile(ctx context.Context, accessToken string) (*DiscordUser, error) {
	userURL := c.UserURL
	if userURL == "" {
		userURL = "https://discord.com/api/users/@me"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http().Do(req)
	if err != nil {
		return nil, err
	}
	defer drainAndClose(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, ErrDiscordProfile
	}

	var user DiscordUser
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&user); err != nil {
		return nil, ErrDiscordProfile
	}
	if !snowflakeRe.MatchString(user.ID) {
		return nil, ErrDiscordProfile
	}
	user.Username = strings.TrimSpace(user.Username)
	if user.Username == "" {
		user.Username = "user-" + user.ID
	}
	if r := []rune(user.Username); len(r) > 100 {
		user.Username = string(r[:100])
	}
	if user.Avatar != "" && !avatarHashRe.MatchString(user.Avatar) {
		user.Avatar = ""
	}
	return &user, nil
}

func (u *DiscordUser) AvatarURL() *string {
	if u.Avatar == "" {
		return nil
	}
	s := "https://cdn.discordapp.com/avatars/" + u.ID + "/" + u.Avatar + ".png"
	return &s
}

func (c *DiscordClient) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	c.once.Do(func() {
		c.client = &http.Client{
			Timeout:       15 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	})
	return c.client
}

func drainAndClose(body io.ReadCloser) {
	_, _ = io.Copy(io.Discard, io.LimitReader(body, 64<<10))
	_ = body.Close()
}

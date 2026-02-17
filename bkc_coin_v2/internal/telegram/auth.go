package telegram

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"sort"
	"strings"
)

type AuthUser struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
}

// VerifyWebAppInitData verifies Telegram WebApp initData using bot token.
// Returns (user, ok).
func VerifyWebAppInitData(initData string, botToken string) (AuthUser, bool) {
	initData = strings.TrimSpace(initData)
	if initData == "" {
		return AuthUser{}, false
	}

	vals, err := url.ParseQuery(initData)
	if err != nil {
		return AuthUser{}, false
	}

	providedHash := vals.Get("hash")
	if providedHash == "" {
		return AuthUser{}, false
	}
	vals.Del("hash")

	// data_check_string: key=value joined with \n, sorted by key
	keys := make([]string, 0, len(vals))
	for k := range vals {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+vals.Get(k))
	}
	dataCheck := strings.Join(parts, "\n")

	secret := hmac.New(sha256.New, []byte("WebAppData"))
	secret.Write([]byte(botToken))
	secretKey := secret.Sum(nil)

	mac := hmac.New(sha256.New, secretKey)
	mac.Write([]byte(dataCheck))
	expected := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expected), []byte(providedHash)) {
		return AuthUser{}, false
	}

	userRaw := vals.Get("user")
	if userRaw == "" {
		return AuthUser{}, false
	}

	var user AuthUser
	if err := json.Unmarshal([]byte(userRaw), &user); err != nil {
		return AuthUser{}, false
	}
	if user.ID == 0 {
		return AuthUser{}, false
	}
	if strings.TrimSpace(user.FirstName) == "" {
		user.FirstName = "User"
	}
	return user, true
}

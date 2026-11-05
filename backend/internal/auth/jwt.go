package auth

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Role int

const (
	RoleUser Role = iota
	RoleTrustedUser
	RoleModerator
	RoleAdmin
)

func (r Role) String() string {
	switch r {
	case RoleTrustedUser:
		return "TrustedUser"
	case RoleModerator:
		return "Moderator"
	case RoleAdmin:
		return "Admin"
	default:
		return "User"
	}
}

func ParseRole(s string) (Role, bool) {
	switch {
	case strings.EqualFold(s, "User"):
		return RoleUser, true
	case strings.EqualFold(s, "TrustedUser"):
		return RoleTrustedUser, true
	case strings.EqualFold(s, "Moderator"):
		return RoleModerator, true
	case strings.EqualFold(s, "Admin"):
		return RoleAdmin, true
	}
	return RoleUser, false
}

func RoleFromInt(i int) (Role, bool) {
	if i < int(RoleUser) || i > int(RoleAdmin) {
		return RoleUser, false
	}
	return Role(i), true
}

func MeetsMinRole(r Role, min string) bool {
	if min == "" || strings.EqualFold(min, "None") {
		return false
	}
	m, ok := ParseRole(min)
	if !ok {
		return false
	}
	return r >= m
}

func ReviewBypassed(r Role, configuredMin string) bool {
	if strings.EqualFold(configuredMin, "None") {
		return false
	}
	min := RoleTrustedUser
	if configuredMin != "" {
		if m, ok := ParseRole(configuredMin); ok {
			min = m
		}
	}
	return r >= min
}

func MeetsPermission(r Role, configuredMin string, defaultMin Role) bool {
	min := defaultMin
	if configuredMin != "" {
		if m, ok := ParseRole(configuredMin); ok {
			min = m
		}
	}
	return r >= min
}

const roleClaimURI = "http://schemas.microsoft.com/ws/2008/06/identity/claims/role"

const maxTokenLength = 4096

type Identity struct {
	UserID    int64
	Name      string
	Role      Role
	DiscordID string
	APIKeyID  *int64
}

type TokenClaims struct {
	UserID    int64
	Name      string
	Role      Role
	DiscordID string
	IssuedAt  time.Time
}

func IssueJWT(key []byte, issuer, audience string, userID int64, name string, role Role, discordID string, lifetime time.Duration) (string, error) {
	if lifetime <= 0 {
		lifetime = 7 * 24 * time.Hour
	}
	now := time.Now().UTC()
	claims := jwt.MapClaims{
		"sub":         strconv.FormatInt(userID, 10),
		"unique_name": name,
		roleClaimURI:  role.String(),
		"discord_id":  discordID,
		"iss":         issuer,
		"aud":         audience,
		"iat":         now.Unix(),
		"nbf":         now.Unix(),
		"exp":         now.Add(lifetime).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(key)
}

var errTokenTooLong = errors.New("token too long")

func ValidateJWT(tokenString string, key []byte, issuer, audience string) (*TokenClaims, error) {
	if len(tokenString) > maxTokenLength {
		return nil, errTokenTooLong
	}
	tok, err := jwt.Parse(tokenString, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return key, nil
	},
		jwt.WithValidMethods([]string{"HS256", "HS384", "HS512"}),
		jwt.WithIssuer(issuer),
		jwt.WithAudience(audience),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(30*time.Second),
	)
	if err != nil {
		return nil, err
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("invalid token claims")
	}

	sub, _ := claims["sub"].(string)
	userID, err := strconv.ParseInt(sub, 10, 64)
	if err != nil || userID <= 0 {
		return nil, errors.New("invalid subject claim")
	}
	roleStr, _ := claims[roleClaimURI].(string)
	role, ok := ParseRole(roleStr)
	if !ok {
		role = RoleUser
	}
	name, _ := claims["unique_name"].(string)
	discordID, _ := claims["discord_id"].(string)

	out := &TokenClaims{UserID: userID, Name: name, Role: role, DiscordID: discordID}
	if iat, err := claims.GetIssuedAt(); err == nil && iat != nil {
		out.IssuedAt = iat.Time
	}
	return out, nil
}

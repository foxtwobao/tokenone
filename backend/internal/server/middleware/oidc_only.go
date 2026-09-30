package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"

	"github.com/gin-gonic/gin"
)

const (
	oidcOnlyMessage      = "Only IDONE (OIDC) authentication is supported."
	oidcOnlyMaxBodyBytes = 64 << 10
	apiV1AuthPrefix      = "/api/v1/auth"
	apiV1UserPrefix      = "/api/v1/user"
)

// OIDCOnlyGuard keeps the application's user-facing authentication surface on
// the configured OIDC provider (IDONE). It is deliberately path-aware: the
// authenticated user API must remain available, while local credentials and
// other OAuth providers are rejected before their handlers run.
//
// The OIDC flow itself, including its pending completion/bind steps, remains
// available so existing accounts can be linked to IDONE without reopening a
// standalone local-login endpoint.
func OIDCOnlyGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions || !oidcOnlyBlocked(c) {
			c.Next()
			return
		}

		response.Forbidden(c, oidcOnlyMessage)
		c.Abort()
	}
}

func oidcOnlyBlocked(c *gin.Context) bool {
	path := strings.TrimRight(strings.ToLower(strings.TrimSpace(c.Request.URL.Path)), "/")
	if path == "" {
		return false
	}

	if strings.HasPrefix(path, apiV1AuthPrefix+"/oauth/") {
		return !oidcOnlyAllowedOAuthPath(path)
	}

	switch path {
	case apiV1AuthPrefix + "/register",
		apiV1AuthPrefix + "/login",
		apiV1AuthPrefix + "/login/2fa",
		apiV1AuthPrefix + "/passkey/login/begin",
		apiV1AuthPrefix + "/passkey/login/finish",
		apiV1AuthPrefix + "/send-verify-code",
		apiV1AuthPrefix + "/forgot-password",
		apiV1AuthPrefix + "/reset-password",
		apiV1AuthPrefix + "/validate-promo-code",
		apiV1AuthPrefix + "/validate-invitation-code":
		return true
	}

	if strings.HasPrefix(path, apiV1UserPrefix+"/passkeys") ||
		path == apiV1UserPrefix+"/password" ||
		strings.HasPrefix(path, apiV1UserPrefix+"/account-bindings/email") {
		return true
	}

	if path == apiV1UserPrefix+"/auth-identities/bind/start" {
		return !oidcOnlyRequestUsesOIDC(c)
	}

	return false
}

func oidcOnlyAllowedOAuthPath(path string) bool {
	// IDONE's OIDC flow and its generic pending continuation endpoints.
	if strings.HasPrefix(path, apiV1AuthPrefix+"/oauth/oidc/") ||
		path == apiV1AuthPrefix+"/oauth/oidc" ||
		strings.HasPrefix(path, apiV1AuthPrefix+"/oauth/pending/") {
		return true
	}

	// WeChat payment OAuth is not an authentication method and must not be
	// affected by the login-provider policy.
	if strings.HasPrefix(path, apiV1AuthPrefix+"/oauth/wechat/payment/") {
		return true
	}

	// This endpoint only transfers the already-authenticated session into the
	// short-lived OAuth bind cookie used by OIDC and other provider flows.
	return path == apiV1AuthPrefix+"/oauth/bind-token"
}

func oidcOnlyRequestUsesOIDC(c *gin.Context) bool {
	if c.Request.Body == nil {
		return false
	}

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, oidcOnlyMaxBodyBytes+1))
	if err != nil || len(body) > oidcOnlyMaxBodyBytes {
		return false
	}
	// Restore the body because the downstream handler still needs to decode it.
	c.Request.Body = io.NopCloser(bytes.NewReader(body))

	var request struct {
		Provider string `json:"provider"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(request.Provider), "oidc")
}

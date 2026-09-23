package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOIDCOnlyGuard(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
		wantBody   string
	}{
		{name: "blocks local login", method: http.MethodPost, path: "/api/v1/auth/login", wantStatus: http.StatusForbidden},
		{name: "blocks local registration", method: http.MethodPost, path: "/api/v1/auth/register", wantStatus: http.StatusForbidden},
		{name: "blocks another oauth provider", method: http.MethodGet, path: "/api/v1/auth/oauth/github/start", wantStatus: http.StatusForbidden},
		{name: "allows oidc start", method: http.MethodGet, path: "/api/v1/auth/oauth/oidc/start", wantStatus: http.StatusOK},
		{name: "allows oidc pending flow", method: http.MethodPost, path: "/api/v1/auth/oauth/pending/exchange", wantStatus: http.StatusOK},
		{name: "allows payment oauth", method: http.MethodGet, path: "/api/v1/auth/oauth/wechat/payment/callback", wantStatus: http.StatusOK},
		{name: "blocks password changes", method: http.MethodPut, path: "/api/v1/user/password", wantStatus: http.StatusForbidden},
		{name: "blocks non oidc binding", method: http.MethodPost, path: "/api/v1/user/auth-identities/bind/start", body: `{"provider":"github"}`, wantStatus: http.StatusForbidden},
		{name: "allows oidc binding and restores body", method: http.MethodPost, path: "/api/v1/user/auth-identities/bind/start", body: `{"provider":"oidc"}`, wantStatus: http.StatusOK, wantBody: `{"provider":"oidc"}`},
		{name: "leaves user api available", method: http.MethodGet, path: "/api/v1/user/profile", wantStatus: http.StatusOK},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.Use(OIDCOnlyGuard())
			r.Any("/*path", func(c *gin.Context) {
				if tc.wantBody != "" {
					body, err := io.ReadAll(c.Request.Body)
					require.NoError(t, err)
					c.String(http.StatusOK, string(body))
					return
				}
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, tc.wantStatus, w.Code)
			if tc.wantBody != "" {
				require.JSONEq(t, tc.wantBody, w.Body.String())
			}
		})
	}
}

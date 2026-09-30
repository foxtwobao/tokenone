package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOIDCRequestRedirectURL(t *testing.T) {
	const configured = "https://a.tokenone.work/api/v1/auth/oauth/oidc/callback"
	tests := []struct {
		name, configured, host, want string
	}{
		{"original host", configured, "a.tokenone.work", configured},
		{"other host", configured, "b.tokenone.work", "https://b.tokenone.work/api/v1/auth/oauth/oidc/callback"},
		{"port", configured, "b.tokenone.work:8443", "https://b.tokenone.work:8443/api/v1/auth/oauth/oidc/callback"},
		{"IPv6", configured, "[::1]:8443", "https://[::1]:8443/api/v1/auth/oauth/oidc/callback"},
		{"preserve scheme path and query", " http://a.tokenone.work:8080/custom%2Fcallback?app=one ", "b.tokenone.work", "http://b.tokenone.work/custom%2Fcallback?app=one"},
		{"empty config", "", "b.tokenone.work", ""},
		{"relative config", "/callback", "b.tokenone.work", ""},
		{"invalid config", "https://%/callback", "b.tokenone.work", ""},
		{"invalid scheme", "javascript://a.tokenone.work/callback", "b.tokenone.work", ""},
		{"userinfo", "https://user@a.tokenone.work/callback", "b.tokenone.work", ""},
		{"fragment", configured + "#fragment", "b.tokenone.work", ""},
		{"empty host", configured, "", ""},
		{"host with path", configured, "b.tokenone.work/path", ""},
		{"host with userinfo", configured, "a.tokenone.work@evil.example", ""},
		{"host with query", configured, "b.tokenone.work?x=y", ""},
		{"host with fragment", configured, "b.tokenone.work#x", ""},
		{"host with newline", configured, "b.tokenone.work\r\n", ""},
		{"invalid port", configured, "b.tokenone.work:bad", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &http.Request{Host: tt.host, Header: http.Header{
				"X-Forwarded-Host":  {"untrusted.example"},
				"X-Forwarded-Proto": {"http"},
			}}
			if got := oidcRequestRedirectURL(r, tt.configured); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
	if got := oidcRequestRedirectURL(nil, configured); got != "" {
		t.Fatalf("nil request: got %q", got)
	}
}

func TestOIDCRedirectMatchesDuringStartAndTokenExchange(t *testing.T) {
	for _, host := range []string{"a.tokenone.work", "b.tokenone.work:8443"} {
		t.Run(host, func(t *testing.T) {
			redirects := make(chan string, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := r.ParseForm(); err != nil {
					t.Error(err)
				}
				redirects <- r.PostForm.Get("redirect_uri")
				// Stop after observing the exchange; account creation is unrelated.
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			}))
			defer upstream.Close()
			handler := newOIDCOAuthTestHandler(t, false, config.OIDCConnectConfig{
				Enabled: true, ClientID: "client", ClientSecret: "secret",
				IssuerURL: upstream.URL, AuthorizeURL: upstream.URL + "/authorize",
				TokenURL: upstream.URL + "/token", Scopes: "openid",
				RedirectURL:         "https://configured.example/api/v1/auth/oauth/oidc/callback",
				FrontendRedirectURL: "/auth/oidc/callback", TokenAuthMethod: "client_secret_post",
			})
			start := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(start)
			c.Request = httptest.NewRequest(http.MethodGet, "https://"+host+"/api/v1/auth/oauth/oidc/start", nil)
			handler.OIDCOAuthStart(c)
			require.Equal(t, http.StatusFound, start.Code)
			authorize, err := url.Parse(start.Header().Get("Location"))
			require.NoError(t, err)
			want := "https://" + host + "/api/v1/auth/oauth/oidc/callback"
			require.Equal(t, want, authorize.Query().Get("redirect_uri"))
			require.NotEmpty(t, authorize.Query().Get("state"))
			callback := httptest.NewRecorder()
			c, _ = gin.CreateTestContext(callback)
			c.Request = httptest.NewRequest(http.MethodGet, want+"?code=test&state="+url.QueryEscape(authorize.Query().Get("state")), nil)
			for _, cookie := range start.Result().Cookies() {
				if cookie.MaxAge >= 0 {
					c.Request.AddCookie(cookie)
				}
			}
			handler.OIDCOAuthCallback(c)
			select {
			case got := <-redirects:
				require.Equal(t, want, got)
			default:
				t.Fatalf("token exchange was not reached: %s", callback.Header().Get("Location"))
			}
			require.Contains(t, callback.Header().Get("Location"), "/auth/oidc/callback")
		})
	}
}

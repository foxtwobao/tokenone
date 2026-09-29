package handler

import (
	"net/http"
	"net/url"
	"strings"
)

// oidcRequestRedirectURL keeps the configured scheme and callback path while
// returning to the host that initiated login. The proxy must preserve Host;
// forwarded headers are deliberately not used. The provider's registered
// redirect URIs determine which hosts may complete authorization.
// FrontendRedirectURL should be a relative path for same-host navigation.
func oidcRequestRedirectURL(r *http.Request, configured string) string {
	u, err := url.Parse(strings.TrimSpace(configured))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.Fragment != "" {
		return ""
	}
	if r == nil || r.Host == "" || strings.ContainsAny(r.Host, "/\\?#@ \t\r\n") {
		return ""
	}
	hostURL, err := url.Parse("https://" + r.Host)
	if err != nil || hostURL.Hostname() == "" {
		return ""
	}
	u.Host = r.Host
	return u.String()
}

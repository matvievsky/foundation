package gateway

import (
	"context"
	"net/http"
	"strings"

	fhttp "github.com/foundation-go/foundation/http"
	fhydra "github.com/foundation-go/foundation/hydra"
)

// TODO: MOVE TO AUTH PACKAGE

type Authenticate func(context.Context, string) (fhydra.Response, error)

// WithAuthenticationFn is a middleware that fetches the authentication details using ORY Hydra
func WithAuthenticationFn(fn Authenticate) func(http.Handler) http.Handler {
	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get the token from the request header
			token := r.Header.Get(fhttp.HeaderAuthorization)
			if token == "" {
				handler.ServeHTTP(w, r)
				return
			}
			// Strip any Bearer prefix
			tokenParts := strings.Split(token, " ")
			// Authenticate the token
			resp, err := fn(r.Context(), tokenParts[len(tokenParts)-1])
			if err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			// Set authentication headers
			if !resp.GetActive() {
				r.Header.Set(fhttp.HeaderXAuthenticated, "false")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			r.Header.Set(fhttp.HeaderXAuthenticated, "true")
			r.Header.Set(fhttp.HeaderXClientID, resp.GetClientId())
			r.Header.Set(fhttp.HeaderXScope, resp.GetScope())
			r.Header.Set(fhttp.HeaderXUserID, resp.GetSub())
			// Continue to the next handler
			handler.ServeHTTP(w, r)
		})
	}
}

// WithAuthenticationExceptions is a middleware that forces the request to be authenticated
func WithAuthenticationExceptions(exceptions []string) func(http.Handler) http.Handler {
	var exceptionsMap = make(map[string]struct{})

	for _, exception := range exceptions {
		exceptionsMap[exception] = struct{}{}
	}

	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if the request path is in the exceptions list
			if _, ok := exceptionsMap[r.URL.Path]; r.Header.Get(fhttp.HeaderXAuthenticated) != "true" && !ok {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			// Continue to the next handler
			handler.ServeHTTP(w, r)
		})
	}
}

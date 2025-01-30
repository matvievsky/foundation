package gateway

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	fhttp "github.com/foundation-go/foundation/http"
	fhydra "github.com/foundation-go/foundation/hydra"

	hydra "github.com/ory/hydra-client-go/v2"
)

func TestWithAuthenticationFn(t *testing.T) {
	// Create a mock authentication handler
	authentication := func(_ context.Context, token string) (fhydra.Response, error) {
		switch token {
		case "valid_token":
			sub := "user_id"
			return &hydra.IntrospectedOAuth2Token{Active: true, Sub: &sub}, nil
		case "invalid_token":
			return &hydra.IntrospectedOAuth2Token{Active: false, Sub: nil}, nil
		default:
			return nil, errors.New("server error")
		}
	}

	// Create a mock handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		xAuthenticated := ""
		xUserID := ""

		token := r.Header.Get(fhttp.HeaderAuthorization)
		switch token {
		case "valid_token":
			xAuthenticated = "true"
			xUserID = "user_id"
		case "invalid_token":
			xAuthenticated = "false"
			xUserID = ""
		}

		if r.Header.Get("X-Authenticated") != xAuthenticated {
			t.Errorf("Expected X-Authenticated header to be %s, but got %s", xAuthenticated, r.Header.Get("X-Authenticated"))
		}
		if r.Header.Get("X-User-Id") != xUserID {
			t.Errorf("Expected X-User-Id header to be %s, but got %s", xUserID, r.Header.Get("X-User-Id"))
		}

		// Write a response
		w.WriteHeader(http.StatusOK)
	})

	for _, tc := range []struct {
		name                         string
		token                        string
		expectedXAuthenticatedHeader string
		expectedStatus               int
	}{
		{
			name:           "Valid token",
			token:          "valid_token",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Invalid token",
			token:          "invalid_token",
			expectedStatus: http.StatusUnauthorized,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set(fhttp.HeaderAuthorization, tc.token)

			recorder := httptest.NewRecorder()

			// Call the middleware with the mock handler and authentication handler
			WithAuthenticationFn(authentication)(handler).ServeHTTP(recorder, req)

			// Check the response code
			if recorder.Code != tc.expectedStatus {
				t.Errorf("Expected status code %d, but got %d", tc.expectedStatus, recorder.Code)
			}
		})
	}
}

func TestWithAuthenticationExceptions(t *testing.T) {
	// Create a mock handler
	mockHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Write a response
		w.WriteHeader(http.StatusOK)
	})

	// Create a test request
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	// Test with authenticated request
	req.Header.Set(fhttp.HeaderXAuthenticated, "true")
	recorder := httptest.NewRecorder()
	WithAuthenticationExceptions([]string{})(mockHandler).ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Errorf("Expected status code %d, but got %d", http.StatusOK, recorder.Code)
	}

	// Test with unauthenticated request
	req.Header.Del(fhttp.HeaderXAuthenticated)
	recorder = httptest.NewRecorder()
	WithAuthenticationExceptions([]string{})(mockHandler).ServeHTTP(recorder, req)
	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("Expected status code %d, but got %d", http.StatusUnauthorized, recorder.Code)
	}

	// Test `except` option
	req.URL.Path = "/signup"
	recorder = httptest.NewRecorder()
	WithAuthenticationExceptions([]string{"/signup"})(mockHandler).ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Errorf("Expected status code %d, but got %d", http.StatusOK, recorder.Code)
	}
}

package gateway

import (
	"strings"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"

	gwruntime "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"

	fhttp "github.com/foundation-go/foundation/http"
)

// DefaultIncomingHeaderMatcher is the default incoming header matcher for the gateway.
//
// It matches all Foundation headers and uses the default matcher for all other headers.
func DefaultIncomingHeaderMatcher(key string) (string, bool) {
	for _, header := range fhttp.FoundationHeaders {
		if strings.EqualFold(header, key) {
			return key, true
		}
	}

	return runtime.DefaultHeaderMatcher(key)
}

// DefaultOutgoingHeaderMatcher is the default outgoing header matcher for the gateway.
//
// It matches all Foundation headers and uses the default matcher for all other headers.
func DefaultOutgoingHeaderMatcher(key string) (string, bool) {
	return DefaultIncomingHeaderMatcher(key)
}

// GetIncomingHeaderMatcherFunc is the header matcher for the incoming custom headers.
func GetIncomingHeaderMatcherFunc(fns ...gwruntime.HeaderMatcherFunc) gwruntime.HeaderMatcherFunc {
	return func(key string) (string, bool) {
		for _, fn := range fns {
			if key, ok := fn(key); ok {
				return key, ok
			}
		}

		return runtime.DefaultHeaderMatcher(key)
	}
}

// GetIncomingHeaderMatcherFunc is the header matcher for the outgoing custom headers.
func GetOutgoingHeaderMatcherFunc(fns ...gwruntime.HeaderMatcherFunc) gwruntime.HeaderMatcherFunc {
	return GetIncomingHeaderMatcherFunc(fns...)
}

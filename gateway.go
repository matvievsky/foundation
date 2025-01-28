package foundation

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"runtime"
	"time"

	"github.com/getsentry/sentry-go"
	gwruntime "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"

	"github.com/foundation-go/foundation/gateway"
	fhydra "github.com/foundation-go/foundation/hydra"
)

const (
	// GatewayDefaultTimeout is the default timeout for downstream services requests.
	GatewayDefaultTimeout = 30 * time.Second
)

// Gateway represents a gateway mode Foundation service.
type Gateway struct {
	*Service

	Options *GatewayOptions
}

// InitGateway initializes a new Foundation service in Gateway mode.
func InitGateway(name string) *Gateway {
	return &Gateway{
		Service: Init(name),
	}
}

// GatewayOptions represents the options for starting the Foundation gateway.
type GatewayOptions struct {
	// Services to register with the gateway
	Services []*gateway.Service
	// Timeout for downstream services requests (default: 30 seconds, if constructed with `NewGatewayOptions`)
	Timeout time.Duration
	// Middleware is a list of middleware to apply to the gateway. The middleware is applied in the order it is defined.
	Middleware []Middleware
	// StartComponentsOptions are the options to start the components.
	StartComponentsOptions []StartComponentsOption
	// CORSOptions are the options for CORS.
	CORSOptions *gateway.CORSOptions
	// Headers matchers
	IncomingHeadersMatchers []gwruntime.HeaderMatcherFunc
	OutgoingHeadersMatchers []gwruntime.HeaderMatcherFunc
	// AuthenticationExceptions is a list of paths that should not be authenticated
	AuthenticationExceptions []string
}

type Middleware func(http.Handler) http.Handler

// NewGatewayOptions returns a new GatewayOptions with default values.
func NewGatewayOptions() *GatewayOptions {
	return &GatewayOptions{
		Timeout:     GatewayDefaultTimeout,
		CORSOptions: gateway.NewCORSOptions(),
	}
}

// Start runs the Foundation gateway.
func (s *Gateway) Start(opts *GatewayOptions) {
	s.Options = opts

	s.Service.Start(&StartOptions{
		ModeName:               "gateway",
		StartComponentsOptions: s.Options.StartComponentsOptions,
		ServiceFunc:            s.ServiceFunc,
	})
}

func (s *Gateway) ServiceFunc(ctx context.Context) error {
	gwruntime.DefaultContextTimeout = s.Options.Timeout
	s.Logger.Debugf("Downstream requests timeout: %s", s.Options.Timeout)

	mux, err := gateway.RegisterServices(
		s.Options.Services,
		&gateway.RegisterServicesOptions{
			MuxOpts: []gwruntime.ServeMuxOption{
				gwruntime.WithIncomingHeaderMatcher(
					gateway.GetIncomingHeaderMatcherFunc(append([]gwruntime.HeaderMatcherFunc{gateway.DefaultIncomingHeaderMatcher}, s.Options.IncomingHeadersMatchers...)...)),
				gwruntime.WithOutgoingHeaderMatcher(
					gateway.GetOutgoingHeaderMatcherFunc(append([]gwruntime.HeaderMatcherFunc{gateway.DefaultOutgoingHeaderMatcher}, s.Options.OutgoingHeadersMatchers...)...)),
			},
			TLSDir: s.Config.GRPC.TLSDir,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to register services: %w", err)
	}

	s.Logger.Info("Using middleware:")
	for _, middleware := range append([]Middleware{
		gateway.WithRequestLogger(s.Logger),
		gateway.WithCORSEnabled(s.Options.CORSOptions),
		gateway.WithAuthenticationFn(fhydra.IntrospectedOAuth2Token),
		gateway.WithAuthenticationExceptions(s.Options.AuthenticationExceptions)},
		s.Options.Middleware...) {
		mux = middleware(mux)
		s.Logger.Infof(" - %s", runtime.FuncForPC(reflect.ValueOf(middleware).Pointer()).Name())
	}

	port := GetEnvOrInt("PORT", 51051)
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	s.Logger.Infof("Listening on http://0.0.0.0:%d", port)

	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			err = fmt.Errorf("failed to start server: %w", err)
			sentry.CaptureException(err)
			s.Logger.Fatal(err)
		}
	}()

	<-ctx.Done()

	// Gracefully stop the HTTP server
	if err := server.Shutdown(context.Background()); err != nil {
		err = fmt.Errorf("failed to gracefully shutdown HTTP server: %w", err)
		return err
	}

	return nil
}

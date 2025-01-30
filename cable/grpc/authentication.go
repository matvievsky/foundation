package cable_grpc

import (
	"context"
	"errors"

	fhydra "github.com/foundation-go/foundation/hydra"
)

func HydraAuthenticationFunc(ctx context.Context, accessToken string) (userID string, err error) {
	result, err := fhydra.IntrospectedOAuth2Token(ctx, accessToken)
	if err != nil {
		return "", err
	}

	if !result.GetActive() {
		return "", errors.New("token is not active")
	}

	return result.GetSub(), nil
}

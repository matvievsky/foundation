package main

import (
	f "github.com/foundation-go/foundation"
	fg "github.com/foundation-go/foundation/gateway"

	pb "github.com/foundation-go/foundation/examples/clubchat/protos/chats"
	fhydra "github.com/foundation-go/foundation/hydra"
)

var (
	svc = f.InitGateway("clubchat-gateway")

	services = []*fg.Service{
		{Name: "chats", Register: pb.RegisterChatsHandlerFromEndpoint},
	}
)

func main() {
	svc.Start(&f.GatewayOptions{
		Services:   services,
		Middleware: []f.Middleware{fg.WithAuthenticationFn(fhydra.IntrospectedOAuth2Token)},
	})
}

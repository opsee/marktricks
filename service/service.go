package service

import (
	"crypto/tls"
	"net/http"

	"golang.org/x/net/context"

	opsee "github.com/opsee/basic/service"
	"google.golang.org/grpc"

	client "github.com/dan-compton/go-kairosdb/client"
	"github.com/opsee/basic/grpcutil"
	"github.com/opsee/basic/tp"
	log "github.com/opsee/logrus"
)

type service struct {
	kclient    client.Client
	kdbAddress string
}

func New(kcConn string) (*service, error) {
	kc := client.NewHttpClient(kcConn)
	s := &service{
		kclient:    kc,
		kdbAddress: kcConn,
	}
	return s, nil
}

// http / grpc multiplexer for http health checks
func (s *service) StartMux(addr, certfile, certkeyfile string) error {
	router := tp.NewHTTPRouter(context.Background())
	server := grpc.NewServer()

	opsee.RegisterMarktricksServer(server, s)
	log.Infof("starting marktricks service at %s", addr)

	httpServer := &http.Server{
		Addr:      addr,
		Handler:   grpcutil.GRPCHandlerFunc(server, router),
		TLSConfig: &tls.Config{},
	}

	return httpServer.ListenAndServeTLS(certfile, certkeyfile)
}

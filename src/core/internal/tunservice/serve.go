package tunservice

import (
	"errors"
	"net"
	"time"

	"github.com/armanlife/firefox-vpn-client/core/internal/logx"
	"github.com/armanlife/firefox-vpn-client/core/internal/tunapi"
)

// serve listens on the service's channel for the users that users names,
// again after a failure, until the server closes.
func serve(server *Server, users func() []string) {
	for {
		ln, err := tunapi.Listen(users())
		if err != nil {
			logx.Error("creating the service's channel failed; retrying err=%s", logx.Err(err))
		} else if err := server.Serve(ln); err == nil || errors.Is(err, net.ErrClosed) {
			return
		} else {
			logx.Error("the service's channel failed; retrying err=%s", logx.Err(err))
		}
		select {
		case <-server.Ended():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

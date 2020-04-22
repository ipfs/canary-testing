package daemon

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/ipfs/testground/pkg/config"
	"github.com/ipfs/testground/pkg/engine"
	"github.com/ipfs/testground/pkg/logging"
	"github.com/pborman/uuid"
)

type Daemon struct {
	server *http.Server
	l      net.Listener
	doneCh chan struct{}
}

// New creates a new Daemon and attaches the following handlers:
//
// * GET /list: sends a `list` request to the daemon. list all test plans and test cases.
// * GET /describe: sends a `describe` request to the daemon. describes a test plan or test case.
// * POST /build: sends a `build` request to the daemon. builds a test plan.
// * POST /run: sends a `run` request to the daemon. (builds and) runs test case with name `<testplan>/<testcase>`.
// A type-safe client for this server can be found in the `pkg/client` package.
func New(cfg *config.EnvConfig) (srv *Daemon, err error) {
	srv = new(Daemon)

	engine, err := engine.NewDefaultEngine(cfg)
	if err != nil {
		return nil, err
	}

	r := mux.NewRouter()

	// Set a unique request ID.
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Header.Set("X-Request-ID", uuid.New()[:8])
			next.ServeHTTP(w, r)
		})
	})

	r.HandleFunc("/build", srv.buildHandler(engine)).Methods("POST")
	r.HandleFunc("/run", srv.runHandler(engine)).Methods("POST")
	r.HandleFunc("/outputs", srv.outputsHandler(engine)).Methods("POST")
	r.HandleFunc("/terminate", srv.terminateHandler(engine)).Methods("POST")
	r.HandleFunc("/healthcheck", srv.healthcheckHandler(engine)).Methods("POST")

	srv.doneCh = make(chan struct{})
	srv.server = &http.Server{
		Handler:      r,
		WriteTimeout: 1200 * time.Second,
		ReadTimeout:  1200 * time.Second,
	}

	srv.l, err = net.Listen("tcp", cfg.Daemon.Listen)
	if err != nil {
		return nil, err
	}

	return srv, nil
}

// Serve starts the server and blocks until the server is closed, either
// explicitly via Shutdown, or due to a fault condition. It propagates the
// non-nil err return value from http.Serve.
func (d *Daemon) Serve() error {
	select {
	case <-d.doneCh:
		return fmt.Errorf("tried to reuse a stopped server")
	default:
	}

	logging.S().Infow("daemon listening", "addr", d.Addr())
	return d.server.Serve(d.l)
}

func (d *Daemon) Addr() string {
	return d.l.Addr().String()
}

func (d *Daemon) Port() int {
	return d.l.Addr().(*net.TCPAddr).Port
}

func (d *Daemon) Shutdown(ctx context.Context) error {
	defer close(d.doneCh)
	return d.server.Shutdown(ctx)
}

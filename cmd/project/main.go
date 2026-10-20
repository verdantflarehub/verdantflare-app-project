package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/verdantflarehub/verdantflare-app-project/internal/artifactclient"
	"github.com/verdantflarehub/verdantflare-app-project/internal/buildinfo"
	"github.com/verdantflarehub/verdantflare-app-project/internal/coordination"
	"github.com/verdantflarehub/verdantflare-app-project/internal/dashboard"
	"github.com/verdantflarehub/verdantflare-app-project/internal/mcpregistration"
	"github.com/verdantflarehub/verdantflare-app-project/internal/project"
	"github.com/verdantflarehub/verdantflare-app-project/internal/worldclient"
	"github.com/verdantflarehub/verdantflare-app-project/migrations"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Project startup or runtime failed; check controlled configuration and schema state")
		os.Exit(1)
	}
}
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	dbURL := os.Getenv("PROJECT_DATABASE_URL")
	if dbURL == "" {
		return errors.New("database required")
	}
	pf, err := strconv.ParseInt(os.Getenv("PROJECT_COORD_FENCE"), 10, 64)
	if err != nil || pf < 1 {
		return coordination.ErrFenced
	}
	wf, err := strconv.ParseInt(os.Getenv("WORLD_COORD_FENCE"), 10, 64)
	if err != nil || wf < 1 {
		return coordination.ErrFenced
	}
	db, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if len(os.Args) == 2 && os.Args[1] == "migrate-fresh" {
		return migrations.ApplyFresh(ctx, db, pf, wf)
	}
	if len(os.Args) == 2 && os.Args[1] == "activate-fresh" {
		return migrations.ActivateFresh(ctx, db)
	}
	if len(os.Args) == 2 && os.Args[1] == "migrate-lifecycle" {
		return migrations.UpgradeLifecycle(ctx, db)
	}
	if len(os.Args) == 2 && os.Args[1] == "migrate-gates" {
		return migrations.UpgradeGates(ctx, db)
	}
	if len(os.Args) != 1 {
		return errors.New("unsupported command")
	}
	if err = migrations.CheckRuntime(ctx, db); err != nil {
		return err
	}
	serviceToken := os.Getenv("PROJECT_SERVICE_TOKEN")
	ownCoord, peerCoord := os.Getenv("PROJECT_COORD_TOKEN"), os.Getenv("WORLD_COORD_TOKEN")
	seen := map[string]bool{}
	for _, token := range []string{serviceToken, ownCoord, peerCoord, os.Getenv("WORLD_SERVICE_TOKEN"), os.Getenv("ARTIFACT_SERVICE_TOKEN")} {
		if len(token) < 32 || strings.ContainsAny(token, " \r\n\t") || seen[token] {
			return errors.New("independent credentials required")
		}
		seen[token] = true
	}
	content, err := artifactclient.NewWithCA(os.Getenv("ARTIFACT_URL"), os.Getenv("ARTIFACT_SERVICE_TOKEN"), os.Getenv("ARTIFACT_CA_FILE"))
	if err != nil {
		return err
	}
	world, err := worldclient.NewWithCA(os.Getenv("WORLD_SERVICE_URL"), os.Getenv("WORLD_SERVICE_TOKEN"), os.Getenv("WORLD_CA_FILE"))
	if err != nil {
		return err
	}
	peer, err := coordination.NewClientWithCA(os.Getenv("WORLD_SERVICE_URL"), ownCoord, os.Getenv("WORLD_CA_FILE"))
	if err != nil {
		return err
	}
	store, err := coordination.NewStore(db, "project", pf, project.AuthorizeReservation)
	if err != nil {
		return err
	}
	if err = store.Check(ctx, map[string]int64{"project": pf, "world": wf}); err != nil {
		return err
	}
	engine := &coordination.Engine{Store: store, Peers: map[string]coordination.Peer{"world": peer}}
	svc, err := project.NewService(ctx, db, content, world, engine)
	if err != nil {
		return err
	}
	internal, err := svc.HTTPHandler(serviceToken)
	if err != nil {
		return err
	}
	mcp, err := svc.MCPHandler(serviceToken)
	if err != nil {
		return err
	}
	coordinationHandler, err := engine.Handler(map[string]string{"world": peerCoord})
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.Handle("/internal/v1/coordination/", coordinationHandler)
	mux.Handle("/internal/v1/", internal)
	registerDashboardAPI(mux, internal)
	health := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.WriteHeader(405)
			return
		}
		c, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if db.Ping(c) != nil {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(204)
	})
	mux.Handle("/healthz", health)
	if directory := os.Getenv("PROJECT_DASHBOARD_DIR"); directory != "" {
		assets, loadErr := dashboard.Load(directory, buildinfo.Version)
		if loadErr != nil {
			return loadErr
		}
		handler, loadErr := assets.Handler(serviceToken)
		if loadErr != nil {
			return loadErr
		}
		mux.Handle("/internal/dashboard/", handler)
	}
	addr := os.Getenv("PROJECT_LISTEN")
	if addr == "" {
		addr = "127.0.0.1:8091"
	}
	server := &http.Server{Addr: addr, Handler: mcpregistration.BusinessHandler(mux), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 3 * time.Minute, IdleTimeout: time.Minute, MaxHeaderBytes: 16 << 10}
	cert, key := os.Getenv("PROJECT_TLS_CERT"), os.Getenv("PROJECT_TLS_KEY")
	if (cert == "") != (key == "") {
		return errors.New("both TLS files required")
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if cert == "" && (ip == nil || !ip.IsLoopback()) {
		return errors.New("non-loopback listener requires TLS")
	}
	mcpAddr, advertise, registryEndpoints := os.Getenv("PROJECT_MCP_LISTEN"), os.Getenv("PROJECT_MCP_ADVERTISE_URL"), os.Getenv("ETCD_ENDPOINTS")
	if err = mcpregistration.ValidateListener(addr, mcpAddr, advertise, registryEndpoints, cert != ""); err != nil {
		return err
	}
	readAddr, readToken := os.Getenv("PROJECT_READ_LISTEN"), os.Getenv("PROJECT_READ_TOKEN")
	if err = validateReadListener(addr, mcpAddr, readAddr, cert != "", readToken,
		[]string{serviceToken, ownCoord, peerCoord, os.Getenv("WORLD_SERVICE_TOKEN"), os.Getenv("ARTIFACT_SERVICE_TOKEN")}); err != nil {
		return err
	}
	if cert != "" {
		pair, loadErr := tls.LoadX509KeyPair(cert, key)
		if loadErr != nil {
			return errors.New("invalid listener TLS configuration")
		}
		server.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{pair}}
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return errors.New("listener unavailable")
	}
	defer listener.Close()
	done := make(chan error, 3)
	serve := func(server *http.Server, listener net.Listener) {
		if cert != "" {
			done <- server.ServeTLS(listener, "", "")
		} else {
			done <- server.Serve(listener)
		}
	}
	go serve(server, listener)
	defer server.Close()
	var protocolServer *http.Server
	if mcpAddr != "" {
		protocolListener, listenErr := net.Listen("tcp", mcpAddr)
		if listenErr != nil {
			return errors.New("MCP listener unavailable")
		}
		defer protocolListener.Close()
		protocolServer = &http.Server{Addr: mcpAddr, Handler: mcpregistration.MCPHandler(mcp, health), TLSConfig: server.TLSConfig, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 3 * time.Minute, IdleTimeout: time.Minute, MaxHeaderBytes: 16 << 10}
		go serve(protocolServer, protocolListener)
		defer protocolServer.Close()
	}
	var readServer *http.Server
	if readAddr != "" {
		readInternal, handlerErr := svc.HTTPHandler(readToken)
		if handlerErr != nil {
			return handlerErr
		}
		readListener, listenErr := net.Listen("tcp", readAddr)
		if listenErr != nil {
			return errors.New("read listener unavailable")
		}
		defer readListener.Close()
		readServer = &http.Server{Addr: readAddr, Handler: projectReadHandler(readInternal, health), TLSConfig: server.TLSConfig, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 3 * time.Minute, IdleTimeout: time.Minute, MaxHeaderBytes: 16 << 10}
		go serve(readServer, readListener)
		defer readServer.Close()
	}
	tools := []mcpregistration.ToolDefinition{}
	for _, tool := range project.MCPTools() {
		tools = append(tools, mcpregistration.ToolDefinition{Name: tool.Name, Description: tool.Description, InputSchema: tool.InputSchema})
	}
	publication, err := mcpregistration.Start(ctx, advertise, registryEndpoints, buildinfo.Version, tools)
	if err != nil {
		return err
	}
	if publication != nil {
		defer publication.Close()
	}
	worker, cancelWorker := context.WithCancel(ctx)
	defer cancelWorker()
	go svc.RunRecovery(worker)
	select {
	case err = <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return errors.New("Project HTTP server failed")
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err = server.Shutdown(shutdown)
		if protocolServer != nil {
			err = errors.Join(err, protocolServer.Shutdown(shutdown))
		}
		if readServer != nil {
			err = errors.Join(err, readServer.Shutdown(shutdown))
		}
		return err
	}
}

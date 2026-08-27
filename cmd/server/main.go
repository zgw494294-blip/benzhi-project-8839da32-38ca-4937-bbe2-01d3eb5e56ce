package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"stage-rigging-safety-release/internal/application"
	"stage-rigging-safety-release/internal/httpui"
	"stage-rigging-safety-release/internal/storage"
)

func main() {
	if err := run(); err != nil {
		log.Printf("服务退出：%v", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := parseConfig()
	if err != nil {
		return err
	}
	dbPath := cfg.database
	if cfg.selfcheck {
		file, err := os.CreateTemp("", "rigging-selfcheck-*.db")
		if err != nil {
			return err
		}
		dbPath = file.Name()
		_ = file.Close()
		defer os.Remove(dbPath)
		defer os.Remove(dbPath + "-wal")
		defer os.Remove(dbPath + "-shm")
	}
	repo, err := storage.Open(dbPath)
	if err != nil {
		return err
	}
	defer repo.Close()
	app := application.New(repo)
	ui := httpui.New(app)
	listener, err := net.Listen("tcp", cfg.addr)
	if err != nil {
		return fmt.Errorf("监听 %s: %w", cfg.addr, err)
	}
	server := &http.Server{Handler: ui.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	errCh := make(chan error, 1)
	go func() { errCh <- server.Serve(listener) }()
	if cfg.selfcheck {
		return runSelfcheckMode(server, listener, errCh, cfg.selfcheckTimeout)
	}
	log.Printf("舞台吊挂安全检验工作台已启动：http://%s", listener.Addr())
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case <-ctx.Done():
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}

func runSelfcheckMode(server *http.Server, listener net.Listener, errCh <-chan error, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	baseURL := "http://" + listener.Addr().String()
	checkErr := selfcheck(ctx, baseURL)
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer shutdownCancel()
	shutdownErr := server.Shutdown(shutdownCtx)
	select {
	case serveErr := <-errCh:
		if !errors.Is(serveErr, http.ErrServerClosed) && checkErr == nil {
			checkErr = serveErr
		}
	default:
	}
	if checkErr != nil {
		return fmt.Errorf("selfcheck 失败: %w", checkErr)
	}
	if shutdownErr != nil {
		return shutdownErr
	}
	log.Printf("selfcheck 通过：建档、方案、实测、整改复验、复核、冻结、签发和验真均成功")
	return nil
}

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

	"manuscript-conservation-gate/internal/application"
	"manuscript-conservation-gate/internal/audit"
	"manuscript-conservation-gate/internal/httpui"
	"manuscript-conservation-gate/internal/store"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Printf("conservation: %v", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	configuration, err := parseConfig(args)
	if err != nil {
		return err
	}
	dataDir := configuration.dataDir
	if configuration.selfcheck {
		temporary, err := os.MkdirTemp("", "conservation-selfcheck-")
		if err != nil {
			return fmt.Errorf("创建自检数据目录: %w", err)
		}
		defer os.RemoveAll(temporary)
		dataDir = temporary
	}
	repository, err := store.Open(dataDir)
	if err != nil {
		return err
	}
	issuer, err := audit.NewIssuer(configuration.secret)
	if err != nil {
		return err
	}
	service := application.NewService(repository, issuer)
	listener, err := net.Listen("tcp", configuration.address)
	if err != nil {
		return fmt.Errorf("监听 %s: %w", configuration.address, err)
	}
	server := &http.Server{Addr: configuration.address, Handler: httpui.New(service), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	serveErrors := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErrors <- err
		}
		close(serveErrors)
	}()
	if configuration.selfcheck {
		checkErr := runSelfcheck(context.Background(), "http://"+listener.Addr().String())
		shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdownErr := server.Shutdown(shutdownContext)
		if checkErr != nil {
			return checkErr
		}
		if shutdownErr != nil {
			return fmt.Errorf("关闭自检服务: %w", shutdownErr)
		}
		if serveErr := <-serveErrors; serveErr != nil {
			return serveErr
		}
		fmt.Println("自检通过：完整修复治理流程、凭据验证与数据完整性检查均成功")
		return nil
	}
	log.Printf("古籍修复放行门禁已启动：http://%s", listener.Addr().String())
	signalContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case <-signalContext.Done():
	case serveErr := <-serveErrors:
		if serveErr != nil {
			return serveErr
		}
		return nil
	}
	shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		return fmt.Errorf("优雅关闭服务: %w", err)
	}
	return nil
}

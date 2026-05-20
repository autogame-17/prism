package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"prism/backend/embed"
)

func main() {
	listenAddr := flag.String("listen", "", "override Prism listen address, e.g. 127.0.0.1:39527")
	dataDir := flag.String("data-dir", "", "override Prism data directory")
	flag.Parse()

	res, err := embed.Boot(embed.BootOptions{
		AppName:    "Prism",
		DataDir:    *dataDir,
		ListenAddr: *listenAddr,
	})
	if err != nil {
		log.Fatalf("prism headless boot failed: %v", err)
	}
	defer embed.Shutdown(res)

	fmt.Printf("Prism headless listening on http://%s\n", res.HTTPAddr)
	fmt.Printf("Data dir: %s\n", res.DataDir)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	if err := res.Server.Shutdown(context.Background()); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("shutdown: %v", err)
	}
}

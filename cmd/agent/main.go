package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mesnet/mesnet/internal/agent"
	"github.com/mesnet/mesnet/internal/version"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println(version.Current)
		return
	}

	serverURL := flag.String("server", "wss://localhost:443/ws/agent/", "control plane URL")
	listenAddr := flag.String("listen", ":443", "peer listen address (backbone only)")
	name := flag.String("name", "", "node name")
	backbone := flag.Bool("backbone", true, "backbone mode (listens for peers); leaf nodes use --backbone=false")
	flag.Parse()

	if *name == "" {
		hostname, _ := os.Hostname()
		*name = hostname
	}

	if !*backbone {
		log.Printf("leaf mode: no inbound listener, outbound connections only")
	}

	ag := agent.New(*name, *serverURL, *listenAddr, *backbone)

	go func() {
		if err := ag.Start(); err != nil {
			log.Fatalf("agent start failed: %v", err)
		}
	}()

	role := "backbone"
	if !*backbone {
		role = "leaf"
	}
	log.Printf("MeshNet agent [%s] started as %s", *name, role)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ag.Stop()
	log.Println("Agent stopped")
}

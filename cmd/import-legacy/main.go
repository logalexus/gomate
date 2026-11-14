package main

import (
	"context"
	"flag"
	"gomate/internal/storage"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	sourceID := flag.String("source-id", "", "stable name of this offline Packmate database snapshot")
	history := flag.Bool("history", false, "also import streams, packets and favorites")
	flag.Parse()
	if *sourceID == "" {
		log.Fatal("--source-id is required; DSNs come from SOURCE_DATABASE_URL and DATABASE_URL")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	s, e := storage.Open(ctx, os.Getenv("DATABASE_URL"))
	if e != nil {
		log.Fatal(e)
	}
	defer s.Close()
	if e = s.ImportLegacy(ctx, os.Getenv("SOURCE_DATABASE_URL"), *sourceID, *history); e != nil {
		log.Fatal(e)
	}
	log.Print("import complete; source was opened read-only")
}

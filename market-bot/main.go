package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	flagMode           = flag.String("mode", "k3s", "runtime mode: k3s or amp")
	flagDBHost         = flag.String("dbhost", "", "PostgreSQL host")
	flagDBPort         = flag.Int("dbport", 15432, "PostgreSQL port")
	flagDBUser         = flag.String("dbuser", "dune", "PostgreSQL user")
	flagDBPass         = flag.String("dbpass", "", "PostgreSQL password")
	flagDBName         = flag.String("dbname", "dune", "PostgreSQL database")
	flagCacheDB        = flag.String("cachedb", "/data/market-bot-cache.db", "SQLite path for category cache")
	flagBuyInterval    = flag.Duration("buyinterval", 5*time.Minute, "how often to buy player listings")
	flagListInterval   = flag.Duration("listinterval", 30*time.Minute, "how often to restock/prune bot listings")
	flagBuyThreshold   = flag.Float64("buythreshold", 1.05, "buy player listings at or below this multiple of the bot's sell price (0 = disable buying)")
	flagMaxBuys        = flag.Int("maxbuys", 50, "max player listings to purchase per tick")
	flagReport         = flag.Bool("report", false, "print per-item sales analytics as TSV and exit (does not run the bot loop)")
	flagDryRun         = flag.Bool("dryrun", false, "plan buys/listings without writing market changes")
	flagStatusInterval = flag.Duration("statusinterval", 1*time.Minute, "how often to print live bot status (0 = disable)")
	flagUI             = flag.Bool("ui", true, "show live terminal dashboard while running")
	flagLogFile        = flag.String("logfile", "", "also write logs to this file")
	flagMaxSpend       = flag.Int64("maxspend", 0, "max Solaris the bot may spend buying player listings per tick (0 = unlimited)")
	flagMaxListings    = flag.Int("maxlistings", 0, "max new bot listings to create per list tick (0 = unlimited)")
)

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func main() {
	flag.Parse()

	mode := strings.ToLower(strings.TrimSpace(*flagMode))
	switch mode {
	case "k3s":
		if *flagDBHost == "" {
			fmt.Fprintln(os.Stderr, "error: -dbhost is required in k3s mode")
			flag.Usage()
			os.Exit(1)
		}
	case "amp":
		if *flagDBHost == "" {
			*flagDBHost = "127.0.0.1"
		}
		if *flagCacheDB == "/data/market-bot-cache.db" {
			*flagCacheDB = "market-bot-cache.db"
		}
	default:
		fmt.Fprintf(os.Stderr, "error: unsupported mode %q (use k3s or amp)\n", *flagMode)
		flag.Usage()
		os.Exit(1)
	}

	log.SetFlags(log.Ldate | log.Ltime | log.Lmsgprefix)
	log.SetPrefix("market-bot ")
	var logFile *os.File
	if strings.TrimSpace(*flagLogFile) != "" {
		logFile, err := os.OpenFile(*flagLogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			log.Fatalf("open logfile: %v", err)
		}
		defer logFile.Close()
		log.SetOutput(io.MultiWriter(os.Stderr, logFile))
	}
	log.Printf("mode: %s", mode)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	connParts := []string{
		fmt.Sprintf("host=%s", *flagDBHost),
		fmt.Sprintf("port=%d", *flagDBPort),
		fmt.Sprintf("user=%s", *flagDBUser),
		fmt.Sprintf("dbname=%s", *flagDBName),
		"sslmode=disable",
	}
	if strings.TrimSpace(*flagDBPass) != "" {
		connParts = append(connParts, fmt.Sprintf("password=%s", *flagDBPass))
	}
	connStr := strings.Join(connParts, " ")
	poolConfig, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		log.Fatalf("db config: %v", err)
	}
	poolConfig.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, `SET search_path TO dune, public`)
		return err
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("db ping: %v", err)
	}
	log.Printf("connected to %s:%d/%s", *flagDBHost, *flagDBPort, *flagDBName)

	log.Println("loading catalog...")
	catalog, err := loadCatalog()
	if err != nil {
		log.Fatalf("load catalog: %v", err)
	}
	log.Printf("catalog: %d listable items", len(catalog))

	var ui *marketUI
	if *flagUI && !*flagReport {
		ui = newMarketUI(mode, fmt.Sprintf("%s:%d/%s", *flagDBHost, *flagDBPort, *flagDBName), len(catalog), *flagDryRun, *flagBuyInterval, *flagListInterval)
		if logFile != nil {
			log.SetOutput(io.MultiWriter(ui, logFile))
		} else {
			log.SetOutput(ui)
		}
		defer ui.Close()
	}

	ex, err := NewExchange(pool, *flagCacheDB, catalog)
	if err != nil {
		log.Fatalf("init exchange: %v", err)
	}
	ex.buyThreshold = *flagBuyThreshold
	ex.maxBuys = *flagMaxBuys
	ex.maxSpendPerTick = *flagMaxSpend
	ex.maxListingsPerTick = *flagMaxListings
	ex.dryRun = *flagDryRun
	if ex.dryRun {
		log.Println("DRY RUN enabled: market changes will be logged but not written")
	}

	log.Println("initializing exchange...")
	if err := ex.Init(ctx, catalog); err != nil {
		log.Fatalf("init: %v", err)
	}
	log.Println("exchange ready")

	// Report mode: print analytics and exit without running the bot loop.
	if *flagReport {
		runReport(ctx, pool, ex, catalog)
		return
	}

	// Run both immediately on start.
	ex.Tick(ctx, catalog)

	tick := time.NewTicker(minDuration(*flagBuyInterval, *flagListInterval))
	defer tick.Stop()
	var statusTick *time.Ticker
	var statusC <-chan time.Time
	if *flagStatusInterval > 0 {
		statusTick = time.NewTicker(*flagStatusInterval)
		defer statusTick.Stop()
		statusC = statusTick.C
	}
	nextBuy := time.Now().Add(*flagBuyInterval)
	nextList := time.Now().Add(*flagListInterval)
	var commands <-chan string
	if ui != nil {
		commands = ui.Commands()
		ui.Render(ex, nextBuy, nextList)
	} else {
		commands = readCommands(ctx)
	}
	for {
		select {
		case <-ctx.Done():
			log.Println("shutting down (signal received)")
			if ui != nil {
				ui.Render(ex, nextBuy, nextList)
			}
			return
		case cmd, ok := <-commands:
			if !ok {
				commands = nil
				continue
			}
			switch cmd {
			case "p", "pause":
				ex.paused = !ex.paused
				log.Printf("paused=%t", ex.paused)
			case "b", "buy":
				log.Println("manual buy tick")
				ex.BuyTick(ctx)
				nextBuy = time.Now().Add(*flagBuyInterval)
			case "l", "list":
				log.Println("manual list tick")
				ex.ListTick(ctx, catalog)
				nextList = time.Now().Add(*flagListInterval)
			case "r", "run":
				log.Println("manual full tick")
				ex.Tick(ctx, catalog)
				nextBuy = time.Now().Add(*flagBuyInterval)
				nextList = time.Now().Add(*flagListInterval)
			case "q", "quit":
				log.Println("quit requested")
				return
			case "h", "help", "?":
				log.Println("commands: p=pause/resume b=buy l=list r=full tick q=quit h=help")
			default:
				log.Printf("unknown command %q (h for help)", cmd)
			}
			if ui != nil {
				ui.Render(ex, nextBuy, nextList)
			}
		case <-statusC:
			log.Println(ex.StatusLine())
			if ui != nil {
				ui.Render(ex, nextBuy, nextList)
			}
		case now := <-tick.C:
			if ex.paused {
				if ui != nil {
					ui.Render(ex, nextBuy, nextList)
				}
				continue
			}
			if now.After(nextBuy) {
				ex.BuyTick(ctx)
				nextBuy = now.Add(*flagBuyInterval)
			}
			if now.After(nextList) {
				ex.ListTick(ctx, catalog)
				nextList = now.Add(*flagListInterval)
			}
			if ui != nil {
				ui.Render(ex, nextBuy, nextList)
			}
		}
	}
}

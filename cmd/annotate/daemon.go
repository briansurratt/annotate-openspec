package main

import (
	"context"
	"fmt"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/briansurratt/annotate/internal/config"
	"github.com/briansurratt/annotate/internal/daemon"
	"github.com/briansurratt/annotate/internal/index"
	"github.com/briansurratt/annotate/internal/queue"
	"github.com/briansurratt/annotate/internal/store"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Start the annotate daemon",
	RunE:  runDaemon,
}

func init() {
	rootCmd.AddCommand(daemonCmd)
}

// runDaemon loads config, opens the store, constructs the daemon, and runs it
// until a SIGTERM or SIGINT is received.
func runDaemon(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	dbPath := cfg.IndexCachePath
	if dbPath == "" {
		dbPath = filepath.Join(cfg.WorkspacePath, ".annotate", "index_cache.sqlite")
	}

	s, err := store.Open(context.Background(), dbPath, nil)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	db := s.DB()

	q, err := queue.New(db)
	if err != nil {
		return fmt.Errorf("create queue: %w", err)
	}
	defer q.Close()

	idx := index.New()

	d, err := daemon.New(daemon.Config{
		DB:               db,
		Index:            idx,
		Queue:            q,
		WorkspaceRoot:    cfg.WorkspacePath,
		SocketPath:       cfg.SocketPath,
		DebounceInterval: time.Duration(cfg.DebounceInterval),
		FlushInterval:    5 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("create daemon: %w", err)
	}

	runCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	return d.Run(runCtx)
}

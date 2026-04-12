package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/briansurratt/annotate/internal/config"
)

var enqueueCmd = &cobra.Command{
	Use:   "enqueue <file>",
	Short: "Enqueue a file for processing by the daemon",
	Args:  cobra.ExactArgs(1),
	RunE:  runEnqueue,
}

func init() {
	rootCmd.AddCommand(enqueueCmd)
}

// runEnqueue loads config for the socket path, dials the daemon's Unix socket,
// and POSTs the file path to /enqueue. Exits non-zero if the daemon is not running.
func runEnqueue(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	socketPath := cfg.SocketPath
	filePath := args[0]

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "unix", socketPath)
			},
		},
		Timeout: 5 * time.Second,
	}

	resp, err := client.Post("http://unix/enqueue", "text/plain", strings.NewReader(filePath))
	if err != nil {
		return fmt.Errorf("daemon is not running")
	}
	defer resp.Body.Close()
	return nil
}

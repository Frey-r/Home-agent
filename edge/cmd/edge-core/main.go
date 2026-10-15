// Comando edge-core: cerebro del hogar (agente, guardrails, memoria) que
// corre como app de Home Assistant.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Frey-r/Home-agent/edge/internal/config"
	"github.com/Frey-r/Home-agent/edge/internal/ha"
	"github.com/Frey-r/Home-agent/edge/internal/store"
)

const (
	usage = "uso: edge-core run [--once]"
	// haStartPoll es el intervalo de consulta mientras HA arranca.
	haStartPoll = 500 * time.Millisecond
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.LookupEnv, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "edge-core:", err)
		stop()
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, lookup config.LookupFunc, logOut io.Writer) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	switch args[0] {
	case "run":
		return runCmd(ctx, args[1:], lookup, logOut)
	default:
		return fmt.Errorf("subcomando desconocido %q; %s", args[0], usage)
	}
}

func runCmd(ctx context.Context, args []string, lookup config.LookupFunc, logOut io.Writer) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(logOut)
	once := fs.Bool("once", false, "conectar, sincronizar y salir (smoke test)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	log := slog.New(slog.NewTextHandler(logOut, nil))

	cfg, err := config.Load(lookup)
	if err != nil {
		return err
	}
	log.Info("configuración cargada", "mode", cfg.Mode, "data_dir", cfg.DataDir)

	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return fmt.Errorf("crear data dir: %w", err)
	}
	st, err := store.Open(ctx, filepath.Join(cfg.DataDir, "domotica.db"))
	if err != nil {
		return err
	}
	defer st.Close()

	conn, err := ha.Connect(ctx, cfg.HAURL, cfg.HAToken)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Tras un reinicio de HA OS la app puede arrancar antes que el core:
	// sincronizar en ese momento entregaría una casa a medias.
	log.Info("esperando a que Home Assistant termine de arrancar")
	if err := conn.WaitStarted(ctx, haStartPoll); err != nil {
		return err
	}

	states, err := conn.GetStates(ctx)
	if err != nil {
		return err
	}
	log.Info("conectado a Home Assistant", "ha_version", conn.HAVersion(), "entities", len(states))

	if *once {
		return nil
	}
	<-ctx.Done()
	log.Info("apagando")
	return nil
}

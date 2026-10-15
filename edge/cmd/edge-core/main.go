// Comando edge-core: cerebro del hogar (agente, guardrails, memoria) que
// corre como app de Home Assistant.
package main

import (
	"context"
	"encoding/json"
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

	"github.com/Frey-r/Home-agent/contracts"
	"github.com/Frey-r/Home-agent/edge/internal/config"
	"github.com/Frey-r/Home-agent/edge/internal/entities"
	"github.com/Frey-r/Home-agent/edge/internal/ha"
	"github.com/Frey-r/Home-agent/edge/internal/store"
)

const (
	usage = "uso: edge-core <run|entities> [...]"
	// haStartPoll es el intervalo de consulta mientras HA arranca.
	haStartPoll = 500 * time.Millisecond
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.LookupEnv, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "edge-core:", err)
		stop()
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, lookup config.LookupFunc, stdout io.Writer, stderr io.Writer) error {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if len(args) == 0 {
		return errors.New(usage)
	}
	switch args[0] {
	case "run":
		return runCmd(ctx, args[1:], lookup, stdout, stderr)
	case "entities":
		return entitiesCmd(ctx, args[1:], lookup, stdout, stderr)
	default:
		return fmt.Errorf("subcomando desconocido %q; %s", args[0], usage)
	}
}

func runCmd(ctx context.Context, args []string, lookup config.LookupFunc, stdout io.Writer, stderr io.Writer) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	once := fs.Bool("once", false, "conectar, sincronizar y salir (smoke test)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	log := slog.New(slog.NewTextHandler(stderr, nil))

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

	dir := entities.NewDirectory()
	syncer := entities.NewSyncer(dir)
	syncer.OnEntityUpdated = func(ent *contracts.Entity) {
		log.Info("entidad actualizada", "entity_id", ent.ID, "state", ent.State, "category", ent.Category, "exposed", ent.Exposed)
	}
	syncer.OnEntityRemoved = func(id string) {
		log.Info("entidad eliminada", "entity_id", id)
	}

	if *once {
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

		syncCtx, syncCancel := context.WithCancel(ctx)
		defer syncCancel()
		go func() {
			_ = syncer.Run(syncCtx, conn)
		}()

		if err := syncer.WaitSynced(ctx, 10*time.Second); err != nil {
			return fmt.Errorf("sincronizar entidades: %w", err)
		}

		log.Info("conectado a Home Assistant", "ha_version", conn.HAVersion(), "entities", dir.Count())
		return nil
	}

	// Modo continuo supervisado
	sup := &ha.Supervisor{
		URL:         cfg.HAURL,
		Token:       cfg.HAToken,
		WaitStarted: haStartPoll,
		Heartbeat:   30 * time.Second,
		PingTimeout: 5 * time.Second,
		HealthyTime: 30 * time.Second,
		Backoff: ha.Backoff{
			Initial:    1 * time.Second,
			Max:        30 * time.Second,
			Multiplier: 2.0,
			Jitter:     0.2,
		},
		OnConnected: func() {
			log.Info("supervisor: conectado a Home Assistant")
		},
		OnDisconnected: func() {
			log.Warn("supervisor: desconectado de Home Assistant")
		},
	}

	session := func(sctx context.Context, conn *ha.Conn) error {
		log.Info("conectado a Home Assistant", "ha_version", conn.HAVersion())
		go func() {
			if err := syncer.WaitSynced(sctx, 10*time.Second); err == nil {
				log.Info("sincronización inicial completada", "entities", dir.Count())
			}
		}()
		return syncer.Run(sctx, conn)
	}

	log.Info("iniciando supervisor continuo")
	return sup.Run(ctx, session)
}

func entitiesCmd(ctx context.Context, args []string, lookup config.LookupFunc, stdout io.Writer, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("uso: edge-core entities <dump> [...]")
	}
	switch args[0] {
	case "dump":
		return entitiesDumpCmd(ctx, args[1:], lookup, stdout, stderr)
	default:
		return fmt.Errorf("subcomando desconocido de entities %q; uso: edge-core entities <dump>", args[0])
	}
}

func entitiesDumpCmd(ctx context.Context, args []string, lookup config.LookupFunc, stdout io.Writer, stderr io.Writer) error {
	fs := flag.NewFlagSet("entities dump", flag.ContinueOnError)
	fs.SetOutput(stderr)
	exposedOnly := fs.Bool("exposed-only", false, "volcar solo entidades expuestas al asistente")
	category := fs.String("category", "", "filtrar por categoría de entidad (p.ej. light, switch, etc.)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(lookup)
	if err != nil {
		return err
	}

	conn, err := ha.Connect(ctx, cfg.HAURL, cfg.HAToken)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := conn.WaitStarted(ctx, haStartPoll); err != nil {
		return err
	}

	dir := entities.NewDirectory()
	syncer := entities.NewSyncer(dir)

	syncCtx, syncCancel := context.WithCancel(ctx)
	defer syncCancel()

	go func() {
		_ = syncer.Run(syncCtx, conn)
	}()

	if err := syncer.WaitSynced(ctx, 10*time.Second); err != nil {
		return fmt.Errorf("sincronizar entidades: %w", err)
	}

	filter := entities.Filter{
		ExposedOnly: *exposedOnly,
		Category:    *category,
	}
	list := dir.List(filter)
	if list == nil {
		list = []*contracts.Entity{}
	}

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(list); err != nil {
		return fmt.Errorf("codificar entidades: %w", err)
	}
	return nil
}

// Comando edge-core: cerebro del hogar (agente, guardrails, memoria) que
// corre como app de Home Assistant.
package main

import (
	"context"
	"errors"
	"io"
	"os"

	"domotica/edge/internal/config"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.LookupEnv, os.Stderr); err != nil {
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, lookup config.LookupFunc, logOut io.Writer) error {
	return errors.New("edge-core: no implementado")
}

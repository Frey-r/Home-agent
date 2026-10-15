package llmkit

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Frey-r/Home-agent/contracts"
)

// RouteTarget define un proveedor dentro de la cadena de enrutamiento con fallback.
type RouteTarget struct {
	Name    string
	Client  Client
	Timeout time.Duration // Timeout opcional exclusivo para este intento
}

// RouterConfig configura los proveedores y prioridades del Router.
type RouterConfig struct {
	Targets []RouteTarget
}

// TargetError registra el fallo específico de un proveedor en la cadena.
type TargetError struct {
	TargetName string
	Err        error
}

func (e TargetError) String() string {
	return fmt.Sprintf("[%s]: %v", e.TargetName, e.Err)
}

// FallbackError representa el fallo compuesto cuando todos los proveedores configurados fallaron.
type FallbackError struct {
	Errors []TargetError
}

func (e *FallbackError) Error() string {
	var msgs []string
	for _, te := range e.Errors {
		msgs = append(msgs, te.String())
	}
	return fmt.Sprintf("llmkit: todos los proveedores fallaron: %s", strings.Join(msgs, "; "))
}

// Router orquesta múltiples clientes LLM con conmutación automática ante 429, 5xx y timeouts.
type Router struct {
	targets []RouteTarget
}

// NewRouter crea una nueva instancia de Router.
func NewRouter(cfg RouterConfig) (*Router, error) {
	if len(cfg.Targets) == 0 {
		return nil, fmt.Errorf("llmkit: RouterConfig requiere al menos un RouteTarget")
	}

	targets := make([]RouteTarget, len(cfg.Targets))
	for i, t := range cfg.Targets {
		if t.Client == nil {
			return nil, fmt.Errorf("llmkit: target %d tiene Client nil", i)
		}
		name := t.Name
		if name == "" {
			name = fmt.Sprintf("target-%d", i)
		}
		targets[i] = RouteTarget{
			Name:    name,
			Client:  t.Client,
			Timeout: t.Timeout,
		}
	}

	return &Router{targets: targets}, nil
}

// Complete ejecuta la petición en el proveedor primario y conmuta transparentemente ante fallos recuperables.
func (r *Router) Complete(ctx context.Context, req contracts.LLMRequest) (contracts.LLMResponse, error) {
	var targetErrors []TargetError

	for _, target := range r.targets {
		if ctx.Err() != nil {
			return contracts.LLMResponse{}, ctx.Err()
		}

		attemptCtx, cancel := r.targetContext(ctx, target.Timeout)
		resp, err := target.Client.Complete(attemptCtx, req)
		cancel()

		if err == nil {
			return resp, nil
		}

		if !r.shouldFallback(ctx, err) {
			return contracts.LLMResponse{}, err
		}

		targetErrors = append(targetErrors, TargetError{
			TargetName: target.Name,
			Err:        err,
		})
	}

	return contracts.LLMResponse{}, &FallbackError{Errors: targetErrors}
}

// Stream inicia streaming en el proveedor primario y conmuta al fallback si el primario falla antes de emitir tokens.
func (r *Router) Stream(ctx context.Context, req contracts.LLMRequest) (<-chan contracts.LLMStreamEvent, <-chan error) {
	eventCh := make(chan contracts.LLMStreamEvent, 100)
	errCh := make(chan error, 1)

	go func() {
		defer close(eventCh)
		defer close(errCh)

		var targetErrors []TargetError

		for _, target := range r.targets {
			if ctx.Err() != nil {
				errCh <- ctx.Err()
				return
			}

			attemptCtx, cancel := r.targetContext(ctx, target.Timeout)
			tEventCh, tErrCh := target.Client.Stream(attemptCtx, req)

			firstEventReceived := false
			var streamErr error

			select {
			case <-ctx.Done():
				cancel()
				errCh <- ctx.Err()
				return
			case err, ok := <-tErrCh:
				if ok && err != nil {
					streamErr = err
				}
			case ev, ok := <-tEventCh:
				if ok {
					firstEventReceived = true
					eventCh <- ev
				} else {
					// Si el canal de eventos cerró vacío, recuperar error pendiente en tErrCh
					select {
					case err, okErr := <-tErrCh:
						if okErr {
							streamErr = err
						}
					default:
					}
				}
			}

			if firstEventReceived {
				// El proveedor primario emitió eventos exitosamente; drenar resto del stream
				for tEventCh != nil || tErrCh != nil {
					select {
					case <-ctx.Done():
						cancel()
						errCh <- ctx.Err()
						return
					case ev, ok := <-tEventCh:
						if !ok {
							tEventCh = nil
							continue
						}
						eventCh <- ev
					case err, ok := <-tErrCh:
						if !ok {
							tErrCh = nil
							continue
						}
						if err != nil {
							errCh <- err
						}
					}
				}
				cancel()
				return
			}

			// No se recibieron eventos en este target; verificar si es recuperable
			cancel()
			if streamErr == nil {
				streamErr = fmt.Errorf("llmkit: target %s cerro conexion sin emitir eventos", target.Name)
			}

			if !r.shouldFallback(ctx, streamErr) {
				errCh <- streamErr
				return
			}

			targetErrors = append(targetErrors, TargetError{
				TargetName: target.Name,
				Err:        streamErr,
			})
		}

		errCh <- &FallbackError{Errors: targetErrors}
	}()

	return eventCh, errCh
}

func (r *Router) targetContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout > 0 {
		return context.WithTimeout(ctx, timeout)
	}
	return context.WithCancel(ctx)
}

func (r *Router) shouldFallback(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	// Si el contexto padre fue cancelado explícitamente, abortar sin reintentar
	if ctx.Err() != nil {
		return false
	}

	var apiErr *APIError
	if errors.As(err, &apiErr) {
		if apiErr.IsRateLimit() { // 429
			return true
		}
		if apiErr.IsServerError() { // 5xx
			return true
		}
		if apiErr.IsClientError() { // 4xx excepto 429
			return false
		}
	}

	// Errores de red o timeouts locales del target son recuperables
	return true
}

package ha

import (
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"time"
)

// Backoff calcula retardos exponenciales con jitter para reintentos de conexión.
type Backoff struct {
	Initial    time.Duration
	Max        time.Duration
	Multiplier float64
	Jitter     float64
}

// Delay devuelve el retardo correspondiente al intento dado (0-indexado).
func (b Backoff) Delay(attempt int) time.Duration {
	if b.Initial <= 0 {
		return 0
	}
	mult := b.Multiplier
	if mult <= 1.0 {
		mult = 2.0
	}
	factor := math.Pow(mult, float64(attempt))
	delayFloat := float64(b.Initial) * factor
	if b.Max > 0 && delayFloat > float64(b.Max) {
		delayFloat = float64(b.Max)
	}
	if b.Jitter > 0 {
		// Variación uniforme en [delay * (1 - Jitter), delay * (1 + Jitter)]
		delta := (rand.Float64()*2 - 1) * b.Jitter
		delayFloat = delayFloat * (1.0 + delta)
	}
	return time.Duration(delayFloat)
}

// SessionFunc es la función que corre mientras la conexión con HA está activa.
type SessionFunc func(ctx context.Context, conn *Conn) error

// Supervisor gestiona el ciclo de vida de la conexión con Home Assistant:
// conexión, espera de arranque, ejecución de sesión, heartbeat y reconexión con backoff.
type Supervisor struct {
	URL         string
	Token       string
	Backoff     Backoff
	WaitStarted time.Duration
	Heartbeat   time.Duration
	PingTimeout time.Duration
	HealthyTime time.Duration

	// Callbacks para observabilidad y testing
	OnConnectAttempt func()
	OnBackoffDelay   func(d time.Duration)
	OnConnected      func()
	OnDisconnected   func()
}

// Run ejecuta el bucle de supervisión hasta que ctx sea cancelado o se produzca
// un error terminal (p. ej. autenticación inválida).
func (s *Supervisor) Run(ctx context.Context, session SessionFunc) error {
	attempt := 0

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if s.OnConnectAttempt != nil {
			s.OnConnectAttempt()
		}

		conn, err := Connect(ctx, s.URL, s.Token)
		if err != nil {
			if errors.Is(err, ErrAuthInvalid) {
				return ErrAuthInvalid // Terminal: credenciales inválidas no se reintentan
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			d := s.Backoff.Delay(attempt)
			if s.OnBackoffDelay != nil {
				s.OnBackoffDelay(d)
			}
			select {
			case <-time.After(d):
				attempt++
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		// Esperar a que HA termine de arrancar si está configurado
		if s.WaitStarted > 0 {
			if err := conn.WaitStarted(ctx, s.WaitStarted); err != nil {
				conn.Close()
				if errors.Is(err, ErrAuthInvalid) {
					return ErrAuthInvalid
				}
				if ctx.Err() != nil {
					return ctx.Err()
				}
				d := s.Backoff.Delay(attempt)
				if s.OnBackoffDelay != nil {
					s.OnBackoffDelay(d)
				}
				select {
				case <-time.After(d):
					attempt++
					continue
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}

		if s.OnConnected != nil {
			s.OnConnected()
		}

		sessionStartTime := time.Now()
		sessCtx, cancelSess := context.WithCancel(ctx)

		// Heartbeat periódico si está habilitado
		var hbDone chan struct{}
		if s.Heartbeat > 0 {
			hbDone = make(chan struct{})
			go func() {
				defer close(hbDone)
				pingTimeout := s.PingTimeout
				if pingTimeout <= 0 {
					pingTimeout = 5 * time.Second
				}
				ticker := time.NewTicker(s.Heartbeat)
				defer ticker.Stop()
				for {
					select {
					case <-sessCtx.Done():
						return
					case <-conn.Done():
						return
					case <-ticker.C:
						pCtx, pCancel := context.WithTimeout(sessCtx, pingTimeout)
						err := conn.Ping(pCtx)
						pCancel()
						if err != nil {
							// Peer congelado o desconectado: cerrar conexión para terminar la sesión
							conn.Close()
							return
						}
					}
				}
			}()
		}

		// Ejecutar sesión
		sessErrCh := make(chan error, 1)
		go func() {
			sessErrCh <- session(sessCtx, conn)
		}()

		select {
		case <-sessErrCh:
		case <-conn.Done():
			cancelSess()
			<-sessErrCh
		case <-ctx.Done():
			cancelSess()
			conn.Close()
			<-sessErrCh
		}

		cancelSess()
		conn.Close()
		if hbDone != nil {
			<-hbDone
		}

		if s.OnDisconnected != nil {
			s.OnDisconnected()
		}

		// Si se mantuvo sana por al menos HealthyTime, se resetea el backoff
		healthyDuration := s.HealthyTime
		if healthyDuration <= 0 {
			healthyDuration = 10 * time.Second
		}
		if time.Since(sessionStartTime) >= healthyDuration {
			attempt = 0
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Esperar backoff antes de reconectar
		d := s.Backoff.Delay(attempt)
		if s.OnBackoffDelay != nil {
			s.OnBackoffDelay(d)
		}
		select {
		case <-time.After(d):
			attempt++
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

package llmtest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// RecordedRequest almacena una petición HTTP entrante para inspección en pruebas.
type RecordedRequest struct {
	Method string
	Path   string
	Header http.Header
	Body   []byte
}

// UnmarshalBody decodifica el cuerpo de la petición en la estructura destino.
func (r RecordedRequest) UnmarshalBody(v any) error {
	return json.Unmarshal(r.Body, v)
}

// HandlerFunc permite a una prueba definir la lógica de respuesta para una petición.
type HandlerFunc func(r *http.Request, body []byte) (statusCode int, headers http.Header, responseBody []byte)

// Server es un servidor HTTP de prueba hermético para los protocolos de OpenAI y Anthropic.
type Server struct {
	server   *httptest.Server
	URL      string
	mu       sync.Mutex
	requests []RecordedRequest
	handler  HandlerFunc
}

// New crea e inicia una nueva instancia de Server. Registra su cierre en t.Cleanup.
func New(t *testing.T) *Server {
	t.Helper()
	s := &Server{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "error leyendo cuerpo", http.StatusInternalServerError)
			return
		}

		s.mu.Lock()
		s.requests = append(s.requests, RecordedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Header: r.Header.Clone(),
			Body:   body,
		})
		h := s.handler
		s.mu.Unlock()

		if h != nil {
			status, headers, respBytes := h(r, body)
			for k, vv := range headers {
				for _, v := range vv {
					w.Header().Add(k, v)
				}
			}
			if status > 0 {
				w.WriteHeader(status)
			}
			_, _ = w.Write(respBytes)
			return
		}

		// Comportamiento por defecto: 404 si no hay handler configurado
		http.Error(w, "llmtest: no handler configured", http.StatusNotFound)
	}))

	s.server = ts
	s.URL = ts.URL
	t.Cleanup(s.Close)
	return s
}

// Close apaga el servidor HTTP de pruebas.
func (s *Server) Close() {
	if s.server != nil {
		s.server.Close()
	}
}

// SetHandler define la función de manejo de peticiones activas.
func (s *Server) SetHandler(h HandlerFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handler = h
}

// Requests devuelve una copia de todas las peticiones recibidas hasta el momento.
func (s *Server) Requests() []RecordedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	res := make([]RecordedRequest, len(s.requests))
	copy(res, s.requests)
	return res
}

// LastRequest devuelve la última petición recibida o false si no hubo ninguna.
func (s *Server) LastRequest() (RecordedRequest, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.requests) == 0 {
		return RecordedRequest{}, false
	}
	return s.requests[len(s.requests)-1], true
}

// ResetRequests limpia el historial de peticiones registradas.
func (s *Server) ResetRequests() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = nil
}

// RespondJSON configura un handler que responde con un código de estado y JSON serializado.
func (s *Server) RespondJSON(statusCode int, data any) {
	bytes, err := json.Marshal(data)
	if err != nil {
		panic(fmt.Sprintf("llmtest: error serializando json: %v", err))
	}
	s.SetHandler(func(r *http.Request, body []byte) (int, http.Header, []byte) {
		h := make(http.Header)
		h.Set("Content-Type", "application/json")
		return statusCode, h, bytes
	})
}

// RespondSSE configura un handler que emite un stream de eventos SSE.
func (s *Server) RespondSSE(events []string) {
	s.SetHandler(func(r *http.Request, body []byte) (int, http.Header, []byte) {
		h := make(http.Header)
		h.Set("Content-Type", "text/event-stream")
		h.Set("Cache-Control", "no-cache")
		h.Set("Connection", "keep-alive")

		var payload []byte
		for _, ev := range events {
			payload = append(payload, []byte(ev)...)
		}
		return http.StatusOK, h, payload
	})
}

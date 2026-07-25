package webserver

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type WebServer struct {
	Router        chi.Router
	WebServerPort string
}

func NewWebServer(serverPort string) *WebServer {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	return &WebServer{
		Router:        r,
		WebServerPort: serverPort,
	}
}

func (s *WebServer) AddHandler(path string, handler http.HandlerFunc) {
	s.Router.Post(path, handler)
}

func (s *WebServer) AddGetHandler(path string, handler http.HandlerFunc) {
	s.Router.Get(path, handler)
}

func (s *WebServer) Start() {
	http.ListenAndServe(":"+s.WebServerPort, s.Router)
}

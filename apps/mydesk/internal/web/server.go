package web

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"

	"github.com/silee-tools/mydesk/static"
)

// Server holds the configuration and HTTP handler for the web UI.
type Server struct {
	ConfigDir string
	Version   string
	mux       *http.ServeMux
}

// New creates a Server with all routes registered.
func New(configDir, version string) *Server {
	s := &Server{
		ConfigDir: configDir,
		Version:   version,
		mux:       http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	// API routes
	s.mux.HandleFunc("/api/status", s.handleStatus)
	s.mux.HandleFunc("/api/links", s.handleLinks)
	s.mux.HandleFunc("/api/links/link", s.handleLinkAction)
	s.mux.HandleFunc("/api/links/unlink", s.handleUnlinkAction)
	s.mux.HandleFunc("/api/drift", s.handleDrift)
	s.mux.HandleFunc("/api/config/links-conf", s.handleLinksConf)
	s.mux.HandleFunc("/api/config/native-dirs", s.handleNativeDirs)
	s.mux.HandleFunc("/api/provision/status", s.handleProvisionStatus)
	s.mux.HandleFunc("/api/provision/", s.handleProvisionAction)
	s.mux.HandleFunc("/api/sync", s.handleSync)

	// Static files (embedded)
	staticFS, err := fs.Sub(static.Assets, ".")
	if err != nil {
		log.Fatal(err)
	}
	s.mux.Handle("/", http.FileServer(http.FS(staticFS)))
}

// Start listens on the given address and serves HTTP.
func (s *Server) Start(addr string) error {
	fmt.Printf("mydesk web → http://%s\n", addr)
	return http.ListenAndServe(addr, s.mux)
}

package web

import (
	"embed"
	"io/fs"
	"net/http"
	"net/url"
)

//go:embed static/*
var staticFiles embed.FS

func (s *service) routes() http.Handler {
	mux := http.NewServeMux()
	assets, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(assets))))
	mux.HandleFunc("GET /", s.handleHome)
	mux.HandleFunc("GET /fragments/chat", s.handleChatFragment)
	mux.HandleFunc("POST /projects/pick", s.handlePickProject)
	mux.HandleFunc("GET /presets", s.handlePresetList)
	mux.HandleFunc("GET /presets/new", s.handleNewPreset)
	mux.HandleFunc("GET /presets/edit", s.handleEditPreset)
	mux.HandleFunc("POST /presets", s.handleCreatePreset)
	mux.HandleFunc("POST /presets/update", s.handleUpdatePreset)
	mux.HandleFunc("POST /presets/archive", s.handleArchivePreset)
	mux.HandleFunc("POST /messages", s.handleSubmitMessage)
	mux.HandleFunc("POST /sessions/model", s.handleSelectModel)
	mux.HandleFunc("POST /sessions/cancel", s.handleCancelSession)
	mux.HandleFunc("GET /fragments/chat-log", s.handleChatLogFragment)
	mux.HandleFunc("GET /fragments/composer", s.handleComposerFragment)
	mux.HandleFunc("GET /events", s.handleEvents)
	return s.sameOriginWrites(mux)
}

func (s *service) sameOriginWrites(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			next.ServeHTTP(writer, request)
			return
		}
		origin := request.Header.Get("Origin")
		if origin != "" && !matchesRequestOrigin(origin, request.Host) {
			http.Error(writer, "跨源写请求被拒绝", http.StatusForbidden)
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func matchesRequestOrigin(origin string, host string) bool {
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return parsed.Scheme == "http" && parsed.Host == host
}

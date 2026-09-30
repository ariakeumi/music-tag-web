package httpapi

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/xhongc/music-tag-web/server/internal/config"
	"github.com/xhongc/music-tag-web/server/internal/service/queue"
	"github.com/xhongc/music-tag-web/server/internal/service/tagtask"
	"github.com/xhongc/music-tag-web/server/internal/store"
	"github.com/xhongc/music-tag-web/server/internal/tagclient"
)

type Server struct {
	Config    *config.Config
	Store     *store.Store
	Tags      *tagclient.Client
	Queue     *queue.Queue
	Scraper   ScraperRegistry
	TagTask   *tagtask.Service
	Translate Translator
}

// ScraperRegistry and Translator are small interfaces so handlers stay
// decoupled from the scraper implementations (see service/scraper).
type ScraperRegistry interface {
	FetchLyric(resource, songID string) (string, error)
	FetchID3ByTitle(resource string, title any) ([]map[string]any, error)
}

type Translator interface {
	TranslateLyc(text string) (string, error)
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	// auth
	mux.HandleFunc("POST /api/token/", s.HandleLogin)
	mux.Handle("GET /user/info/", s.auth(http.HandlerFunc(s.HandleUserInfo)))

	// file browsing + tags
	mux.Handle("POST /api/file_list/", s.auth(http.HandlerFunc(s.HandleFileList)))
	mux.Handle("POST /api/music_id3/", s.auth(http.HandlerFunc(s.HandleMusicID3)))
	mux.Handle("POST /api/update_id3/", s.auth(http.HandlerFunc(s.HandleUpdateID3)))
	mux.Handle("POST /api/batch_update_id3/", s.auth(http.HandlerFunc(s.HandleBatchUpdateID3)))
	mux.Handle("POST /api/batch_auto_update_id3/", s.auth(http.HandlerFunc(s.HandleBatchAutoUpdateID3)))
	mux.Handle("POST /api/upload_image/", s.auth(http.HandlerFunc(s.HandleUploadImage)))

	// scraping
	mux.Handle("POST /api/fetch_id3_by_title/", s.auth(http.HandlerFunc(s.HandleFetchID3ByTitle)))
	mux.Handle("POST /api/fetch_lyric/", s.auth(http.HandlerFunc(s.HandleFetchLyric)))
	mux.Handle("POST /api/translation_lyc/", s.auth(http.HandlerFunc(s.HandleTranslationLyc)))

	// background tasks
	mux.Handle("POST /api/tidy_folder/", s.auth(http.HandlerFunc(s.HandleTidyFolder)))
	mux.Handle("GET /api/record/", s.auth(http.HandlerFunc(s.HandleRecord)))
	mux.Handle("GET /api/active_queue/", s.auth(http.HandlerFunc(s.HandleActiveQueue)))
	mux.Handle("GET /api/clear_celery/", s.auth(http.HandlerFunc(s.HandleClearQueue)))
	// legacy endpoints kept for API compatibility; library tables were removed
	mux.Handle("GET /api/full_scan_folder/", s.auth(http.HandlerFunc(s.HandleNoop)))
	mux.Handle("GET /api/task1/", s.auth(http.HandlerFunc(s.HandleNoop)))
	mux.Handle("GET /api/task2/", s.auth(http.HandlerFunc(s.HandleNoop)))

	// SPA + static assets
	mux.HandleFunc("GET /{$}", s.serveIndex)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir(s.Config.StaticDir))))
	mux.Handle("GET /media/", http.StripPrefix("/media/", http.FileServer(http.Dir(s.Config.MediaRoot))))

	return s.gzipAPI(mux)
}

func (s *Server) serveIndex(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, filepath.Join(s.Config.StaticDir, "dist", "index.prod.html"))
}

func (s *Server) auth(next http.Handler) http.Handler {
	return AuthMiddleware(s.Config.JWTSecret, s.Config.LoginRequired, next)
}

func (s *Server) gzipAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/user/") {
			GzipMiddleware(next).ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) HandleNoop(w http.ResponseWriter, r *http.Request) {
	Success(w, nil)
}

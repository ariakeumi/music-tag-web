package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/xhongc/music-tag-web/server/internal/config"
	"github.com/xhongc/music-tag-web/server/internal/httpapi"
	"github.com/xhongc/music-tag-web/server/internal/service/queue"
	"github.com/xhongc/music-tag-web/server/internal/service/scraper"
	"github.com/xhongc/music-tag-web/server/internal/service/tagtask"
	"github.com/xhongc/music-tag-web/server/internal/service/translate"
	"github.com/xhongc/music-tag-web/server/internal/store"
	"github.com/xhongc/music-tag-web/server/internal/tagclient"
)

type translatorAdapter struct{ y *translate.Youdao }

func (t translatorAdapter) TranslateLyc(text string) (string, error) {
	return translate.TranslateLyc(t.y, text)
}

func main() {
	cfg := config.Load()

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer st.Close()
	// import users from a legacy Django db.sqlite3 when present, else seed
	// the documented default admin/admin (or ADMIN_PASSWORD)
	if err := st.EnsureBootstrap("db.sqlite3", cfg.AdminPassword); err != nil {
		log.Fatalf("bootstrap users: %v", err)
	}

	tags := tagclient.New(cfg.PythonBin, cfg.TagCLIPath)
	reg := &scraper.Registry{Tags: tags, FPCalcPath: cfg.FPCalcPath}
	q := queue.New()

	srv := &httpapi.Server{
		Config:    cfg,
		Store:     st,
		Tags:      tags,
		Queue:     q,
		Scraper:   reg,
		TagTask:   &tagtask.Service{Store: st, Tags: tags, Scraper: reg},
		Translate: translatorAdapter{y: translate.New()},
	}

	addr := ":" + cfg.Port
	fmt.Printf("Music Tag Web (Go) listening on %s\n", addr)
	fmt.Printf("  media root: %s\n  database:   %s\n  tag cli:    %s\n  fpcalc:     %s\n",
		cfg.MediaRoot, cfg.DBPath, cfg.TagCLIPath, cfg.FPCalcPath)

	log.Fatal(http.ListenAndServe(addr, srv.Routes()))
}

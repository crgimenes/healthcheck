package main

import (
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"
)

//go:embed web
var webFS embed.FS

const weekHours = 7 * 24

type cell struct {
	Class string
	Title string
}

type servicePage struct {
	Name   string
	Kind   string
	Target string
	Uptime string
	Cells  []cell
}

type page struct {
	Services    []servicePage
	GeneratedAt string
}

func buildPage(cfg *Config, store *Store, now time.Time) (*page, error) {
	since := now.Truncate(time.Hour).Add(-(weekHours - 1) * time.Hour)
	agg, err := store.hourly(since)
	if err != nil {
		return nil, err
	}

	p := &page{GeneratedAt: now.Format("2006-01-02 15:04 UTC")}
	for _, c := range cfg.Checks {
		sp := servicePage{
			Name:   c.Name,
			Kind:   c.Kind,
			Target: c.Target,
			Uptime: "no data",
			Cells:  make([]cell, 0, weekHours),
		}

		var total hourCount
		for i := range weekHours {
			hour := since.Add(time.Duration(i) * time.Hour)
			counts, ok := agg[c.Name][hour.Format("2006-01-02T15")]
			label := hour.Format("2006-01-02 15:04 UTC")
			if !ok {
				sp.Cells = append(sp.Cells, cell{Class: "nodata", Title: label + ": no data"})
				continue
			}

			class := statusUp
			if counts.Unstable > 0 {
				class = statusUnstable
			}
			if counts.Down > 0 {
				class = statusDown
			}
			title := fmt.Sprintf("%s: up %d, unstable %d, down %d", label, counts.Up, counts.Unstable, counts.Down)
			sp.Cells = append(sp.Cells, cell{Class: class, Title: title})

			total.Up += counts.Up
			total.Unstable += counts.Unstable
			total.Down += counts.Down
		}

		checks := total.Up + total.Unstable + total.Down
		if checks > 0 {
			sp.Uptime = fmt.Sprintf("%.2f%%", float64(total.Up)/float64(checks)*100)
		}
		p.Services = append(p.Services, sp)
	}
	return p, nil
}

func newWebHandler(cfg *Config, store *Store) http.Handler {
	tpl := template.Must(template.ParseFS(webFS, "web/index.go.tmpl"))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /style.css", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, webFS, "web/style.css")
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		p, err := buildPage(cfg, store, time.Now().UTC())
		if err != nil {
			log.Printf("page: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		err = tpl.Execute(w, p)
		if err != nil {
			log.Printf("render: %v", err)
		}
	})
	return mux
}

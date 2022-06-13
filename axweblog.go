package axweblog

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"html/template"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

//go:embed "static/*"
var htmlStatic embed.FS

//go:embed "templates/index.html"
var indexTemplate []byte

var maxLogChunk uint64 = 100
var maxLogLines = 500
var lpTTL = time.Second * 60
var ShowLogLineNumber = false

type WebLogJsonLine struct {
	ID   uint64
	Data map[string]interface{}
}

type WebLogWriter struct {
	id       uint64
	lines    []*WebLogJsonLine
	router   *chi.Mux
	pattern  string
	lpLock   sync.Mutex
	lpChan   map[uint64]chan *WebLogJsonLine
	lpChanID uint64
	uniq     string
}

func (h *WebLogWriter) WriteLevel(_ zerolog.Level, p []byte) (n int, err error) {
	return h.Write(p)
}

func (h *WebLogWriter) Write(p []byte) (n int, err error) {
	lineLogMap := map[string]interface{}{}
	err = json.Unmarshal(p, &lineLogMap)
	if err != nil {
		return 0, err
	}
	line := &WebLogJsonLine{
		ID:   atomic.AddUint64(&h.id, 1),
		Data: lineLogMap,
	}
	h.lines = append(h.lines, line)
	if len(h.lines) > maxLogLines {
		h.lines = h.lines[len(h.lines)-maxLogLines:]
	}
	go func() {
		h.lpLock.Lock()
		defer h.lpLock.Unlock()
		for _, c := range h.lpChan {
			c <- line
		}
		h.lpChan = map[uint64]chan *WebLogJsonLine{}
	}()
	return len(p), nil
}

func NewWebLogWriter(pattern string) *WebLogWriter {
	wl := &WebLogWriter{
		lines:   []*WebLogJsonLine{},
		router:  chi.NewRouter(),
		pattern: pattern,
		lpChan:  map[uint64]chan *WebLogJsonLine{},
		lpLock:  sync.Mutex{},
		uniq:    fmt.Sprintf("%d", time.Now().UnixMicro()),
	}
	wl.router.Route(pattern, func(r chi.Router) {
		r.Get("/data/", wl.handlerSimpleGet)
		r.Get("/lp/", wl.handlerLPGet)
		r.Get("/", wl.handleHtml)
		r.Handle("/static/*", http.StripPrefix(pattern, http.FileServer(http.FS(htmlStatic))))
	})
	return wl
}

func (h *WebLogWriter) Get(id uint64) []*WebLogJsonLine {
	l := len(h.lines)
	if l == 0 {
		return []*WebLogJsonLine{}
	}
	firstId := h.lines[0].ID
	if h.id < id {
		return []*WebLogJsonLine{}
	}
	if id < firstId {
		return h.lines[:min(maxLogChunk, uint64(l))]
	}
	startIndex := id - firstId
	return h.lines[startIndex:min(startIndex+maxLogChunk, uint64(l))]
}

func (h *WebLogWriter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.router.ServeHTTP(w, r)
}

func (h *WebLogWriter) handlerSimpleGet(w http.ResponseWriter, r *http.Request) {
	rid := r.URL.Query().Get("r")
	uniq := r.URL.Query().Get("uniq")
	var id uint64
	var err error
	if (uniq == "" || uniq == h.uniq) && rid != "" {
		id, err = strconv.ParseUint(rid, 10, 64)
		if err != nil {
			w.WriteHeader(500)
			return
		}
	}
	lines := h.Get(id)
	jb, err := json.Marshal(lines)
	if err != nil {
		w.WriteHeader(500)
		return
	}
	w.Header().Add("Content-Type", "application/json")
	w.Header().Add("uniq", h.uniq)
	w.WriteHeader(200)
	w.Write(jb)
}

func (h *WebLogWriter) handlerLPGet(w http.ResponseWriter, r *http.Request) {
	rid := r.URL.Query().Get("r")
	uniq := r.URL.Query().Get("uniq")
	var id uint64
	var err error
	if (uniq == "" || uniq == h.uniq) && rid != "" {
		id, err = strconv.ParseUint(rid, 10, 64)
		if err != nil {
			w.WriteHeader(500)
			return
		}
	}
	w.Header().Add("Content-Type", "application/json")
	w.Header().Add("uniq", h.uniq)
	if h.id > id {
		lines := h.Get(id)
		jb, err := json.Marshal(lines)
		if err != nil {
			w.WriteHeader(500)
			return
		}

		w.WriteHeader(200)
		w.Write(jb)
		return
	}
	h.lpLock.Lock()
	c := make(chan *WebLogJsonLine, 1)
	cid := atomic.AddUint64(&h.lpChanID, 1)
	h.lpChan[cid] = c
	h.lpLock.Unlock()
	select {
	case line := <-c:
		jl, err := json.Marshal([]*WebLogJsonLine{line})
		if err != nil {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(200)
		w.Write(jl)
		break
	case <-time.After(lpTTL):
		h.lpLock.Lock()
		delete(h.lpChan, cid)
		h.lpLock.Unlock()
		w.WriteHeader(408)
		break
	}
}

func (h *WebLogWriter) handleHtml(w http.ResponseWriter, r *http.Request) {
	b, err := render(indexTemplate, struct {
		MaxLogChunk       uint64
		MaxLogLines       int
		LpTTL             int64
		ShowLogLineNumber bool
	}{
		MaxLogChunk:       maxLogChunk,
		MaxLogLines:       maxLogLines,
		LpTTL:             lpTTL.Milliseconds(),
		ShowLogLineNumber: ShowLogLineNumber,
	})
	if err != nil {
		w.WriteHeader(500)
		fmt.Printf("WEBLOG ERROR %s\n", err.Error())
		return
	}
	w.WriteHeader(200)
	w.Write(b)
}

func min(a uint64, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

func render(templateByte []byte, data interface{}) ([]byte, error) {
	t, err := template.New("").Parse(string(templateByte))
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create template")
		return nil, err
	}
	var tpl bytes.Buffer
	err = t.Execute(&tpl, data)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to render template")
		return nil, err
	}
	return tpl.Bytes(), nil
}

func (h *WebLogWriter) NewWebLogHttpListener(address string) error {
	return http.ListenAndServe(address, h)
}

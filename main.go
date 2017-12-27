package app

import (
	"google.golang.org/appengine"
	"net/http"
)

func init() {
	http.Handle("/", ContextHandler(StaticWebsiteHandler))
}

type HttpResult struct {
	Status   int
	Message  string
	Location string
}

type ContextHandler func(w http.ResponseWriter, r *http.Request) HttpResult

func (h ContextHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	hr := h(w, r.WithContext(appengine.NewContext(r)))

	if hr.Location != "" {
		h := w.Header()
		for k := range h {
			delete(h, k)
		}
		if hr.Status == 0 {
			hr.Status = http.StatusTemporaryRedirect
		}
		http.Redirect(w, r, hr.Location, hr.Status)
		return
	}

	if hr.Status >= 400 {
		h := w.Header()
		for k := range h {
			delete(h, k)
		}
		if hr.Message == "" {
			hr.Message = http.StatusText(hr.Status)
		}
		http.Error(w, hr.Message, hr.Status)
		return
	}
}

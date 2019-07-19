package main

import (
	"google.golang.org/appengine"
	"net/http"
)

func main() {
	http.HandleFunc("/", Main)
	appengine.Main()
}

func Main(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)

	switch res := StaticWebsiteHandler(w, r.WithContext(ctx)); {

	case res.Location != "":
		if res.Status == 0 {
			res.Status = http.StatusTemporaryRedirect
		}
		http.Redirect(w, r, res.Location, res.Status)

	case res.Status >= 400:
		h := w.Header()
		for k := range h {
			delete(h, k)
		}
		if res.Message == "" {
			res.Message = http.StatusText(res.Status)
		}
		http.Error(w, res.Message, res.Status)

	case res.Status != 0:
		w.WriteHeader(res.Status)

	}
}

type HttpResult struct {
	Status   int
	Message  string
	Location string
}

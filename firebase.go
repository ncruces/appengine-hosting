package app

import (
	"net/http"
)

type FirebaseConfiguration struct {
	Redirects []struct {
		Source      string `json:"source"`
		Destination string `json:"destination"`
		Type        int    `json:"type,omitempty"`
	} `json:"redirects"`
	Rewrites []struct {
		Source      string `json:"source"`
		Destination string `json:"destination"`
	} `json:"rewrites"`
	Headers []struct {
		Source  string `json:"source"`
		Headers []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"headers"`
	} `json:"headers"`
}

func (c FirebaseConfiguration) processRedirects(path string) HttpResult {
	for _, redirect := range c.Redirects {
		pattern, err := CompileExtGlob(redirect.Source)
		if err != nil {
			return HttpResult{Status: http.StatusInternalServerError}
		}
		if pattern.MatchString(path) {
			return HttpResult{Status: redirect.Type, Location: redirect.Destination}
		}
	}
	return HttpResult{}
}

func (c FirebaseConfiguration) processRewrites(path string) string {
	for _, rewrite := range c.Rewrites {
		pattern, err := CompileExtGlob(rewrite.Source)
		if err != nil {
			return ""
		}
		if pattern.MatchString(path) {
			return rewrite.Destination
		}
	}
	return path
}

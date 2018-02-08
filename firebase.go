package app

import (
	"net/http"
	"strings"
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
	CleanUrls     bool  `json:"cleanUrls"`
	TrailingSlash *bool `json:"trailingSlash"`
}

func (c FirebaseConfiguration) processRedirects(path string) (int, string) {
	for _, redirect := range c.Redirects {
		pattern, err := CompileExtGlob("/" + strings.TrimPrefix(redirect.Source, "/"))
		if err != nil {
			return http.StatusInternalServerError, ""
		}
		if pattern.MatchString(path) {
			if redirect.Type == 0 {
				return http.StatusMovedPermanently, redirect.Destination
			}
			return redirect.Type, redirect.Destination
		}
	}
	return 0, ""
}

func (c FirebaseConfiguration) processRewrites(path string) string {
	for _, rewrite := range c.Rewrites {
		pattern, err := CompileExtGlob("/" + strings.TrimPrefix(rewrite.Source, "/"))
		if err != nil {
			return ""
		}
		if pattern.MatchString(path) {
			return rewrite.Destination
		}
	}
	return path
}

func (c FirebaseConfiguration) processHeaders(path string, header http.Header) {
	for _, headers := range c.Headers {
		pattern, err := CompileExtGlob("/" + strings.TrimPrefix(headers.Source, "/"))
		if err != nil {
			return
		}
		if pattern.MatchString(path) {
			for _, h := range headers.Headers {
				header.Set(h.Key, h.Value)
			}
		}
	}
}

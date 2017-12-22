package app

import (
	"encoding/xml"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/appengine/blobstore"
	"google.golang.org/appengine/urlfetch"
	"io"
	"net/http"
	"strings"
)

var websites = map[string]WebsiteConfiguration{}

type WebsiteConfiguration struct {
	MainPageSuffix string
	NotFoundPage   string
}

func StaticWebsiteHandler(w http.ResponseWriter, r *http.Request) HttpResult {
	if result := checkMethod(w, r); result != 0 {
		return HttpResult{Status: result}
	}

	bucket := r.URL.Hostname()
	object := r.URL.EscapedPath()

	gcsClient := &http.Client{
		Transport: &oauth2.Transport{
			Base:   &urlfetch.Transport{Context: r.Context()},
			Source: google.AppEngineTokenSource(r.Context(), "https://www.googleapis.com/auth/devstorage.read_only"),
		},
	}

	if !initWebsite(gcsClient, bucket) {
		if !strings.HasPrefix(bucket, "www.") && initWebsite(gcsClient, "www."+bucket) {
			r.URL.Host = "www." + r.URL.Host
			return HttpResult{Status: http.StatusMovedPermanently, Location: r.URL.String()}
		}
		return HttpResult{Status: http.StatusNotFound}
	}

	res, object := getMetadata(gcsClient, bucket, object)

	if res == nil {
		return HttpResult{Status: http.StatusInternalServerError}
	}
	if res.StatusCode == http.StatusNotFound {
		return sendNotFound(w, gcsClient, bucket)
	}
	if res.StatusCode != http.StatusOK {
		return HttpResult{Status: res.StatusCode, Message: res.Status}
	}
	if res.Header.Get("x-goog-stored-content-encoding") != "identity" {
		return HttpResult{Status: http.StatusNotImplemented}
	}

	etag := res.Header.Get("Etag")
	lastModified := res.Header.Get("Last-Modified")
	check := checkConditions(r, etag, lastModified)

	if check >= 400 {
		return HttpResult{Status: check}
	}
	if check == http.StatusNotModified {
		for name, values := range res.Header {
			switch name {
			case
				"Etag",
				"Last-Modified",
				"Cache-Control":
				w.Header()[name] = values
			}
		}
		return HttpResult{Status: http.StatusNotModified}
	} else {
		key, err := blobstore.BlobKeyForFile(r.Context(), "/gs/"+bucket+object)
		if err != nil {
			return HttpResult{Status: http.StatusInternalServerError}
		}

		for name, values := range res.Header {
			switch name {
			case
				"Etag",
				"Last-Modified",
				"Cache-Control",
				"Content-Type",
				"Content-Language",
				"Content-Disposition":
				w.Header()[name] = values
			}
		}

		sendHeaders(w)
		sendBlob(w, string(key))
		sendRange(w, r, etag, lastModified)
		return HttpResult{}
	}
}

func initWebsite(gcsClient *http.Client, bucket string) bool {
	var config WebsiteConfiguration

	if _, ok := websites[bucket]; ok {
		return true
	}

	res, _ := gcsClient.Get("https://storage.googleapis.com/" + bucket + "?websiteConfig")

	if res != nil {
		defer res.Body.Close()
	}
	if res == nil || res.StatusCode != http.StatusOK || xml.NewDecoder(res.Body).Decode(&config) != nil {
		return false
	}

	websites[bucket] = config
	return true
}

func getMetadata(gcsClient *http.Client, bucket string, object string) (*http.Response, string) {
	website := websites[bucket]
	notFoundPage := "/" + website.NotFoundPage
	mainPageSuffix := "/" + website.MainPageSuffix

	if len(object) <= 1 {
		object = mainPageSuffix
	}
	if len(object) <= 1 || object == notFoundPage {
		return &http.Response{StatusCode: http.StatusNotFound}, object
	}

	res, _ := gcsClient.Head("https://storage.googleapis.com/" + bucket + object)

	if res != nil && (res.StatusCode == http.StatusNotFound || strings.HasSuffix(object, "/") && res.Header.Get("x-goog-stored-content-length") == "0") {
		object = strings.TrimRight(object, "/") + mainPageSuffix
		res, _ = gcsClient.Head("https://storage.googleapis.com/" + bucket + object)
	}

	return res, object
}

func checkMethod(w http.ResponseWriter, r *http.Request) int {
	if r.Method != "GET" && r.Method != "HEAD" {
		w.Header().Set("Allow", "GET, HEAD")
		return http.StatusMethodNotAllowed
	}
	return 0
}

func checkConditions(r *http.Request, etag string, lastModified string) int {
	modified, err := http.ParseTime(lastModified)

	if etag == "" || etag[0] != '"' || err != nil {
		return http.StatusInternalServerError
	}

	if headers, ok := r.Header["If-Match"]; ok {
		match := false
		for _, header := range headers {
			if header == "*" || strings.Contains(header, etag) {
				match = true
				break
			}
		}
		if !match {
			return http.StatusPreconditionFailed
		}
	} else {
		since, err := http.ParseTime(r.Header.Get("If-Unmodified-Since"))
		if err == nil && modified.After(since) {
			return http.StatusPreconditionFailed
		}
	}

	if headers, ok := r.Header["If-None-Match"]; ok {
		match := false
		for _, header := range headers {
			if header == "*" || strings.Contains(header, etag) {
				match = true
				break
			}
		}
		if match {
			return http.StatusNotModified
		}
	} else {
		since, err := http.ParseTime(r.Header.Get("If-Modified-Since"))
		if err == nil && !modified.After(since) {
			return http.StatusNotModified
		}
	}

	return 0
}

func sendBlob(w http.ResponseWriter, key string) {
	w.Header().Set("X-AppEngine-BlobKey", key)
}

func sendRange(w http.ResponseWriter, r *http.Request, etag string, modified string) {
	header := r.Header.Get("Range")
	if len(header) > 0 {
		condition := r.Header.Get("If-Range")
		if len(condition) == 0 || condition == etag || condition == modified {
			w.Header().Set("X-AppEngine-BlobRange", header)
		} else {
			w.Header().Set("X-AppEngine-BlobRange", "")
		}
	}
}

func sendHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Download-Options", "noopen")
	h.Set("X-Frame-Options", "SAMEORIGIN")
	h.Set("X-XSS-Protection", "1; mode=block")
}

func sendNotFound(w http.ResponseWriter, gcsClient *http.Client, bucket string) HttpResult {
	website := websites[bucket]
	notFoundPage := "/" + website.NotFoundPage

	if len(notFoundPage) <= 1 {
		return HttpResult{Status: http.StatusNotFound}
	}

	res, _ := gcsClient.Get("https://storage.googleapis.com/" + bucket + notFoundPage)

	if res != nil {
		defer res.Body.Close()
	}
	if res == nil || res.StatusCode != http.StatusOK {
		return HttpResult{Status: http.StatusNotFound}
	}

	for name, values := range res.Header {
		switch name {
		case
			"Content-Type",
			"Content-Language",
			"Content-Disposition":
			w.Header()[name] = values
		}
	}

	w.WriteHeader(http.StatusNotFound)
	io.Copy(w, res.Body)
	return HttpResult{}
}

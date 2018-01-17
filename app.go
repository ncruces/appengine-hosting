package app

import (
	"encoding/json"
	"encoding/xml"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/appengine/blobstore"
	"google.golang.org/appengine/urlfetch"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

var websites = map[string]WebsiteConfiguration{}
var firebase = map[string]FirebaseConfiguration{}

type WebsiteConfiguration struct {
	MainPageSuffix string
	NotFoundPage   string
}

func init() {
	f, err := os.Open("firebase.json")
	if err == nil {
		defer f.Close()
		json.NewDecoder(f).Decode(&firebase)
	}
}

func StaticWebsiteHandler(w http.ResponseWriter, r *http.Request) HttpResult {
	if code := checkMethod(w, r); code >= 400 {
		return HttpResult{Status: code}
	}

	bucket := r.URL.Hostname()
	object := r.URL.EscapedPath()
	config := firebase[bucket]

	if res := config.processRedirects(r.URL.Path); res.Status != 0 {
		return res
	}

	gcs := &http.Client{
		Transport: &oauth2.Transport{
			Base:   &urlfetch.Transport{Context: r.Context()},
			Source: google.AppEngineTokenSource(r.Context(), "https://www.googleapis.com/auth/devstorage.read_only"),
		},
	}

	if code := initWebsite(gcs, bucket); code >= 400 {
		return HttpResult{Status: code}
	}

	res, object := getMetadata(gcs, bucket, object)

	if res == nil {
		return HttpResult{Status: http.StatusInternalServerError}
	}
	if res.StatusCode == http.StatusNotFound {
		return sendNotFound(w, gcs, bucket)
	}
	if res.StatusCode >= 400 {
		return HttpResult{Status: res.StatusCode, Message: res.Status}
	}
	if res.StatusCode != http.StatusOK {
		return HttpResult{Status: http.StatusInternalServerError}
	}

	etag := res.Header.Get("Etag")
	lastModified := res.Header.Get("Last-Modified")
	code := checkConditions(r, etag, lastModified, true)

	if code >= 400 {
		return HttpResult{Status: code}
	}
	if code == http.StatusNotModified {
		w.Header()["Cache-Control"] = res.Header["Cache-Control"]
		return HttpResult{Status: code}
	} else {
		setHeaders(w.Header())
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		w.Header()["Cache-Control"] = res.Header["Cache-Control"]
		w.Header()["Content-Type"] = res.Header["Content-Type"]
		w.Header()["Content-Language"] = res.Header["Content-Language"]
		w.Header()["Content-Disposition"] = res.Header["Content-Disposition"]

		if res.Header.Get("x-goog-stored-content-encoding") == "identity" {
			return sendBlob(w, r, bucket, object, etag, lastModified, true)
		} else {
			return sendEncodedBlob(w, gcs, bucket, object)
		}
	}
}

func initWebsite(gcs *http.Client, bucket string) int {
	var config WebsiteConfiguration

	if _, ok := websites[bucket]; ok {
		return 0
	}

	res, _ := gcs.Get("https://storage.googleapis.com/" + bucket + "?websiteConfig")
	if res != nil {
		defer res.Body.Close()

		if res.StatusCode >= 400 {
			return res.StatusCode
		}
		if res.StatusCode == http.StatusOK && xml.NewDecoder(res.Body).Decode(&config) == nil {
			websites[bucket] = config
			return 0
		}
	}

	return http.StatusInternalServerError
}

func getMetadata(gcs *http.Client, bucket string, object string) (*http.Response, string) {
	website := websites[bucket]
	notFoundPage := "/" + website.NotFoundPage
	mainPageSuffix := "/" + website.MainPageSuffix

	if len(object) <= 1 {
		object = mainPageSuffix
	}
	if len(object) <= 1 || object == notFoundPage {
		return &http.Response{StatusCode: http.StatusNotFound}, object
	}

	res, _ := gcs.Head("https://storage.googleapis.com/" + bucket + object)

	if res != nil && (res.StatusCode == http.StatusNotFound || strings.HasSuffix(object, "/") && res.Header.Get("x-goog-stored-content-length") == "0") {
		object = strings.TrimRight(object, "/") + mainPageSuffix
		res, _ = gcs.Head("https://storage.googleapis.com/" + bucket + object)
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

func checkConditions(r *http.Request, etag string, lastModified string, mutable bool) int {
	modified, err := http.ParseTime(lastModified)

	if etag == "" || etag[0] != '"' || err != nil {
		return http.StatusInternalServerError
	}

	if matchers, ok := r.Header["If-Match"]; ok {
		match := false
		for _, matcher := range matchers {
			if matcher == "*" || strings.Contains(matcher, etag) && !mutable {
				match = true
				break
			}
		}
		if !match {
			return http.StatusPreconditionFailed
		}
	} else {
		since, err := http.ParseTime(r.Header.Get("If-Unmodified-Since"))
		if err == nil && (modified.After(since) || mutable) {
			return http.StatusPreconditionFailed
		}
	}

	if matchers, ok := r.Header["If-None-Match"]; ok {
		match := false
		for _, matcher := range matchers {
			if matcher == "*" || strings.Contains(matcher, etag) {
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

func setHeaders(h http.Header) {
	h.Set("Content-Security-Policy", "default-src * 'unsafe-eval' 'unsafe-inline' data: blob: filesystem: about: ws: wss:")
	h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
	h.Set("Strict-Transport-Security", "max-age=86400")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Download-Options", "noopen")
	h.Set("X-Frame-Options", "SAMEORIGIN")
	h.Set("X-XSS-Protection", "1; mode=block")
}

func sendBlob(w http.ResponseWriter, r *http.Request, bucket string, object string, etag string, modified string, mutable bool) HttpResult {
	key, err := blobstore.BlobKeyForFile(r.Context(), "/gs/"+bucket+object)
	if err != nil {
		return HttpResult{Status: http.StatusInternalServerError}
	}

	if header := r.Header.Get("Range"); len(header) > 0 {
		condition := r.Header.Get("If-Range")
		if len(condition) != 0 && condition != etag && condition != modified {
			header = ""
		}
		if mutable {
			header = ""
		}
		w.Header().Set("X-AppEngine-BlobRange", header)
	}

	w.Header().Set("X-AppEngine-BlobKey", string(key))
	return HttpResult{}
}

func sendEncodedBlob(w http.ResponseWriter, gcs *http.Client, bucket string, object string) HttpResult {
	res, _ := gcs.Get("https://storage.googleapis.com/" + bucket + object)

	if res != nil {
		defer res.Body.Close()
	}
	if res == nil || res.StatusCode != http.StatusOK {
		return HttpResult{Status: http.StatusInternalServerError}
	}

	io.Copy(w, res.Body)
	return HttpResult{}
}

func sendNotFound(w http.ResponseWriter, gcs *http.Client, bucket string) HttpResult {
	website := websites[bucket]
	notFoundPage := "/" + website.NotFoundPage

	if len(notFoundPage) <= 1 {
		return HttpResult{Status: http.StatusNotFound}
	}

	res, _ := gcs.Get("https://storage.googleapis.com/" + bucket + notFoundPage)

	if res != nil {
		defer res.Body.Close()
	}
	if res == nil || res.StatusCode != http.StatusOK {
		return HttpResult{Status: http.StatusNotFound}
	}

	w.Header()["Content-Type"] = res.Header["Content-Type"]
	w.Header()["Content-Language"] = res.Header["Content-Language"]
	w.Header()["Content-Disposition"] = res.Header["Content-Disposition"]
	w.WriteHeader(http.StatusNotFound)
	io.Copy(w, res.Body)
	return HttpResult{}
}

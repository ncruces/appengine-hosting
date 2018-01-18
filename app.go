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

type HandlerContext struct {
	w        http.ResponseWriter
	r        *http.Request
	gcs      *http.Client
	bucket   string
	object   string
	website  WebsiteConfiguration
	firebase FirebaseConfiguration
}

func StaticWebsiteHandler(w http.ResponseWriter, r *http.Request) HttpResult {
	if code := checkMethod(w, r); code >= 400 {
		return HttpResult{Status: code}
	}

	ctx := makeContext(w, r)

	if res := ctx.firebase.processRedirects(r.URL.Path); res.Status != 0 {
		return res
	}

	if code := ctx.initWebsite(); code >= 400 {
		return HttpResult{Status: code}
	}

	res := ctx.getMetadata()

	if res == nil {
		return HttpResult{Status: http.StatusInternalServerError}
	}
	if res.StatusCode == http.StatusNotFound {
		return ctx.sendNotFound()
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
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		w.Header()["Cache-Control"] = res.Header["Cache-Control"]
		w.Header()["Content-Type"] = res.Header["Content-Type"]
		w.Header()["Content-Language"] = res.Header["Content-Language"]
		w.Header()["Content-Disposition"] = res.Header["Content-Disposition"]
		ctx.setHeaders()

		if res.Header.Get("x-goog-stored-content-encoding") == "identity" {
			return ctx.sendBlob(etag, lastModified, true)
		} else {
			return ctx.sendEncodedBlob()
		}
	}
}

func makeContext(w http.ResponseWriter, r *http.Request) HandlerContext {
	bucket := r.URL.Hostname()
	object := r.URL.EscapedPath()

	return HandlerContext{
		w:        w,
		r:        r,
		bucket:   bucket,
		object:   object,
		website:  websites[bucket],
		firebase: firebase[bucket],
		gcs: &http.Client{
			Transport: &oauth2.Transport{
				Base:   &urlfetch.Transport{Context: r.Context()},
				Source: google.AppEngineTokenSource(r.Context(), "https://www.googleapis.com/auth/devstorage.read_only"),
			},
		},
	}
}

func (ctx *HandlerContext) initWebsite() int {
	if _, ok := websites[ctx.bucket]; ok {
		return 0
	}

	res, _ := ctx.gcs.Get("https://storage.googleapis.com/" + ctx.bucket + "?websiteConfig")
	if res != nil {
		defer res.Body.Close()

		if res.StatusCode >= 400 {
			return res.StatusCode
		}
		if res.StatusCode == http.StatusOK && xml.NewDecoder(res.Body).Decode(&ctx.website) == nil {
			websites[ctx.bucket] = ctx.website
			return 0
		}
	}

	return http.StatusInternalServerError
}

func (ctx *HandlerContext) getMetadata() *http.Response {
	notFoundPage := "/" + ctx.website.NotFoundPage
	mainPageSuffix := "/" + ctx.website.MainPageSuffix

	if len(ctx.object) <= 1 {
		ctx.object = mainPageSuffix
	}
	if len(ctx.object) <= 1 || ctx.object == notFoundPage {
		if r := ctx.getRewriteMetadata(ctx.firebase.processRewrites(ctx.r.URL.Path)); r != nil {
			return r
		}
		return &http.Response{StatusCode: http.StatusNotFound}
	}

	res, _ := ctx.gcs.Head("https://storage.googleapis.com/" + ctx.bucket + ctx.object)

	if res != nil && (res.StatusCode == http.StatusNotFound || strings.HasSuffix(ctx.object, "/") && res.Header.Get("x-goog-stored-content-length") == "0") {
		if r := ctx.getRewriteMetadata(strings.TrimRight(ctx.object, "/") + mainPageSuffix); r != nil {
			return r
		}
		if r := ctx.getRewriteMetadata(ctx.firebase.processRewrites(ctx.r.URL.Path)); r != nil {
			return r
		}
	}

	return res
}

func (ctx *HandlerContext) getRewriteMetadata(rewrite string) *http.Response {
	if rewrite != "" && rewrite != ctx.object {
		res, _ := ctx.gcs.Head("https://storage.googleapis.com/" + ctx.bucket + rewrite)
		if res != nil && res.StatusCode == http.StatusOK {
			ctx.object = rewrite
			return res
		}
	}
	return nil
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

func (ctx *HandlerContext) setHeaders() {
	h := ctx.w.Header()
	h.Set("Content-Security-Policy", "default-src * 'unsafe-eval' 'unsafe-inline' data: blob: filesystem: about: ws: wss:")
	h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
	h.Set("Strict-Transport-Security", "max-age=86400")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Download-Options", "noopen")
	h.Set("X-Frame-Options", "SAMEORIGIN")
	h.Set("X-XSS-Protection", "1; mode=block")
	ctx.firebase.processHeaders(ctx.r.URL.Path, h)
}

func (ctx *HandlerContext) sendBlob(etag string, modified string, mutable bool) HttpResult {
	key, err := blobstore.BlobKeyForFile(ctx.r.Context(), "/gs/"+ctx.bucket+ctx.object)
	if err != nil {
		return HttpResult{Status: http.StatusInternalServerError}
	}

	if header := ctx.r.Header.Get("Range"); len(header) > 0 {
		condition := ctx.r.Header.Get("If-Range")
		if len(condition) != 0 && condition != etag && condition != modified {
			header = ""
		}
		if mutable {
			header = ""
		}
		ctx.w.Header().Set("X-AppEngine-BlobRange", header)
	}

	ctx.w.Header().Set("X-AppEngine-BlobKey", string(key))
	return HttpResult{}
}

func (ctx *HandlerContext) sendEncodedBlob() HttpResult {
	res, _ := ctx.gcs.Get("https://storage.googleapis.com/" + ctx.bucket + ctx.object)

	if res != nil {
		defer res.Body.Close()
	}
	if res == nil || res.StatusCode != http.StatusOK {
		return HttpResult{Status: http.StatusInternalServerError}
	}

	io.Copy(ctx.w, res.Body)
	return HttpResult{}
}

func (ctx *HandlerContext) sendNotFound() HttpResult {
	notFoundPage := "/" + ctx.website.NotFoundPage

	if len(notFoundPage) <= 1 {
		return HttpResult{Status: http.StatusNotFound}
	}

	res, _ := ctx.gcs.Get("https://storage.googleapis.com/" + ctx.bucket + notFoundPage)

	if res != nil {
		defer res.Body.Close()
	}
	if res == nil || res.StatusCode != http.StatusOK {
		return HttpResult{Status: http.StatusNotFound}
	}

	ctx.w.Header()["Content-Type"] = res.Header["Content-Type"]
	ctx.w.Header()["Content-Language"] = res.Header["Content-Language"]
	ctx.w.Header()["Content-Disposition"] = res.Header["Content-Disposition"]
	ctx.w.WriteHeader(http.StatusNotFound)
	io.Copy(ctx.w, res.Body)
	return HttpResult{}
}

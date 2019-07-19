# appengine-hosting

[Google Cloud Storage](https://cloud.google.com/storage/) allows you to configure a bucket to [host a static website](https://cloud.google.com/storage/docs/hosting-static-website), with one big caveat: no HTTPS support.

[Proposed solutions](https://cloud.google.com/storage/docs/troubleshooting#https) are: [a load balancer](https://cloud.google.com/compute/docs/load-balancing/http/adding-a-backend-bucket-to-content-based-load-balancing), a [third-party CDN](https://cloud.google.com/interconnect/docs/how-to/cdn-interconnect), and [Firebase Hosting](https://firebase.google.com/docs/hosting/).

This offers an additional, cost effective, customizable, alternative: a single App Engine app that can host as many static websites as needed.

### How and why?

A [Go](https://golang.org/), [App Engine](https://cloud.google.com/appengine/) app, serves static content from [Cloud Storage](https://cloud.google.com/storage/) buckets. [Always Free](https://cloud.google.com/free/docs/gcp-free-tier#always-free) mean you can host this for free, and scale up from there.

Follow the [Quickstart for Go App Engine Standard Environment](https://cloud.google.com/appengine/docs/standard/go111/quickstart) tutorial. Choose the `us-central` region if you want to take full advantage of [Always Free](https://cloud.google.com/free/docs/gcp-free-tier#always-free-usage-limits). Don't clean up.

Follow the [Hosting a Static Website](https://cloud.google.com/storage/docs/hosting-static-website) tutorial. Use the same project as above. For best performance, place the bucket in the same region as above (pick `us-central1` for the [Always Free](https://cloud.google.com/free/docs/gcp-free-tier#always-free-usage-limits) allowance). Verify that everything works as expected through the custom domain. Don't clean up.

Now, clone this repository and deploy it to App Engine. Then, [map the custom domain](https://cloud.google.com/appengine/docs/standard/go111/mapping-custom-domains) for the website to the App Engine app. Wait for the [managed SSL certificate](https://cloud.google.com/appengine/docs/standard/go111/securing-custom-domains-with-ssl#verify_a_managed_certificate_has_been_provisioned) to be provisioned for your app, or configure [your own SSL certificate](https://cloud.google.com/appengine/docs/standard/go111/securing-custom-domains-with-ssl#using_your_own_ssl_certificates).

You should now be able to use HTTPS to access the website.

### What works, and what doesn't?

* Website configuration for the bucket (Main page, and 404 page) is respected by default.
* Multiple domains can be mapped to the app, content will be served from the corresponding buckets.
* All HTTP traffic is 301 redirected to HTTPS (see [app.yaml](app.yaml))
* Some [security headers](https://securityheaders.com/) are added, many [Cloud Storage headers](https://cloud.google.com/storage/docs/xml-api/reference-headers) are hidden.
* Redirects, rewrites, etc, as in [Firebase Hosting](https://firebase.google.com/docs/hosting/full-config) (see [firebase-sample.json](firebase-sample.json)).
* This [issue](https://issuetracker.google.com/issues/70223986) means compressed objects in Cloud Storage larger than 32Mb are not supported (don't use `gsutil -z` or `-Z` to upload them).
* This [issue](https://cloud.google.com/storage/docs/troubleshooting#empty-obj) is fixed.

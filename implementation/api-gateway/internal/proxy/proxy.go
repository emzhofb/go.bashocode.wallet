package proxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/google/uuid"
)

type ReverseProxy struct {
	target *url.URL
	proxy  *httputil.ReverseProxy
}

func NewReverseProxy(targetURL string) (*ReverseProxy, error) {
	url, err := url.Parse(targetURL)
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(url)

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Header.Set("X-Forwarded-Host", req.Header.Get("Host"))
		req.Host = url.Host

		// Inject & forward Request Correlation ID for distributed logging
		corID := req.Header.Get("X-Correlation-ID")
		if corID == "" {
			corID = uuid.New().String()
		}
		req.Header.Set("X-Correlation-ID", corID)
	}

	return &ReverseProxy{
		target: url,
		proxy:  proxy,
	}, nil
}

func (p *ReverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.proxy.ServeHTTP(w, r)
}

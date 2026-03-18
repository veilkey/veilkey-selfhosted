package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestProxyAllowsHTTPRequests(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok-http")
	}))
	defer upstream.Close()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	proxy := &http.Server{
		Handler: &proxyServer{
			allowHosts: map[string]struct{}{},
			client:     &http.Client{Transport: &http.Transport{Proxy: nil}},
		},
	}
	go proxy.Serve(listener)
	defer shutdownProxyServer(proxy)

	proxyURL := "http://" + listener.Addr().String()
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: func(*http.Request) (*url.URL, error) {
				return url.Parse(proxyURL)
			},
		},
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(upstream.URL)
	if err != nil {
		t.Fatalf("get through proxy: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok-http" {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestProxyAllowsHTTPSConnect(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok-https")
	}))
	defer upstream.Close()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	proxy := &http.Server{
		Handler: &proxyServer{
			allowHosts: map[string]struct{}{},
			client:     &http.Client{Transport: &http.Transport{Proxy: nil}},
		},
	}
	go proxy.Serve(listener)
	defer shutdownProxyServer(proxy)

	proxyURL := "http://" + listener.Addr().String()
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: func(*http.Request) (*url.URL, error) {
				return url.Parse(proxyURL)
			},
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(upstream.URL)
	if err != nil {
		t.Fatalf("https get through proxy: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok-https" {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestProxyBlocksDisallowedHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://blocked.example/path", nil)
	rec := httptest.NewRecorder()
	server := &proxyServer{
		allowHosts: map[string]struct{}{"allowed.example": {}},
		client:     &http.Client{Transport: &http.Transport{Proxy: nil}},
	}

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden, got %d", rec.Code)
	}
}

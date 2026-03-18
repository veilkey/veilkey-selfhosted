package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type multiFlag []string

func (m *multiFlag) String() string {
	return strings.Join(*m, ",")
}

func (m *multiFlag) Set(value string) error {
	*m = append(*m, value)
	return nil
}

type proxyServer struct {
	allowHosts map[string]struct{}
	client     *http.Client
	logMu      sync.Mutex
}

type proxyLogEntry struct {
	Time   string `json:"time"`
	Method string `json:"method"`
	Host   string `json:"host"`
	Status int    `json:"status"`
	Allow  bool   `json:"allow"`
	Error  string `json:"error,omitempty"`
}

func cmdProxy(args []string) {
	fs := flag.NewFlagSet("proxy", flag.ExitOnError)
	listen := fs.String("listen", "", "listen address")
	var allowHosts multiFlag
	fs.Var(&allowHosts, "allow-host", "allowed hostname (repeatable)")
	fs.Parse(args)
	if strings.TrimSpace(*listen) == "" {
		fmt.Fprintln(os.Stderr, "proxy listen address is required (--listen)")
		os.Exit(1)
	}

	server := &proxyServer{
		allowHosts: make(map[string]struct{}, len(allowHosts)),
		client: &http.Client{
			Timeout: 0,
			Transport: &http.Transport{
				Proxy: nil,
			},
		},
	}
	for _, host := range allowHosts {
		server.allowHosts[strings.ToLower(host)] = struct{}{}
	}

	httpServer := &http.Server{
		Addr:    strings.TrimSpace(*listen),
		Handler: server,
	}
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "proxy listen failed: %v\n", err)
		os.Exit(1)
	}
}

func (p *proxyServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := canonicalProxyHost(r)
	if !p.hostAllowed(host) {
		p.writeAccessLog(r.Method, host, http.StatusForbidden, false, "host not allowed")
		http.Error(w, "host not allowed", http.StatusForbidden)
		return
	}
	if r.Method == http.MethodConnect {
		p.handleConnect(w, r, host)
		return
	}
	p.handleHTTP(w, r, host)
}

func (p *proxyServer) handleHTTP(w http.ResponseWriter, r *http.Request, host string) {
	upstreamURL := *r.URL
	if upstreamURL.Scheme == "" {
		upstreamURL.Scheme = "http"
	}
	if upstreamURL.Host == "" {
		upstreamURL.Host = r.Host
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL.String(), r.Body)
	if err != nil {
		p.writeAccessLog(r.Method, host, http.StatusBadRequest, true, err.Error())
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	req.Header = r.Header.Clone()
	req.Header.Del("Proxy-Connection")
	req.RequestURI = ""

	resp, err := p.client.Do(req)
	if err != nil {
		p.writeAccessLog(r.Method, host, http.StatusBadGateway, true, err.Error())
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
	p.writeAccessLog(r.Method, host, resp.StatusCode, true, "")
}

func (p *proxyServer) handleConnect(w http.ResponseWriter, r *http.Request, host string) {
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		p.writeAccessLog(r.Method, host, http.StatusInternalServerError, true, "hijacking not supported")
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}

	targetConn, err := net.DialTimeout("tcp", ensurePort(r.Host, "443"), 10*time.Second)
	if err != nil {
		p.writeAccessLog(r.Method, host, http.StatusBadGateway, true, err.Error())
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		targetConn.Close()
		p.writeAccessLog(r.Method, host, http.StatusInternalServerError, true, err.Error())
		http.Error(w, "hijack failed", http.StatusInternalServerError)
		return
	}

	_, _ = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	p.writeAccessLog(r.Method, host, http.StatusOK, true, "")

	go proxyCopy(targetConn, clientConn)
	go proxyCopy(clientConn, targetConn)
}

func proxyCopy(dst net.Conn, src net.Conn) {
	defer dst.Close()
	defer src.Close()
	_, _ = io.Copy(dst, src)
}

func (p *proxyServer) hostAllowed(host string) bool {
	if len(p.allowHosts) == 0 {
		return true
	}
	host = strings.ToLower(host)
	_, ok := p.allowHosts[host]
	return ok
}

func canonicalProxyHost(r *http.Request) string {
	host := r.URL.Hostname()
	if host == "" {
		host = r.URL.Host
	}
	if host == "" {
		host = r.Host
	}
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	return strings.ToLower(host)
}

func ensurePort(addr, defaultPort string) string {
	if _, _, err := net.SplitHostPort(addr); err == nil {
		return addr
	}
	return net.JoinHostPort(addr, defaultPort)
}

func copyHeader(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func (p *proxyServer) writeAccessLog(method, host string, status int, allow bool, errMsg string) {
	entry := proxyLogEntry{
		Time:   time.Now().UTC().Format(time.RFC3339),
		Method: method,
		Host:   host,
		Status: status,
		Allow:  allow,
		Error:  errMsg,
	}
	p.logMu.Lock()
	defer p.logMu.Unlock()
	_ = json.NewEncoder(os.Stdout).Encode(entry)
}

func shutdownProxyServer(srv *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

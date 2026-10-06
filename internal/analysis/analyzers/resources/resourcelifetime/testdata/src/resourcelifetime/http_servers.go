package resourcelifetime

// The accepted local-server cases depend on exact request/server identity and
// complete header-only writer effects. They do not establish global transport
// immutability; mutation hidden in other packages remains a coverage gap.

import (
	"net/http"
	"net/http/httptest"
	"time"
)

func headersOnly(w http.ResponseWriter, _ *http.Request) {
	setResponseCookie(w)
	w.WriteHeader(http.StatusOK)
}

func setResponseCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: "session", Value: "value"})
	w.Header().Set("Content-Type", "text/plain")
}

func localHeaderOnlyResponse() {
	server := httptest.NewServer(http.HandlerFunc(headersOnly))
	defer server.Close()
	_, _ = http.Get(server.URL + "/cookies")
}

func localBodyResponse() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("body"))
	}))
	defer server.Close()
	_, _ = http.Get(server.URL) // want "owned resource from http.Get is not released"
}

func localFramedResponse() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(200)
	}))
	defer server.Close()
	_, _ = http.Get(server.URL) // want "owned resource from http.Get is not released"
}

func localChunkedResponse() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Transfer-Encoding", "chunked")
		w.WriteHeader(200)
	}))
	defer server.Close()
	_, _ = http.Get(server.URL) // want "owned resource from http.Get is not released"
}

func localDynamicHeaderResponse(key string) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(key, "100")
	}))
	defer server.Close()
	_, _ = http.Get(server.URL) // want "owned resource from http.Get is not released"
}

func localFlushedResponse() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.(http.Flusher).Flush()
	}))
	defer server.Close()
	_, _ = http.Get(server.URL) // want "owned resource from http.Get is not released"
}

func localHijackedResponse() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _, _ = w.(http.Hijacker).Hijack()
	}))
	defer server.Close()
	_, _ = http.Get(server.URL) // want "owned resource from http.Get is not released"
}

func localAsyncResponse() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		go func() { _, _ = w.Write([]byte("body")) }()
	}))
	defer server.Close()
	_, _ = http.Get(server.URL) // want "owned resource from http.Get is not released"
}

func localEscapedWriterResponse(use func(http.ResponseWriter)) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { use(w) }))
	defer server.Close()
	_, _ = http.Get(server.URL) // want "owned resource from http.Get is not released"
}

func localRedirectResponse() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "http://example.com")
		w.WriteHeader(302)
	}))
	defer server.Close()
	_, _ = http.Get(server.URL) // want "owned resource from http.Get is not released"
}

func localChangedHandlerResponse() {
	server := httptest.NewServer(http.HandlerFunc(headersOnly))
	defer server.Close()
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("body")) })
	_, _ = http.Get(server.URL) // want "owned resource from http.Get is not released"
}

func localChangedURLResponse(other string) {
	server := httptest.NewServer(http.HandlerFunc(headersOnly))
	defer server.Close()
	server.URL = other
	_, _ = http.Get(server.URL) // want "owned resource from http.Get is not released"
}

func localChangedClientResponse() {
	server := httptest.NewServer(http.HandlerFunc(headersOnly))
	defer server.Close()
	http.DefaultClient.Timeout = time.Second
	_, _ = http.Get(server.URL) // want "owned resource from http.Get is not released"
}

func replaceHTTPClient() { http.DefaultClient = &http.Client{Timeout: time.Second} }

func localIndirectClientChangeResponse() {
	server := httptest.NewServer(http.HandlerFunc(headersOnly))
	defer server.Close()
	replaceHTTPClient()
	_, _ = http.Get(server.URL) // want "owned resource from http.Get is not released"
}

func localChangedTransportResponse(transport http.RoundTripper) {
	server := httptest.NewServer(http.HandlerFunc(headersOnly))
	defer server.Close()
	http.DefaultTransport = transport
	_, _ = http.Get(server.URL) // want "owned resource from http.Get is not released"
}

func localTrailerResponse() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Trailer", "X-Final-Status")
		w.WriteHeader(200)
	}))
	defer server.Close()
	_, _ = http.Get(server.URL) // want "owned resource from http.Get is not released"
}

func localServerClientHeaderOnlyResponse() {
	server := httptest.NewServer(http.HandlerFunc(headersOnly))
	defer server.Close()
	_, _ = server.Client().Get(server.URL + "/cookies")
}

func localServerClientBodyResponse() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("body"))
	}))
	defer server.Close()
	_, _ = server.Client().Get(server.URL) // want "owned resource from http.Get is not released"
}

func localServerClientWithTimeoutResponse() {
	server := httptest.NewServer(http.HandlerFunc(headersOnly))
	defer server.Close()
	client := server.Client()
	client.Timeout = time.Second
	_, _ = client.Get(server.URL) // want "owned resource from http.Get is not released"
}

func localOtherClientResponse(client *http.Client) {
	server := httptest.NewServer(http.HandlerFunc(headersOnly))
	defer server.Close()
	_, _ = client.Get(server.URL) // want "owned resource from http.Get is not released"
}

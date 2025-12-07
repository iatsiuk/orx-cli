package testutil

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func NewTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

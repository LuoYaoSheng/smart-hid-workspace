package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGated_All(t *testing.T) {
	ts := httptest.NewServer(Gated(true, true))
	defer ts.Close()
	for _, p := range []string{"/", "/demo.html", "/demo.js", "/app.js", "/api-test.html"} {
		if code := get(t, ts.URL+p); code != 200 {
			t.Errorf("all-on %s = %d, want 200", p, code)
		}
	}
}

func TestGated_DemoOff(t *testing.T) {
	ts := httptest.NewServer(Gated(true, false))
	defer ts.Close()
	if code := get(t, ts.URL+"/demo.html"); code != 404 {
		t.Errorf("demo off /demo.html = %d, want 404", code)
	}
	if code := get(t, ts.URL+"/"); code != 200 {
		t.Errorf("demo off / = %d, want 200", code)
	}
}

func TestGated_ConsoleOff(t *testing.T) {
	ts := httptest.NewServer(Gated(false, true))
	defer ts.Close()
	for _, p := range []string{"/", "/index.html", "/app.js", "/api-test.html"} {
		if code := get(t, ts.URL+p); code != 404 {
			t.Errorf("console off %s = %d, want 404", p, code)
		}
	}
	if code := get(t, ts.URL+"/demo.html"); code != 200 {
		t.Errorf("console off /demo.html = %d, want 200", code)
	}
}

func get(t *testing.T, url string) int {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	return res.StatusCode
}

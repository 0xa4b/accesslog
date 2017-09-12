package accesslog

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// withTime sets the time to use when logging. This should be used only for testing
func withTime(t time.Time) optFunc {
	return func(o *opt) {
		o.Time = t
	}
}

func HandlerTesting(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	io.WriteString(w, `{"testing": true}`)
}

func TestLoggingMiddleware(t *testing.T) {
	req, err := http.NewRequest("GET", "/testing", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	buf := new(bytes.Buffer)
	tm, err := time.Parse("Jan 2, 2006 at 3:04pm (MST)", "Feb 3, 2013 at 7:54pm (PST)")
	if err != nil {
		t.Errorf("parse time error: %v", err)
	}
	aLog := Format(ApacheCommonLogFormat, WithOutput(buf), withTime(tm))
	handler := aLog(http.HandlerFunc(HandlerTesting))

	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("wrong status code: got %v expect %v",
			status, http.StatusOK)
	}
	expected := `{"testing": true}`
	if rr.Body.String() != expected {
		t.Errorf("wrong body: got %v expect %v",
			rr.Body.String(), expected)
	}
}

func TestLoggingMiddlewareWithUser(t *testing.T) {
	req, err := http.NewRequest("GET", "/testing", nil)
	if err != nil {
		t.Fatal(err)
	}

	req.SetBasicAuth("Frank", "<none>")
	rr := httptest.NewRecorder()
	buf := new(bytes.Buffer)
	tm, err := time.Parse("Jan 2, 2006 at 3:04pm (MST)", "Feb 3, 2013 at 7:54pm (PST)")
	if err != nil {
		t.Errorf("parse time error: %v", err)
	}
	aLog := ApacheCommonLog(WithOutput(buf), withTime(tm))
	handler := aLog(http.HandlerFunc(HandlerTesting))

	handler.ServeHTTP(rr, req)

	want1 := `127.0.0.1 - Frank [03/02/2013:07:54:00 +0000] "GET /testing HTTP/1.1" 200 17` + "\n"
	if buf.String() != want1 {
		t.Errorf("wrong log line: got %v expect %v", buf.String(), want1)
	}

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("wrong status code: got %v expect %v",
			status, http.StatusOK)
	}

	expected := `{"testing": true}`
	if rr.Body.String() != expected {
		t.Errorf("wrong body: got %v expect %v",
			rr.Body.String(), expected)
	}
}

func TestLoggingMiddlewareCombined(t *testing.T) {
	req, err := http.NewRequest("GET", "/testing", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	buf := new(bytes.Buffer)
	tm, err := time.Parse("Jan 2, 2006 at 3:04pm (MST)", "Feb 3, 2013 at 7:54pm (PST)")
	if err != nil {
		t.Errorf("parse time error: %v", err)
	}
	aLog := ApacheCombinedLog(WithOutput(buf), withTime(tm))
	handler := aLog(http.HandlerFunc(HandlerTesting))
	req.Header.Set("referer", "http://localhost/test")
	req.Header.Set("user-agent", "Go testing")

	handler.ServeHTTP(rr, req)

	want1 := `127.0.0.1 - - [03/02/2013:07:54:00 +0000] "GET /testing HTTP/1.1" 200 17 "http://localhost/test" "Go testing"` + "\n"
	if buf.String() != want1 {
		t.Errorf("wrong log line: got %v expect %v", buf.String(), want1)
	}

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("wrong status code: got %v expect %v",
			status, http.StatusOK)
	}

	expected := `{"testing": true}`
	if rr.Body.String() != expected {
		t.Errorf("wrong body: got %v expect %v",
			rr.Body.String(), expected)
	}
}

func TestLoggingMiddlewareCustom(t *testing.T) {
	req, err := http.NewRequest("GET", "/testing", nil)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	buf := new(bytes.Buffer)
	tm, err := time.Parse("Jan 2, 2006 at 3:04pm (MST)", "Feb 3, 2013 at 7:54pm (PST)")
	if err != nil {
		t.Errorf("parse time error: %v", err)
	}
	aLog := Format("[%{%s %r}t] %b", WithOutput(buf), withTime(tm))
	handler := aLog(http.HandlerFunc(HandlerTesting))
	req.Header.Set("referer", "http://localhost/test")
	req.Header.Set("user-agent", "Go testing")
	handler.ServeHTTP(rr, req)

	want1 := `[1359921240 07:54:00 PM] 17` + "\n"
	if buf.String() != want1 {
		t.Errorf("wrong log line: got %v expect %v", buf.String(), want1)
	}

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("wrong status code: got %v expect %v",
			status, http.StatusOK)
	}

	expected := `{"testing": true}`
	if rr.Body.String() != expected {
		t.Errorf("wrong body: got %v expect %v",
			rr.Body.String(), expected)
	}
}

func BenchmarkServeNone(b *testing.B) {
	b.ReportAllocs()

	req, _ := http.NewRequest("GET", "/testing", nil)
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(HandlerTesting)
	req.Header.Set("referer", "http://localhost/test")
	req.Header.Set("user-agent", "Go testing")

	for i := 0; i < b.N; i++ {
		handler.ServeHTTP(rr, req)
	}
}

func BenchmarkServe(b *testing.B) {
	b.ReportAllocs()

	req, _ := http.NewRequest("GET", "/testing", nil)
	rr := httptest.NewRecorder()
	buf := new(bytes.Buffer)
	tm, _ := time.Parse("Jan 2, 2006 at 3:04pm (MST)", "Feb 3, 2013 at 7:54pm (PST)")
	aLog := Format("[%{%s %r}t] %b %D", WithOutput(buf), withTime(tm))
	handler := aLog(http.HandlerFunc(HandlerTesting))
	req.Header.Set("referer", "http://localhost/test")
	req.Header.Set("user-agent", "Go testing")
	for i := 0; i < b.N; i++ {
		handler.ServeHTTP(rr, req)
	}
}

func BenchmarkServeRebuild(b *testing.B) {
	b.ReportAllocs()

	req, _ := http.NewRequest("GET", "/testing", nil)
	rr := httptest.NewRecorder()
	buf := new(bytes.Buffer)
	tm, _ := time.Parse("Jan 2, 2006 at 3:04pm (MST)", "Feb 3, 2013 at 7:54pm (PST)")
	aLog := Format(ApacheCombinedLogFormat, WithOutput(buf), withTime(tm))
	handler := aLog(http.HandlerFunc(HandlerTesting))
	req.Header.Set("referer", "http://localhost/test")
	req.Header.Set("user-agent", "Go testing")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		handler.ServeHTTP(rr, req)
	}
}

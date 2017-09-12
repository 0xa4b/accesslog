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
	// A very simple health check.
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")

	// In the future we could report back on the status of our DB, or our cache
	// (e.g. Redis) by performing a simple PING, and include them in the response.
	io.WriteString(w, `{"testing": true}`)
}

func TestLoggingMiddleware(t *testing.T) {
	req, err := http.NewRequest("GET", "/testing", nil)
	if err != nil {
		t.Fatal(err)
	}

	// We create a ResponseRecorder (which satisfies http.ResponseWriter) to record the response.
	rr := httptest.NewRecorder()
	buf := new(bytes.Buffer)
	tm, err := time.Parse("Jan 2, 2006 at 3:04pm (MST)", "Feb 3, 2013 at 7:54pm (PST)")
	if err != nil {
		t.Errorf("parse time error: %v", err)
	}
	aLog := Format(ApacheCommonLogFormat, WithOutput(buf), withTime(tm))
	handler := aLog(http.HandlerFunc(HandlerTesting))

	// Our handlers satisfy http.Handler, so we can call their ServeHTTP method
	// directly and pass in our Request and ResponseRecorder.
	handler.ServeHTTP(rr, req)

	// Check the status code is what we expect.
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("wrong status code: got %v expect %v",
			status, http.StatusOK)
	}

	// Check the response body is what we expect.
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
	// We create a ResponseRecorder (which satisfies http.ResponseWriter) to record the response.
	rr := httptest.NewRecorder()
	buf := new(bytes.Buffer)
	tm, err := time.Parse("Jan 2, 2006 at 3:04pm (MST)", "Feb 3, 2013 at 7:54pm (PST)")
	if err != nil {
		t.Errorf("parse time error: %v", err)
	}
	aLog := Format(ApacheCommonLogFormat, WithOutput(buf), withTime(tm))
	handler := aLog(http.HandlerFunc(HandlerTesting))

	// Our handlers satisfy http.Handler, so we can call their ServeHTTP method
	// directly and pass in our Request and ResponseRecorder.
	handler.ServeHTTP(rr, req)

	want1 := `127.0.0.1 - Frank [03/02/2013:07:54:00 +0000] "GET /testing HTTP/1.1" 200 17` + "\n"
	if buf.String() != want1 {
		t.Errorf("wrong log line: got %v expect %v", buf.String(), want1)
	}

	// Check the status code is what we expect.
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("wrong status code: got %v expect %v",
			status, http.StatusOK)
	}

	// Check the response body is what we expect.
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

	// We create a ResponseRecorder (which satisfies http.ResponseWriter) to record the response.
	rr := httptest.NewRecorder()
	buf := new(bytes.Buffer)
	tm, err := time.Parse("Jan 2, 2006 at 3:04pm (MST)", "Feb 3, 2013 at 7:54pm (PST)")
	if err != nil {
		t.Errorf("parse time error: %v", err)
	}
	aLog := Format(ApacheCombinedLogFormat, WithOutput(buf), withTime(tm))
	handler := aLog(http.HandlerFunc(HandlerTesting))
	req.Header.Set("referer", "http://localhost/test")
	req.Header.Set("user-agent", "Go testing")
	// Our handlers satisfy http.Handler, so we can call their ServeHTTP method
	// directly and pass in our Request and ResponseRecorder.
	handler.ServeHTTP(rr, req)

	want1 := `127.0.0.1 - - [03/02/2013:07:54:00 +0000] "GET /testing HTTP/1.1" 200 17 "http://localhost/test" "Go testing"` + "\n"
	if buf.String() != want1 {
		t.Errorf("wrong log line: got %v expect %v", buf.String(), want1)
	}

	// Check the status code is what we expect.
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("wrong status code: got %v expect %v",
			status, http.StatusOK)
	}

	// Check the response body is what we expect.
	expected := `{"testing": true}`
	if rr.Body.String() != expected {
		t.Errorf("wrong body: got %v expect %v",
			rr.Body.String(), expected)
	}
}

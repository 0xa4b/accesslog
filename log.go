package accesslog

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// optFunc is a type of function that can add options to the option struct during initialization
type optFunc func(*opt)

// opt is the struct that holds the options for logging.
type opt struct {
	Out  io.Writer
	Time time.Time
}

// newOpt returns a new struct to hold options, with the default output to stdout.
func newOpt() *opt {
	o := new(opt)
	o.Out = os.Stdout
	return o
}

// WithOutput sets the io.Writer output for the log file.
func WithOutput(out io.Writer) optFunc {
	return func(o *opt) {
		o.Out = out
	}
}

// logging is the internal struct that will hold the status and number of bytes written
type logging struct {
	http.ResponseWriter
	status int
	wLen   int
}

// WriteHeader intercepts the http.ResponseWriter WriteHeader method so we can save the status to display later
func (l *logging) WriteHeader(i int) {
	if l.status == 0 {
		l.status = i
	}
	l.ResponseWriter.WriteHeader(i)
}

// Write intercepts the http.ResponseWriter Write method so we can capture the bytes written
func (l *logging) Write(p []byte) (n int, err error) {
	if l.status == 0 {
		l.status = http.StatusOK
	}
	n, err = l.ResponseWriter.Write(p)
	l.wLen += n
	return
}

const (
	ApacheCommonLogFormat   = "%h %l %u %t \"%r\" %>s %b"                                    // The Common Log directives
	ApacheCombinedLogFormat = "%h %l %u %t \"%r\" %>s %b \"%{Referer}i\" \"%{User-agent}i\"" // The Combined Log directives
)

// ApacheCommonLog will log HTTP requests using the Apache Common Log format
var ApacheCommonLog = Log(ApacheCommonLogFormat)

// ApacheCombinedLog will log HTTP requests using the Apache Combined Log format
var ApacheCombinedLog = Log(ApacheCombinedLogFormat)

// Log accepts a format using Apache formatting directives with option functions and returns a function that can handle standard HTTP middleware.
func Log(format string, opts ...optFunc) func(http.Handler) http.Handler {
	options := newOpt()
	for _, opt := range opts {
		opt(options)
	}

	add := func(logVals []interface{}, indexes []int, v interface{}) {
		for _, i := range indexes {
			logVals[i] = v
		}
	}

	var formatStr []string
	var formatVals = make(map[rune][]int)
	var headerVals = make(map[int]string)

	var headerKey string
	var isFmtDirective bool
	var fmtDirectiveIdx int

	var buf = new(bytes.Buffer)
	for _, r := range format {
		if !isFmtDirective && r == '%' {
			isFmtDirective = true
			if buf.Len() > 0 {
				formatStr = append(formatStr, buf.String())
				buf.Reset()
			}
			continue
		}
		if isFmtDirective {
			switch r {
			case '>':
				continue // just skip...
			case '{':
				headerKey = ""
				if buf.Len() > 0 {
					formatStr = append(formatStr, buf.String())
					buf.Reset()
				}
				isFmtDirective = false
				continue
			case '%':
				formatStr = append(formatStr, "%%")
				fmtDirectiveIdx -= 1 // because there is no value
			case 'i', 't':
				if len(headerKey) > 0 {
					headerVals[fmtDirectiveIdx] = headerKey
				}
				fallthrough
			case 'h', 'l', 'u', 'r':
				formatStr = append(formatStr, "%s")
				if _, ok := formatVals[r]; ok {
					formatVals[r] = append(formatVals[r], fmtDirectiveIdx)
				} else {
					formatVals[r] = []int{fmtDirectiveIdx}
				}
			case 'b', 's':
				formatStr = append(formatStr, "%d")
				if _, ok := formatVals[r]; ok {
					formatVals[r] = append(formatVals[r], fmtDirectiveIdx)
				} else {
					formatVals[r] = []int{fmtDirectiveIdx}
				}
			}
			fmtDirectiveIdx++
			isFmtDirective = false
			continue
		}
		switch r {
		case '}':
			if buf.Len() > 0 {
				headerKey = buf.String()
				isFmtDirective = true
				buf.Reset()
			}
			continue
		}
		buf.WriteRune(r)
	}
	if buf.Len() > 0 {
		formatStr = append(formatStr, buf.String())
	}
	buf.Reset()

	logStr := strings.Join(formatStr, "")
	logFunc := func(w *logging, r *http.Request) string {

		logVals := make([]interface{}, fmtDirectiveIdx)
		for k, v := range formatVals {
			switch k {
			case 'h':
				host := "127.0.0.1"
				if r.URL != nil && len(r.URL.Host) > 0 {
					host = r.URL.Host
				}
				add(logVals, v, host)
			case 'l':
				add(logVals, v, "-")
			case 'u':
				un := "-"
				s := strings.SplitN(r.Header.Get("Authorization"), " ", 2)
				if len(s) == 2 {
					b, err := base64.StdEncoding.DecodeString(s[1])
					if err == nil {
						pair := strings.SplitN(string(b), ":", 2)
						if len(pair) == 2 {
							un = pair[0]
						}
					}
				}
				add(logVals, v, un)
			case 't':
				t := time.Now()
				if !options.Time.IsZero() {
					t = options.Time
				}
				add(logVals, v, t.Format("[02/01/2006:03:04:05 -0700]"))
			case 'r':
				add(logVals, v, strings.ToUpper(r.Method)+" "+r.URL.Path+" "+r.Proto)
			case 's':
				add(logVals, v, w.status)
			case 'b':
				add(logVals, v, w.wLen)
			case 'i':
				for _, i := range v {
					logVals[i] = r.Header.Get(headerVals[i])
				}
			}
		}
		return fmt.Sprintf(logStr, logVals...)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			l := &logging{ResponseWriter: w, status: 0, wLen: 0}
			next.ServeHTTP(l, r)
			fmt.Fprintln(options.Out, logFunc(l, r))
		})
	}
}

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

type optFunc func(*opt)

type opt struct {
	Out  io.Writer
	Time time.Time
}

func newOpt() *opt {
	o := new(opt)
	o.Out = os.Stdout
	return o
}

func WithOutput(out io.Writer) optFunc {
	return func(o *opt) {
		o.Out = out
	}
}

func withTime(t time.Time) optFunc {
	return func(o *opt) {
		o.Time = t
	}
}

type logging struct {
	http.ResponseWriter
	status int
	wLen   int
}

func (l *logging) WriteHeader(i int) {
	if l.status == 0 {
		l.status = i
	}
	l.ResponseWriter.WriteHeader(i)
}

func (l *logging) Write(p []byte) (n int, err error) {
	if l.status == 0 {
		l.status = http.StatusOK
	}
	n, err = l.ResponseWriter.Write(p)
	l.wLen += n
	return
}

const ApacheCommonLogFormat = "%h %l %u %t \"%r\" %>s %b"
const ApacheCombinedLogFormat = "%h %l %u %t \"%r\" %>s %b \"%{Referer}i\" \"%{User-agent}i\""

var ApacheCommonLog = Log(ApacheCommonLogFormat)
var ApacheCombinedLog = Log(ApacheCombinedLogFormat)

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
				if val, ok := formatVals[r]; ok {
					val = append(val, fmtDirectiveIdx)
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

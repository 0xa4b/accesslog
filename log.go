package accesslog

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
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
var ApacheCommonLog = Format(ApacheCommonLogFormat)

// ApacheCombinedLog will log HTTP requests using the Apache Combined Log format
var ApacheCombinedLog = Format(ApacheCombinedLogFormat)

func convertTimeFormat(t time.Time, s string) string {
	m := map[rune]string{
		'a': "Mon",
		'A': "Monday",
		'b': "Jan",
		'B': "January",
		'c': "?",
		'C': "06",
		'd': "02",
		'D': "01/02/06",
		'e': "_2",
		'E': "?",
		'F': "2006-01-02",
		'G': "%d",
		'g': "%s",
		'h': "Jan",
		'H': "15",
		'I': "3",
		'j': "%d",
		'k': "•15",
		'l': "_3",
		'm': "01",
		'M': "04",
		'n': "\n",
		'O': "?",
		'p': "PM",
		'P': "pm",
		'r': "03:04:05 PM",
		'R': "15:04",
		's': "%d",
		'S': "05",
		't': "\t",
		'T': "15:04:05",
		'u': "%d",
		'U': "?",
		'V': "%d",
		'w': "%d",
		'W': "?",
		'x': "?",
		'X': "?",
		'y': "06",
		'Y': "2006",
		'z': "-700",
		'Z': "MST",
		'+': "?",
		'%': "%%",
	}

	var i []interface{}
	var x bool
	buf := new(bytes.Buffer)
	for _, r := range s {
		if r == '%' {
			x = true
			continue
		}
		if x {
			x = false
			if val, ok := m[r]; ok {
				if val == "%s" || val == "%d" {
					switch r {
					case 'G':
						y, _ := t.ISOWeek()
						i = append(i, y)
					case 'g':
						y, _ := t.ISOWeek()
						i = append(i, strconv.Itoa(y)[2:])
					case 'j':
						i = append(i, t.YearDay())
					case 's':
						i = append(i, t.Unix())
					case 'u':
						w := t.Weekday()
						if w == 0 {
							w = 7
						}
						i = append(i, w)
					case 'V':
						_, w := t.ISOWeek()
						i = append(i, w)
					case 'w':
						i = append(i, t.Weekday())
					}
				}
				buf.WriteString(val)
				continue
			}
			buf.WriteRune('%')
		}
		buf.WriteRune(r)
	}
	f := t.Format(buf.String())
	if len(i) > 0 {
		f = fmt.Sprintf(f, i...)
	}
	buf.Reset()
	return f
}

// Format accepts a format using Apache formatting directives with option functions and returns a function that can handle standard HTTP middleware.
func Format(format string, opts ...optFunc) func(http.Handler) http.Handler {
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
	var isFmtDirective, isHeader bool
	var fmtDirectiveIdx int

	var buf = new(bytes.Buffer)
	for _, r := range format {
		if !isHeader && !isFmtDirective && r == '%' {
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
				isHeader = true
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
			isHeader = false
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
				for _, i := range v {
					if _, ok := headerVals[i]; ok {
						logVals[i] = convertTimeFormat(t, headerVals[i])
					} else {
						logVals[i] = t.Format("[02/01/2006:03:04:05 -0700]")
					}
				}
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

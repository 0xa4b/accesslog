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
	"unicode"
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

// alog is the internal struct that will hold the status and number of bytes written
type alog struct {
	http.ResponseWriter

	status int
	wLen   int

	start time.Time
}

// WriteHeader intercepts the http.ResponseWriter WriteHeader method so we can save the status to display later
func (a *alog) WriteHeader(i int) {
	if a.status == 0 {
		a.status = i
	}
	a.ResponseWriter.WriteHeader(i)
}

// Write intercepts the http.ResponseWriter Write method so we can capture the bytes written
func (a *alog) Write(p []byte) (n int, err error) {
	if a.status == 0 {
		a.status = http.StatusOK
	}
	n, err = a.ResponseWriter.Write(p)
	a.wLen += n
	return
}

// startTime sets the start time to calculate the elapsed time for the %D directive
func (a *alog) startTime() {
	a.start = time.Now()
}

const (
	ApacheCommonLogFormat   = "%h %l %u %t \"%r\" %>s %b"                                    // The Common Log directives
	ApacheCombinedLogFormat = "%h %l %u %t \"%r\" %>s %b \"%{Referer}i\" \"%{User-agent}i\"" // The Combined Log directives
)

// ApacheCommonLog will log HTTP requests using the Apache Common Log format
var ApacheCommonLog = Format(ApacheCommonLogFormat)

// ApacheCombinedLog will log HTTP requests using the Apache Combined Log format
var ApacheCombinedLog = Format(ApacheCombinedLogFormat)

// convertTimeFormat converts strftime formatting directives to a go time.Time format
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

// logfmt the type that will hold all of the runtime formating
type logfmt struct {
	ti time.Time
	rq *http.Request
	al *alog

	h, u, t, q, e, ws, wl string
}

func (lf *logfmt) withTime(o *opt) *logfmt {
	if !o.Time.IsZero() {
		lf.ti = o.Time
		return lf
	}
	lf.ti = time.Now()
	return lf
}

func (lf *logfmt) withRequest(r *http.Request) *logfmt {
	lf.rq = r
	return lf
}

func (lf *logfmt) withResponse(a *alog) *logfmt {
	lf.al = a
	return lf
}

func (lf *logfmt) timeFormatted(format string) string {
	if len(lf.t) == 0 {
		lf.t = lf.ti.Format(format)
	}
	return lf.t
}

func (lf *logfmt) remoteHostname() string {
	if len(lf.h) == 0 {
		lf.h = lf.rq.URL.Host
		if len(lf.h) == 0 {
			lf.h = "127.0.0.1"
		}
	}
	return lf.h
}

func (lf *logfmt) username() string {
	if len(lf.u) == 0 {
		lf.u = "-"
		if s := strings.SplitN(lf.rq.Header.Get("Authorization"), " ", 2); len(s) == 2 {
			if b, err := base64.StdEncoding.DecodeString(s[1]); err == nil {
				if pair := strings.SplitN(string(b), ":", 2); len(pair) == 2 {
					lf.u = pair[0]
				}
			}
		}
	}
	return lf.u
}

func (lf *logfmt) requestLine() string {
	if len(lf.q) == 0 {
		lf.q = strings.ToUpper(lf.rq.Method) + " " + lf.rq.URL.Path + " " + lf.rq.Proto
	}
	return lf.q
}

func (lf *logfmt) status() string {
	if len(lf.ws) == 0 {
		lf.ws = strconv.Itoa(lf.al.status)
	}
	return lf.ws
}

func (lf *logfmt) bytesWritten() string {
	if len(lf.wl) == 0 {
		lf.wl = strconv.Itoa(lf.al.wLen)
	}
	return lf.wl
}

func (lf *logfmt) timeElapsed() string {
	if len(lf.e) > 0 {
		lf.e = time.Now().Sub(lf.al.start).String()
	}
	return lf.e
}

// flatten takes two slices and merges them into one
func flatten(o *opt, a, b []string) func(w *alog, r *http.Request) string {
	return func(w *alog, r *http.Request) string {
		lf := new(logfmt)
		lf.withTime(o).withRequest(r).withResponse(w)

		buf := new(bytes.Buffer)
		for i, s := range a {
			switch s {
			case "":
				buf.WriteString(b[i])
			case "%h":
				buf.WriteString(lf.remoteHostname())
			case "%l":
				buf.WriteString("-")
			case "%u":
				buf.WriteString(lf.username())
			case "%t":
				buf.WriteString(lf.timeFormatted("[02/01/2006:03:04:05 -0700]"))
			case "%r":
				buf.WriteString(lf.requestLine())
			case "%s", "%>s":
				buf.WriteString(lf.status())
			case "%b":
				buf.WriteString(lf.bytesWritten())
			case "%D":
				buf.WriteString(lf.timeElapsed())
			default:
				if len(s) > 4 && s[:2] == "%{" && s[len(s)-2] == '}' {
					label := s[2 : len(s)-2]
					switch s[len(s)-1] {
					case 'i':
						buf.WriteString(r.Header.Get(label))
					case 't':
						buf.WriteString(convertTimeFormat(lf.ti, label))
					}
				}
			}
		}
		return buf.String()
	}
}

// Format accepts a format using Apache formatting and returns a function accepting option functions
// which then returns a function that can handle standard HTTP middleware. This is to have better standard
// logging functions accessible from the library
func Format(format string) func(...optFunc) func(http.Handler) http.Handler {
	return func(opts ...optFunc) func(http.Handler) http.Handler {
		return FormatWith(format, opts...)
	}
}

// FormatWith accepts a format using Apache formatting directives with option functions and returns a function that can handle standard HTTP middleware.
func FormatWith(format string, opts ...optFunc) func(http.Handler) http.Handler {
	options := newOpt()
	for _, opt := range opts {
		opt(options)
	}

	var directives, betweens = make([]string, 0, 50), make([]string, 0, 50)
	var mBuf *bytes.Buffer
	aBuf, bBuf := new(bytes.Buffer), new(bytes.Buffer)
	mBuf = bBuf

	var isDirective, isEnclosure bool
	for i, r := range format {
		switch r {
		case '%':
			if isDirective {
				mBuf.WriteRune(r)
				continue
			}
			isDirective = true
			if i != 0 {
				directives = append(directives, aBuf.String())
				betweens = append(betweens, bBuf.String())
				aBuf.Reset()
				bBuf.Reset()
			}
			mBuf = aBuf
		case '{':
			isEnclosure = true
		case '}':
			isEnclosure = false
		case '>':
			// do the same thing
		default:
			if isDirective && !isEnclosure && !unicode.IsLetter(r) {
				isDirective = false
				isEnclosure = false
				if i != 0 {
					directives = append(directives, aBuf.String())
					betweens = append(betweens, bBuf.String())
					aBuf.Reset()
					bBuf.Reset()
				}
				mBuf = bBuf
			}
		}
		mBuf.WriteRune(r)
	}

	directives = append(directives, aBuf.String())
	betweens = append(betweens, bBuf.String())
	aBuf.Reset()
	bBuf.Reset()

	logFunc := flatten(options, directives, betweens)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			a := &alog{ResponseWriter: w}
			a.startTime()
			next.ServeHTTP(a, r)
			fmt.Fprintln(options.Out, logFunc(a, r))
		})
	}
}

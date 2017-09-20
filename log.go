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

// optFunc is the type to use to options to the option struct during initialization
type optFunc func(*opt)

// opt is the internal struct that holds the options for logging.
type opt struct {
	Output io.Writer
	Time   time.Time
}

// newOpt returns a new struct to hold options, with the default output to stdout.
func newOpt() *opt {
	o := new(opt)
	o.Output = os.Stdout
	return o
}

// WithOutput sets the io.Writer output for the log file.
func WithOutput(out io.Writer) optFunc {
	return func(o *opt) {
		o.Output = out
	}
}

// responseWriter is the internal struct that will wrap the http.ResponseWriter
// and hold the status and number of bytes written
type responseWriter struct {
	http.ResponseWriter

	status    int
	byteCount int

	start time.Time
}

// WriteHeader intercepts the http.ResponseWriter WriteHeader method so we can save the status to display later
func (rw *responseWriter) WriteHeader(i int) {
	if rw.status == 0 {
		rw.status = i
	}
	rw.ResponseWriter.WriteHeader(i)
}

// Write intercepts the http.ResponseWriter Write method so we can capture the bytes written
func (rw *responseWriter) Write(p []byte) (n int, err error) {
	if rw.status == 0 {
		rw.status = http.StatusOK
	}
	n, err = rw.ResponseWriter.Write(p)
	rw.byteCount += n
	return
}

// startTime sets the start time to calculate the elapsed time for the %D directive
func (rw *responseWriter) startTime() {
	rw.start = time.Now()
}

const (
	// ApacheCommonLogFormat is the Apache Common Log directives
	ApacheCommonLogFormat = "%h %l %u %t \"%r\" %>s %b"

	// ApacheCombinedLogFormat is the Apache Combined Log directives
	ApacheCombinedLogFormat = "%h %l %u %t \"%r\" %>s %b \"%{Referer}i\" \"%{User-agent}i\""
)

// ApacheCommonLog will log HTTP requests using the Apache Common Log format
var ApacheCommonLog = Format(ApacheCommonLogFormat)

// ApacheCombinedLog will log HTTP requests using the Apache Combined Log format
var ApacheCombinedLog = Format(ApacheCombinedLogFormat)

var timeFmtMap = map[rune]string{
	'a': "Mon", 'A': "Monday", 'b': "Jan", 'B': "January", 'C': "06",
	'd': "02", 'D': "01/02/06", 'e': "_2", 'F': "2006-01-02",
	'h': "Jan", 'H': "15", 'I': "3", 'k': "•15", 'l': "_3",
	'm': "01", 'M': "04", 'n': "\n", 'p': "PM", 'P': "pm",
	'r': "03:04:05 PM", 'R': "15:04", 'S': "05",
	't': "\t", 'T': "15:04:05", 'y': "06", 'Y': "2006",
	'z': "-700", 'Z': "MST", '%': "%%",

	// require calculated time
	'G': "%v", 'g': "%v", 'j': "%v", 's': "%v",
	'u': "%v", 'V': "%v", 'w': "%v",

	// Unsupported directives
	'c': "?", 'E': "?", 'O': "?", 'U': "?",
	'W': "?", 'x': "?", 'X': "?", '+': "?",
}

// convertTimeFormat converts strftime formatting directives to a go time.Time format
func convertTimeFormat(now time.Time, format string) string {
	var isDirective bool
	var calcTime []int64
	var buf = new(bytes.Buffer)
	for _, r := range format {
		if !isDirective && r == '%' {
			isDirective = true
			continue
		}
		if !isDirective {
			buf.WriteRune(r)
			continue
		}
		if val, ok := timeFmtMap[r]; ok {
			if val == "%v" {
				switch r {
				case 'G':
					y, _ := now.ISOWeek()
					calcTime = append(calcTime, int64(y))
				case 'g':
					y, _ := now.ISOWeek()
					y -= (y / 100) * 100
					calcTime = append(calcTime, int64(y))
					buf.WriteString("%02d") // we need to pad the number
					isDirective = false
					continue
				case 'j':
					calcTime = append(calcTime, int64(now.YearDay()))
				case 's':
					calcTime = append(calcTime, now.Unix())
				case 'u':
					w := now.Weekday()
					if w == 0 {
						w = 7
					}
					calcTime = append(calcTime, int64(w))
				case 'V':
					_, w := now.ISOWeek()
					calcTime = append(calcTime, int64(w))
				case 'w':
					calcTime = append(calcTime, int64(now.Weekday()))
				}
			}
			buf.WriteString(val)
			isDirective = false
			continue
		}
		buf.WriteString("(%" + string(r) + " is invalid)")
	}
	s := now.Format(buf.String())
	if len(calcTime) > 0 {
		ctInter := make([]interface{}, len(calcTime))
		for i := range calcTime {
			ctInter[i] = calcTime[i]
		}
		s = fmt.Sprintf(s, ctInter...)
	}
	buf.Reset()
	return s
}

// line is the type that will hold all of the runtime formating directives for the log line
type line struct {
	time    time.Time
	request *http.Request
	writer  *responseWriter

	// directives
	h, u, t, r, s, b, D string
}

func (ln *line) withTime(o *opt) *line {
	if !o.Time.IsZero() {
		ln.time = o.Time
		return ln
	}
	ln.time = time.Now()
	return ln
}

func (ln *line) withRequest(r *http.Request) *line {
	ln.request = r
	return ln
}

func (ln *line) withResponse(a *responseWriter) *line {
	ln.writer = a
	return ln
}

// remoteHostname - %h
func (ln *line) remoteHostname() string {
	if len(ln.h) == 0 {
		ln.h = ln.request.URL.Host
		if len(ln.h) == 0 {
			ln.h = "127.0.0.1"
		}
	}
	return ln.h
}

// username - %u
func (ln *line) username() string {
	if len(ln.u) == 0 {
		ln.u = "-"
		if s := strings.SplitN(ln.request.Header.Get("Authorization"), " ", 2); len(s) == 2 {
			if b, err := base64.StdEncoding.DecodeString(s[1]); err == nil {
				if pair := strings.SplitN(string(b), ":", 2); len(pair) == 2 {
					ln.u = pair[0]
				}
			}
		}
	}
	return ln.u
}

//timeFormatted - %t
func (ln *line) timeFormatted(format string) string {
	if len(ln.t) == 0 {
		ln.t = ln.time.Format(format)
	}
	return ln.t
}

// requestLine - %r
func (ln *line) requestLine() string {
	if len(ln.r) == 0 {
		ln.r = strings.ToUpper(ln.request.Method) + " " + ln.request.URL.Path + " " + ln.request.Proto
	}
	return ln.r
}

// status - %s
func (ln *line) status() string {
	if len(ln.s) == 0 {
		ln.s = strconv.Itoa(ln.writer.status)
	}
	return ln.s
}

// bytesWritten - %b
func (ln *line) bytesWritten() string {
	if len(ln.b) == 0 {
		ln.b = strconv.Itoa(ln.writer.byteCount)
	}
	return ln.b
}

// timeElapsed - %D
func (ln *line) timeElapsed() string {
	if len(ln.D) > 0 {
		ln.D = time.Now().Sub(ln.writer.start).String()
	}
	return ln.D
}

// flatten takes two slices and merges them into one
func flatten(o *opt, a, b []string) func(w *responseWriter, r *http.Request) string {
	return func(w *responseWriter, r *http.Request) string {
		ln := new(line)
		ln.withTime(o).withRequest(r).withResponse(w)

		buf := new(bytes.Buffer)
		for i, s := range a {
			switch s {
			case "":
				buf.WriteString(b[i])
			case "%h":
				buf.WriteString(ln.remoteHostname())
			case "%l":
				buf.WriteString("-")
			case "%u":
				buf.WriteString(ln.username())
			case "%t":
				buf.WriteString(ln.timeFormatted("[02/01/2006:03:04:05 -0700]"))
			case "%r":
				buf.WriteString(ln.requestLine())
			case "%s", "%>s":
				buf.WriteString(ln.status())
			case "%b":
				buf.WriteString(ln.bytesWritten())
			case "%D":
				buf.WriteString(ln.timeElapsed())
			default:
				if len(s) > 4 && s[:2] == "%{" && s[len(s)-2] == '}' {
					label := s[2 : len(s)-2]
					switch s[len(s)-1] {
					case 'i':
						buf.WriteString(r.Header.Get(label))
					case 't':
						buf.WriteString(convertTimeFormat(ln.time, label))
					}
				}
			}
		}
		return buf.String()
	}
}

// Format accepts a format string using Apache formatting directives and returns
// a function accepting internal option functions which then returns
// a function that can handle standard HTTP middleware.
// This function more convenient to use when saving formatting to
// a variable, then using with standard HTTP middleware
func Format(format string) func(...optFunc) func(http.Handler) http.Handler {
	return func(opts ...optFunc) func(http.Handler) http.Handler {
		return FormatWith(format, opts...)
	}
}

// FormatWith accepts a format string using Apache formatting directives with
// option functions and returns a function that can handle standard HTTP middleware.
func FormatWith(format string, opts ...optFunc) func(http.Handler) http.Handler {
	options := newOpt()
	for _, opt := range opts {
		opt(options)
	}

	var directives, betweens = make([]string, 0, 50), make([]string, 0, 50)
	var cBuf *bytes.Buffer // current buffer
	aBuf, bBuf := new(bytes.Buffer), new(bytes.Buffer)
	cBuf = bBuf

	var isDirective, isEnclosure bool
	for i, r := range format {
		switch r {
		case '%':
			if isDirective {
				cBuf.WriteRune(r)
				continue
			}
			isDirective = true
			if i != 0 {
				directives = append(directives, aBuf.String())
				betweens = append(betweens, bBuf.String())
				aBuf.Reset()
				bBuf.Reset()
			}
			cBuf = aBuf
		case '{':
			isEnclosure = true
		case '}':
			isEnclosure = false
		case '>':
			// nothing - no change in status
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
				cBuf = bBuf
			}
		}
		cBuf.WriteRune(r)
	}

	directives = append(directives, aBuf.String())
	betweens = append(betweens, bBuf.String())
	aBuf.Reset()
	bBuf.Reset()

	logFunc := flatten(options, directives, betweens)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rw := &responseWriter{ResponseWriter: w}
			rw.startTime()
			next.ServeHTTP(rw, r)
			fmt.Fprintln(options.Output, logFunc(rw, r))
		})
	}
}

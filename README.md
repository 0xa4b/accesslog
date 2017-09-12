[![GoDoc](https://godoc.org/github.com/xa4b/accesslog?status.svg)](https://godoc.org/github.com/0xa4b/accesslog)
[![Go Report Card](https://goreportcard.com/badge/0xa4b/accesslog)](https://goreportcard.com/report/0xa4b/accesslog)

# AccessLog    

Provides simple middleware logging that conforms to Apache Common Log and Apache Combined Log formats. It can also have custom formats using the Apache Log Directives.

## Installation

    go get github.com/0xa4b/accesslog

## Example

    import (
        "fmt"
        "log"
        "net/http"
        "os"

        "github.com/0xa4b/accesslog"
    )

    func heartBeatHandler(w http.ResponseWriter, r *http.Request) {
        fmt.Fprintln(w, "beat")
    }

    func homeHandler(w http.ResponseWriter, r *http.Request) {
        fmt.Fprintln(w, "you are home")
    }

    func main(){

        f, err := os.Create("access.log")
        if err != nil {
            log.Fatalf("access log create error: %v", err)
        }
        defer f.Close()

        // log to stdout
        http.Handle("/heart-beat", accesslog.ApacheCommonLogWith()(heartBeatHandler))
        
        // log to access file
        http.Handle("/home", accesslog.ApacheCommonLogWith(f)(homeHandler))

        log.Fatal(http.ListenAndServe(":8080", nil))
    }

## License

AccessLog is available under the [MIT License](https://opensource.org/licenses/MIT).

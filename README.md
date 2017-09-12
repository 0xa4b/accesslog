[![GoDoc](https://godoc.org/github.com/xa4b/accesslog?status.svg)](https://godoc.org/github.com/0xa4b/accesslog)
[![Go Report Card](https://goreportcard.com/badge/xa4b/accesslog)](https://goreportcard.com/report/0xa4b/accesslog)

# AccessLog    

Provides simple middleware logging that conforms to Apache Common Log and Apache Combined Log formats. It can also have custom formats using the Apache Log Directives.

## Installation

    go get github.com/0xa4b/accesslog

## Example

    import "github.com/0xa4b/accesslog"

    func main(){
        http.HandleFunc("/heart-beat", accesslog.ApacheCommonLog(heartBeatHandler))

        log.Fatal(http.ListenAndServe(":8080", nil))
    }

## License

AccessLog is available under the [MIT License](https://opensource.org/licenses/MIT).

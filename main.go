package main

import (
    "fmt"
    "log"
    "net"
    "net/http"
    "net/url"
    "io/ioutil"
    "strings"

    "github.com/gorilla/mux"

    //"conversion/conversion"
)

const MAX_SIZE = 99999999999

/*
    Codes:
    0 - OK
    1 - Timeout
    2 - Error contacting
    3 - Request too large
    4 - Internal error
*/
type Validation struct {
    Valid   bool
    Code    int
}

func main() {
    //Create router for API
    router := mux.NewRouter().StrictSlash(true)
    router.HandleFunc("/capture", CaptureHandler)

    //Listen on port for requests
    fmt.Println("Listening on port 8080...")
    log.Fatal(http.ListenAndServe(":8080", router))
}

//Handles requests to the capture route
func CaptureHandler(w http.ResponseWriter, r *http.Request) {
    //Get URL from request
    reqURL, err := url.Parse(r.URL.String())
    if err != nil {
        //502
        w.WriteHeader(http.StatusBadGateway)
        w.Write([]byte("Capture page not found."))
        return
    }
    //Get query string parameters from URL
    query := reqURL.Query()
    fmt.Printf("Full URL from request: %q\n", reqURL)
    if query.Get("url") == "" {
        //502
        w.WriteHeader(http.StatusBadGateway)
        w.Write([]byte("Capture page not found in querystring."))
        return
    }

    //Get html string from specified page
    requestedURL := query.Get("url")
    selector := query.Get("dynamic_size_selector")
    fmt.Printf("Requested URL: %s\n", requestedURL)
    fmt.Printf("Requested Selector: %s\n", selector)
    pageHTML, validation := getPageHTML(requestedURL, selector)

    if validation.Valid != true {
        fmt.Printf("Invalid: %s\n", validation)
        switch validation.Code {
        case 1: //504 - timeout
            w.WriteHeader(http.StatusGatewayTimeout)
            w.Write([]byte("Capture site did not load in time"))
        case 2: //503 - unable to contact
            w.WriteHeader(http.StatusServiceUnavailable)
            w.Write([]byte("Capture site could not be contacted"))
        case 3: //400 - Request too large
            w.WriteHeader(http.StatusBadRequest)
            w.Write([]byte("Request is too large"))
        default: //500 - internal error
            w.WriteHeader(http.StatusInternalServerError)
            w.Write([]byte("Internal server error"))
        }
        return
    }

    fmt.Printf("Length of pageHTML after get function: %v\n\n", len(pageHTML))
}

/*
    Takes a url and optional selector
*/
func getPageHTML(url, selector string) (html []byte, validation Validation) {
    //Set defaults
    validation.Valid = false
    html = []byte("")
    //Send request to page
    resp, err := http.Get(url)
	//Handle the error if there is one
	if err != nil {
        if err, ok := err.(net.Error); ok && err.Timeout() {
            //504
            validation.Code = 1
        } else {
            //503
            validation.Code = 2
        }
        return
	}
	//Do this now so it won't be forgotten
	defer resp.Body.Close()

    //Read returned HTML
    html, err = ioutil.ReadAll(resp.Body)
    if err != nil {
        //Error reading returned HTML is 500 error
        validation.Code = 4
        return
    }

    //Size check
    if len(html) > MAX_SIZE {
        validation.Code = 3
        return
    }

    if selector != "" {
        selections := strings.Fields(selector)

        //TODO: Account for taking nth element; ie, ul4 gets the 4th unordered list
        for _, elem := range selections {
            start := 0
            end := len(html)

            //Look for opening of CSS element
            idx := strings.Index(string(html), "<" + elem)
            fmt.Printf("Strings start index for %s: %v\n", elem, idx)

            //Set start to index location
            if idx != -1 {
                fmt.Printf("Found at %v.\n First 50 chars from index: %s\n", idx, html[idx:idx+50])
                start = idx
            } else {
                //Selector not found, skipping selection
                validation.Code = 5
                break
            }
            //Find closing tag to match
            idx = strings.Index(string(html), "</" + elem)
            fmt.Printf("Strings end for %s: %v\n", elem, idx)
            //Set start to index location
            if idx != -1 {
                fmt.Printf("Found at %v.\n First 50 chars from index: %s\n", idx, html[idx:idx+50])
                end = idx + 3 + len(elem) //length of </{elem}>
            } else {
                //Selector end not found, skipping selection
                validation.Code = 5
                break
            }
            html = html[start:end]
            // fmt.Printf("HTML after loop of selections: %s\n", html)
        }
    }

    validation.Valid = true
    validation.Code = 0
    return
}

package main

import (
	"fmt"
	"image/png"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/gorilla/mux"
)

const MAX_SIZE = 75000

/*
   Codes:
       0 - OK
       1 - Timeout
       2 - Error contacting
       3 - Request too large
       4 - Selector not found - still valid
       5 - Internal error
*/
type Validation struct {
	Valid bool
	Code  int
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
		fmt.Printf("Invalid HTML: %v\n", validation)
		switch validation.Code {
		case 1: //504
			w.WriteHeader(http.StatusGatewayTimeout)
			w.Write([]byte("Capture site did not load in time"))
		case 2: //503
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("Capture site could not be contacted"))
		case 3: //400
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Request is too large"))
		default: //500
			InternalServerErrorWriter(w)
			// w.WriteHeader(http.StatusInternalServerError)
			// w.Write([]byte("Internal server error"))
		}
		return
	}

	if validation.Code == 4 {
		//205
		w.WriteHeader(http.StatusResetContent)
	}
	fmt.Printf("Length of pageHTML after get function: %v\n\n", len(pageHTML))

	//Convert image
	converter, err := ConvertImage(pageHTML)
	defer CleanUp(converter)

	if err != nil {
		//500
		InternalServerErrorWriter(w)
		// w.WriteHeader(http.StatusInternalServerError)
		// w.Write([]byte("Internal server error"))
		return
	}

	//Write png to http response
	file, err := os.Open(converter.OutFilePattern)
	if err != nil {
		//500
		InternalServerErrorWriter(w)
		// fmt.Printf("Error reading temp png file: %q", converter.OutFilePattern)
		// w.WriteHeader(http.StatusInternalServerError)
		// w.Write([]byte("Internal server error"))
		return
	}
	defer file.Close()

	img, err := png.Decode(file)
	if err != nil {
		//500
		InternalServerErrorWriter(w)
		fmt.Printf("Error decoding file to png: %q", converter.OutFilePattern)
		// w.WriteHeader(http.StatusInternalServerError)
		// w.Write([]byte("Internal server error"))
		return
	}

	w.Header().Set("Content-Type", "image/png")
	if err := png.Encode(w, img); err != nil {
		//500
		InternalServerErrorWriter(w)
		fmt.Printf("Error decoding file to png: %q", converter.OutFilePattern)
		// w.WriteHeader(http.StatusInternalServerError)
		// w.Write([]byte("Internal server error"))
		return
	}

	fmt.Printf("Converter.ID in main: %+q", converter.ID)
}

/*
    Function getPageHTML
   Params:
       url - string. URL from which to retrieve html
       selector - String. Element or elements of the html to capture;
       Multiple elements must be located inside previous elements
   Returns:
       html - []byte. portion of the html defined by selector scope or full page html
       validation - Contains error codes and success of failure of gathering html.
   Notes: Can fail based on not contacting the site (unavailable, timout, etc.), and the desired file html being too large. 0 and 5 and non-failing codes. See the Validation struct for full codes.
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
		validation.Code = 5
		return
	}

	//Selector check
	if selector != "" {
		html = applySelector(html, selector, &validation)
	}
	fmt.Printf("Validation after selection: %v\n", validation)

	//Size check
	if len(html) > MAX_SIZE {
		fmt.Println("HTML selection too large for image conversion after selection.")
		validation.Code = 3
		return
	}

	validation.Valid = true
	validation.Code = 0
	fmt.Printf("Validation at return: %v\n", validation)
	return
}

/*
    Function applySelector
   Params:
       html - []byte. HTML byte slice to check for selector elements.
       selector - string. Element or multiple elements to look for.
       validation - *Validation. Updates the Validation code in case selector can't be found.
   Return:
       []byte - slice of the original html if selector found, or simply the original html if not.
   Notes:
       If multiple selectors are present (separated by spaces in the string),
       it will pair down the html byte slice by each selector found in order
       until it fails to find one and returns the current slice.
*/
func applySelector(html []byte, selector string, validation *Validation) []byte {
	selections := strings.Fields(selector)

	//TODO: Account for taking nth element; ie, ul4 gets the 4th unordered list
	for _, elem := range selections {
		start := 0
		end := len(html)

		//Look for opening of CSS element
		idx := strings.Index(string(html), "<"+elem)

		//Set start to index location
		if idx != -1 {
			start = idx
		} else {
			//Selector not found, skipping selection from this element on
			validation.Code = 4
			break
		}

		//Find closing tag to match
		idx = strings.Index(string(html), "</"+elem)

		//Set start to index location
		if idx != -1 {
			end = idx + 3 + len(elem) //length of </{elem}>
		} else {
			//Selector end not found, skipping selection from this element on
			validation.Code = 4
			break
		}
		html = html[start:end]
	}
	return html
}

//Writes an internal server error to the response writer
func InternalServerErrorWriter(w http.ResponseWriter) {
	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte("Internal server error"))
}

package main

import (
	"bytes"
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

const HTTP_MAX_SIZE = 75000

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
	router.HandleFunc("/capture", captureHandler).Methods("GET")

	//Listen on port for requests
	fmt.Println("Listening on port 8080...")
	log.Fatal(http.ListenAndServe(":8080", router))
}

//Handles requests to the capture route
func captureHandler(w http.ResponseWriter, r *http.Request) {
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
	if query.Get("url") == "" {
		//502
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("Capture page not found in querystring."))
		return
	}

	//Get html string from specified page
	requestedURL := query.Get("url")
	selector := query.Get("dynamic_size_selector")

	pageHTML, validation := getPageHTML(requestedURL, selector)

	if validation.Valid != true {
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
		case 5: //500
			InternalServerErrorWriter(w)
		}
		return
	}

	//Convert image
	converter, err := ConvertImage(pageHTML)
	defer converter.CleanUp()
	if err != nil {
		//500
		InternalServerErrorWriter(w)
		fmt.Printf("Error converting to png: %q\n", err)
		return
	}

	//Open temp png file from convert
	file, err := os.Open(converter.OutFilePattern)
	if err != nil {
		//500
		InternalServerErrorWriter(w)
		fmt.Printf("Error reading temp png file: %q\n", converter.OutFilePattern)
		return
	}
	defer file.Close()

	//Decode
	img, err := png.Decode(file)
	if err != nil {
		//500
		InternalServerErrorWriter(w)
		fmt.Printf("Error decoding file to png: %q\n", converter.OutFilePattern)
		return
	}

	//Place in buffer for writing to responseWriter
	buffer := new(bytes.Buffer)
	if err := png.Encode(buffer, img); err != nil {
		//500
		InternalServerErrorWriter(w)
		fmt.Printf("Error decoding file to png: %q\n", err)
		return
	}

	//Set final status code
	if validation.Code == 4 {
		//206 - 205's cannot have payloads and will not send an image
		w.WriteHeader(http.StatusPartialContent)
	} else {
		//200
		w.WriteHeader(http.StatusOK)
	}

	//Set content headers
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", fmt.Sprint(len(buffer.Bytes())))

	//Write from buffer
	if _, err := w.Write(buffer.Bytes()); err != nil {
		fmt.Println("Error writing image to response writer.")
	}

	/*
	   Reading from file straight to writer is faster, but larger images are lost
	   	//Open temp png file from convert
	   	fmt.Println("Opening file...")
	   	//Realized I could just read from file and straight to writer
	   	file, err := ioutil.ReadFile(converter.OutFilePattern)
	   	if err != nil {
	   		//500
	   		InternalServerErrorWriter(w)
	   		fmt.Printf("Error reading temp png file: %q", converter.OutFilePattern)
	   		return
	   	}

	   	//Set content headers
	   	w.Header().Set("Content-Type", "image/png")
	   	w.Header().Set("Content-Length", strconv.Itoa(len(file)))
	   	fmt.Printf("Content length set to: %v\n", strconv.Itoa(len(file)))

	   	//Write to response
	   	if _, err := w.Write(file); err != nil {
	   		fmt.Println("Unable to write image.")
	   	}
	*/

	fmt.Printf("Finished job: %q\n\n", converter.ID)
	return
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
   	Notes:
		Can fail based on not contacting the site (unavailable, timout, etc.), and the desired file html being too large. 0 and 5 and non-failing codes. See the Validation struct for full codes.
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

	//Size check
	if len(html) > HTTP_MAX_SIZE {
		fmt.Println("HTML selection too large for image conversion after selection.")
		validation.Code = 3
		return
	}

	validation.Valid = true
	if validation.Code != 4 {
		validation.Code = 0
	}

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

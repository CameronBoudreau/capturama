package main

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"time"
)

const CONVERT_COMMAND = "wkhtmltoimage"

//Can add options such as height/width/quality here in the future if desired.
type Converter struct {
	HTML           []byte
	ID             string
	InFilePattern  string
	OutFilePattern string
}

func ConvertImage(html []byte) (converter Converter, err error) {
	converter = Converter{HTML: html}
	converter.ID = converter.GenerateUniqueTempId()
	fmt.Printf("ConverterID in ConvertImage: %v\n", string(converter.ID))
	err = converter.HtmlToImage()
	return
}

func (c *Converter) HtmlToImage() (err error) {
	fmt.Println("Start of HTMLToImage")
	cmd := CONVERT_COMMAND
	args := []string{}

	//Conversion needs to be from file
	err = ioutil.WriteFile(c.InFilePattern, c.HTML, 0644)
	if err != nil {
		fmt.Printf("Error writing file: %q\n", err)
		return
	}

	args = append(args,
		"--format", "png",
		c.InFilePattern,
		c.OutFilePattern,
	)

	fmt.Printf("args to service: %q\n", args)
	if err := exec.Command(cmd, args...).Run(); err != nil {
		fmt.Printf("Error executing conversion on command line: %v", err)
	}
	fmt.Printf("Image conversion in HTMLToImage complete for %q.\n\n", c.ID)
	return
}

const TEMP_DIR = "tmp/"

//Returns a random number to append to temp files to prevent overwriting.
//Checks against existing temp files
func (c *Converter) GenerateUniqueTempId() string {
	fmt.Printf("Starting Generate ID.\n")
	rand.Seed(time.Now().Unix())
	num := strconv.Itoa(rand.Intn(99999999-10000000) + 10000000)
	fmt.Printf("Trying unique ID: %s.\n", num)

	c.InFilePattern = TEMP_DIR + "input" + num + ".html"
	c.OutFilePattern = TEMP_DIR + "output" + num + ".png"

	//Rerun if number currently in use by either input or output files
	if _, err := os.Stat(c.InFilePattern); err == nil {
		fmt.Printf("Found inFile Pattern exists\n")
		return c.GenerateUniqueTempId()
	} else if _, err := os.Stat(c.OutFilePattern); err == nil {
		fmt.Printf("Found outFile Pattern exists\n")
		return c.GenerateUniqueTempId()
	}

	fmt.Printf("InFile Pattern: %s.\n", c.InFilePattern)
	fmt.Printf("OutFile Pattern: %s.\n", c.OutFilePattern)

	return string(num)
}

//Remove existing temp files from converter run
func (c *Converter) CleanUp() {
	if _, err := os.Stat(c.InFilePattern); err == nil {
		err := os.Remove(c.InFilePattern)
		if err != nil {
			fmt.Printf("Error removing temp file: %v", c.InFilePattern)
		}
	}

	if _, err := os.Stat(c.OutFilePattern); err == nil {
		err = os.Remove(c.OutFilePattern)
		if err != nil {
			fmt.Printf("Error removing temp file: %v", c.OutFilePattern)
		}
	}
}

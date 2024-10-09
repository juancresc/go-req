package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	flag "github.com/spf13/pflag"

	"golang.org/x/time/rate"
)

func main() {
	// Define flags
	rps := flag.Float64("rps", 0, "Requests per second (required)")
	address := flag.String("address", "", "Address to hit (required)")
	authentication := flag.String("authentication", "", "Token required for api authentication")
	// headers is a list of headers to include in the request
	headers := flag.StringSlice("headers", []string{}, "Headers to include in the request")
	flag.Parse()

	// Validate that required flags are provided
	if *rps <= 0 {
		fmt.Println("Error: The 'rps' flag is required and must be greater than 0.")
		flag.Usage()
		os.Exit(1)
	}

	if *address == "" {
		fmt.Println("Error: The 'address' flag is required.")
		flag.Usage()
		os.Exit(1)
	}

	started := time.Now()
	lastPrint := time.Now()
	requests := 0
	limiter := rate.NewLimiter(rate.Limit(*rps), 1)
	successChannel := make(chan int)
	successCount := 0
	errorChannel := make(chan int)
	errorCount := 0

	for {
		select {
		case <-successChannel:
			successCount++
		case <-errorChannel:
			errorCount++
		default:
			if limiter.Allow() {
				go doRequest(*address, *headers, *authentication, successChannel, errorChannel)
				requests++
			}
			elapsed := time.Since(started)
			currentRps := float64(requests) / elapsed.Seconds()
			if time.Since(lastPrint) > 1*time.Second {
				clearScreen()
				log.Printf("Requests: %d, Elapsed time (s): %d, RPS: %s, Success: %d, Error: %d", requests, int(elapsed.Seconds()), fmt.Sprintf("%.2f", currentRps), successCount, errorCount)
			}
		}

	}
}

func clearScreen() {
	switch runtime.GOOS {
	case "linux", "darwin": // Unix-like systems
		cmd := exec.Command("clear")
		cmd.Stdout = os.Stdout
		cmd.Run()
	case "windows": // Windows system
		cmd := exec.Command("cmd", "/c", "cls")
		cmd.Stdout = os.Stdout
		cmd.Run()
	}
}

func doRequest(url string, headers []string, authentication string, successChannel chan int, errorChannel chan int) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("Error creating request: %v", err)
		// count error
		errorChannel <- 1
	}
	for _, header := range headers {
		headerParts := strings.Split(header, ":")
		if len(headerParts) != 2 {
			log.Printf("Invalid header: %s", header)
			continue
		}
		req.Header.Set(headerParts[0], headerParts[1])
	}
	if authentication != "" {
		req.Header.Set("Authentication", fmt.Sprintf("bearer %s", authentication))
	}
	resp, err := client.Do(req)
	io.Copy(io.Discard, resp.Body)
	if err != nil {
		log.Printf("Error making request: %v", err)
		// count error
		errorChannel <- 1
	}
	if resp.StatusCode != 200 {
		log.Printf("Error: Status code %d", resp.StatusCode)
		// count error
		errorChannel <- 1
	}
	// count success
	successChannel <- 1
	defer resp.Body.Close()
}

package main

import (
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"slices"
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
	duration := flag.Duration("duration", 0, "Duration to run the test for in time format (e.g. 1h30m, 10s, 100ms)")
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

	successChannel := make(chan time.Duration, 100)
	successCount := 0
	successDurations := []time.Duration{}

	errorsChannel := make(chan string, 100)
	errorCount := 0
	errors := []string{}

	for {
		select {
		case duration := <-successChannel:
			successDurations = append(successDurations, duration)
			successCount++
		case errMsg := <-errorsChannel:
			errorCount++
			errors = append(errors, errMsg)
		default:
			if limiter.Allow() {
				go doRequest(*address, *headers, *authentication, successChannel, errorsChannel)
				requests++
			}

			elapsed := time.Since(started)
			if time.Since(lastPrint) <= 1*time.Second {
				printMetrics(requests, successCount, errorCount, successDurations, errors, duration, elapsed, false)
				lastPrint = time.Now()
			}
			if *duration > 0 && time.Since(started) > *duration {
				printMetrics(requests, successCount, errorCount, successDurations, errors, duration, elapsed, true)
				os.Exit(0)
			}
		}

	}

}

func printMetrics(requests int, successCount int, errorCount int, successDurations []time.Duration, errors []string, duration *time.Duration, elapsed time.Duration, printTimes bool) {
	currentRps := float64(requests) / elapsed.Seconds()
	// count different errors and show
	formattedErrors := ""
	if len(errors) > 0 {
		errorMap := make(map[string]int)
		for _, err := range errors {
			errorMap[err]++
		}
		for k, v := range errorMap {
			formattedErrors += fmt.Sprintf("Error %s: %d\n", k, v)
		}
	}

	slices.Sort(successDurations)
	p99 := percentile(successDurations, 0.99)
	p95 := percentile(successDurations, 0.95)
	p90 := percentile(successDurations, 0.90)
	avg := time.Duration(0)
	if len(successDurations) > 0 {
		for _, d := range successDurations {
			avg += d
		}
		avg = avg / time.Duration(len(successDurations))
	}

	// metrics
	metrics := map[string]interface{}{
		"requests":      requests,
		"elapsed":       elapsed.Truncate(time.Second),
		"rps":           fmt.Sprintf("%.2f", currentRps),
		"success count": successCount,
		"error count":   errorCount,
		"avg duration":  avg.Round(time.Millisecond),
	}
	if *duration > 0 {
		metrics["duration"] = *duration
	}
	var formated string

	keys := []string{}
	for k := range metrics {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, key := range keys {
		value := metrics[key]
		formated += fmt.Sprintf("%s: %v ", key, value)
	}
	clearScreen()
	log.Print(formattedErrors)
	log.Print(formated)
	if printTimes {
		latency := fmt.Sprintf("p99: %s, p95: %s, p90: %s", p99.Round(time.Millisecond), p95.Round(time.Millisecond), p90.Round(time.Millisecond))
		log.Print(latency)
	}
}

func percentile(durations []time.Duration, p float64) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	idx := int(math.Ceil(p*float64(len(durations)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(durations) {
		idx = len(durations) - 1
	}
	return durations[idx]
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

func doRequest(url string, headers []string, authentication string, successChannel chan time.Duration, errorsChannel chan string) {
	client := &http.Client{}
	start := time.Now()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		errorsChannel <- "Error creating request"
		return
	}
	for _, header := range headers {
		headerParts := strings.Split(header, ":")
		if len(headerParts) != 2 {
			log.Printf("Invalid header: %s", header)
			os.Exit(1)
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
		errorsChannel <- "Error making request"
		return
	}
	elapsed := time.Since(start)
	if resp.StatusCode != 200 {
		log.Printf("Error: Status code %d", resp.StatusCode)
		errorsChannel <- fmt.Sprintf("%d", resp.StatusCode)
		return
	}
	successChannel <- elapsed
	defer resp.Body.Close()
}

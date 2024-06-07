package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Dataset map[string]string
type progressWriter struct {
    total    int64
    current  int64
    ticker   *time.Ticker
    filename string
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.current += int64(n)

	select {
	case <-pw.ticker.C:
		fmt.Printf("Downloading %s... %d%% \n", pw.filename, pw.current*100/pw.total)
	default:
	}

	return n, nil
}

func IsVideoCorrupted(path string) (bool, error) {
	cmd := exec.Command("ffmpeg", "-v", "error", "-i", path, "-f", "null", "-")
	output, err := cmd.CombinedOutput()
	
	outputStr := string(output)

	if strings.Contains(outputStr, "error") || strings.Contains(outputStr, "Invalid data found when processing input"){
		fmt.Println("Video is corrupted: ", path, string(output))
		return true, nil
	}

	if err != nil {
		return false, fmt.Errorf("%s: %v", output, err)
	}

	return false, nil
}
func DownloadVideos() error {
	file, err := os.Open("dataset.json")
	if err != nil {
		return fmt.Errorf("failed to open dataset.json: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	dataset := Dataset{}
	err = decoder.Decode(&dataset)
	if err != nil {
		return fmt.Errorf("failed to decode dataset: %w", err)
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, 8) // limit to x concurrent downloads

	for filename, url := range dataset {
		wg.Add(1)
		go func(filename, url string) {
			defer wg.Done()
			sem <- struct{}{} // acquire a token
			defer func() { <-sem }() // release the token

			path := filepath.Join("Video", filename)
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				corrupted, err := IsVideoCorrupted(path)
				if err != nil {
					fmt.Printf("failed to check if video '%s' from url '%s' is corrupted: %v\n", filename, url, err)
					return
				}
				if corrupted {
					err = os.Remove(path)
					if err != nil {
						fmt.Printf("failed to remove corrupted video: %v\n", err)
						return
					}
				}
			}

			out, err := os.Create(path)
			if err != nil {
				fmt.Printf("failed to create file %s: %v\n", path, err)
				return
			}
			defer out.Close()

			resp, err := http.Get(url)
			if err != nil {
				fmt.Printf("failed to get url %s: %v\n", url, err)
				return
			}
			defer resp.Body.Close()

			pw := &progressWriter{
				total:    resp.ContentLength,
				ticker:   time.NewTicker(5 * time.Second),
				filename: filename,
			}
			_, err = io.Copy(out, io.TeeReader(resp.Body, pw))
			defer pw.ticker.Stop()
			if err != nil {
				fmt.Printf("failed to copy data to file: %v\n", err)
			}
			fmt.Printf("\nDownload of %s finished\n", filename)
		}(filename, url)
	}

	wg.Wait()
	return nil
}
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
)

type Dataset map[string]string

type progressWriter struct {
	total   int64
	current int64
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.current += int64(n)
	fmt.Printf("Downloading... %d%% \n", pw.current*100/pw.total)
	return n, nil
}

func IsVideoCorrupted(path string) (bool, error) {
	cmd := exec.Command("ffmpeg", "-v", "error", "-i", path, "-f", "null", "-")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("%s: %v", output, err)
	}

	if strings.Contains(string(output), "error") {
		fmt.Println("Video is corrupted: ", path, string(output))
		return true, nil
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

	for filename, url := range dataset {
		wg.Add(1)
		go func(filename, url string) {
			defer wg.Done()
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

			pw := &progressWriter{total: resp.ContentLength}
			_, err = io.Copy(out, io.TeeReader(resp.Body, pw))
			if err != nil {
				fmt.Printf("failed to copy data to file: %v\n", err)
			}
			fmt.Println("\nDownload finished")
		}(filename, url)
	}

	wg.Wait()
	return nil
}

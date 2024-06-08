package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
    // Create a context with a timeout
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second) // 30 seconds timeout
    defer cancel()

    // Use exec.CommandContext instead of exec.Command to apply the timeout
    cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=codec_type", "-of", "default=noprint_wrappers=1:nokey=1", path)
    output, err := cmd.CombinedOutput()

    outputStr := string(output)

    if ctx.Err() == context.DeadlineExceeded {
        // The command was killed due to the timeout
        return true, nil // seem good
    }

    if strings.Contains(outputStr, "error") || strings.Contains(outputStr, "Invalid data found when processing input") {
        fmt.Println("Video is corrupted: ", path, outputStr)
        return true, nil
    }

    if err != nil {
        return false, fmt.Errorf("%s: %v", outputStr, err)
    }

    return false, nil
}

// CacheEntry represents the cache structure for each video file.
type CacheEntry struct {
    Hash     string `json:"hash"`
    Corrupted bool   `json:"corrupted"`
}

// ReadCache reads the cache from cache.json.
func ReadCache() (map[string]CacheEntry, error) {
    cache := make(map[string]CacheEntry)
    data, err := os.ReadFile("cache.json")
    if err != nil {
        if os.IsNotExist(err) {
            return cache, nil // Return an empty cache if the file doesn't exist
        }
        return nil, err
    }
    err = json.Unmarshal(data, &cache)
    return cache, err
}

// WriteCache writes the cache to cache.json.
func WriteCache(cache map[string]CacheEntry) error {
    data, err := json.Marshal(cache)
    if err != nil {
        return err
    }
    return os.WriteFile("cache.json", data, 0644)
}

// HashFile generates a SHA256 hash for the contents of a file.
func HashFile(path string) (string, error) {
    file, err := os.Open(path)
    if err != nil {
        return "", err
    }
    defer file.Close()

    hasher := sha256.New()
    if _, err := io.Copy(hasher, file); err != nil {
        return "", err
    }

    return hex.EncodeToString(hasher.Sum(nil)), nil
}

func ValidVideos() ([]string, error) {
    cache, err := ReadCache()
    if err != nil {
        return nil, fmt.Errorf("failed to read cache: %w", err)
    }

    files, err := os.ReadDir("Video")
    if err != nil {
        return nil, fmt.Errorf("failed to read 'Video' directory: %w", err)
    }

    var validVideos []string
    for _, file := range files {
        if !file.IsDir() {
            filePath := filepath.Join("Video", file.Name())
            fileHash, err := HashFile(filePath)
            if err != nil {
                return nil, fmt.Errorf("failed to hash video '%s': %w", file.Name(), err)
            }

            if entry, exists := cache[fileHash]; exists {
                if !entry.Corrupted {
                    validVideos = append(validVideos, file.Name())
                }
                continue
            }

            corrupted, err := IsVideoCorrupted(filePath)
            if err != nil {
                return nil, fmt.Errorf("failed to check if video '%s' is corrupted: %w", file.Name(), err)
            }

            cache[fileHash] = CacheEntry{Hash: fileHash, Corrupted: corrupted}
            if !corrupted {
                validVideos = append(validVideos, file.Name())
            }
        }
    }

    if err := WriteCache(cache); err != nil {
        return nil, fmt.Errorf("failed to write cache: %w", err)
    }

    return validVideos, nil
}

func DownloadVideos(maxDownloads int) error {
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
	var i int = 0
	for filename, url := range dataset {
		if i >= maxDownloads && maxDownloads != -1 {
			break
		}
		i++
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

func CleanupCorruptedVideos() error {
	files, err := os.ReadDir("Video")
	if err != nil {
		return fmt.Errorf("failed to read 'Video' directory: %w", err)
	}

	for _, file := range files {
		if !file.IsDir() {
			corrupted, err := IsVideoCorrupted(filepath.Join("Video", file.Name()))
			if err != nil {
				return fmt.Errorf("failed to check if video '%s' is corrupted: %w", file.Name(), err)
			}
			if corrupted {
				err = os.Remove(filepath.Join("Video", file.Name()))
				if err != nil {
					return fmt.Errorf("failed to remove corrupted video: %w", err)
				}
				fmt.Printf("Removed corrupted video: %s\n", file.Name())
			}
		}
	}

	return nil
}
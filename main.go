package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

func main() {
	// get args
	var single_vid bool = false
	var dont_download bool = false
	for _, arg := range os.Args[1:] {
		if arg == "-s" {
			single_vid = true
		}
		if arg == "-d" {
			dont_download = true
		}
	}
	var download_vids_count = -1
	if single_vid {
		download_vids_count = 1
	}

	if dont_download {
		fmt.Println("Skipping download")
	} else {
		fmt.Println("Downloading videos")

		var err = DownloadVideos(download_vids_count)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}

	var wg sync.WaitGroup
	wg.Add(1)

	cmd := exec.Command("./mediamtx/mediamtx")
	stdout, _ := cmd.StdoutPipe()
	scanner := bufio.NewScanner(stdout)

	go func() {
		defer wg.Done()
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Println(line)
			if strings.Contains(line, "[SRT] listener opened on :8890 (UDP)") {
				break
			}
		}
	}()

	if err := cmd.Start(); err != nil {
		fmt.Println("Error starting command:", err)
		os.Exit(1)
	}

	wg.Wait()

	fmt.Println("Hello, world.")

	// Exit the program
	cmd.Process.Kill()
	os.Exit(0)
}

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)



func ffmpeg_stream_loop(video string, stream string) {
	go func() {
		// validate video
		corrupted, err := IsVideoCorrupted(video)
		if err != nil {
			fmt.Println(err)
			return // Exit the goroutine, replace with more sophisticated error handling if necessary
		}
		if corrupted {
			fmt.Println("Video is corrupted: ", video)
			return // Exit the goroutine
		}
		cmd := exec.Command("ffmpeg", "-re", "-stream_loop", "-1", "-i", video, "-c:v", "libx264", "-x264opts", "bframes=0", "-g", "50", "-keyint_min", "50", "-c:a", "aac", "-f", "rtsp", "-rtsp_transport", "tcp", "rtsp://localhost:8554/"+stream)
		output, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Error starting command: %v, Output: %s\n", err, output)
		}
		
	}()
}

func main() {
	// get args
	var single_vid bool = false
	var dont_download bool = false
	var remove_corrupted bool = false
	var port string = "8080"
	for _, arg := range os.Args[1:] {
		if arg == "-s" {
			single_vid = true
		}
		if arg == "-d" {
			dont_download = true
		}
		if arg == "-r" {
			remove_corrupted = true
		}
		if strings.Contains(arg, "-p=") {
			port = strings.Split(arg, "=")[1]
		}

		if arg == "--help" {
			fmt.Println("Usage: ./main [-s] [-d]")
			fmt.Println("Options:")
			fmt.Println("  -s: Download only one video")
			fmt.Println("  -d: Skip downloading videos")
			fmt.Println("  -r: Remove corrupted videos")
			os.Exit(0)
		}
	}
	if remove_corrupted {
		fmt.Println("Removing corrupted videos")
		err := CleanupCorruptedVideos()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// validate port
	if _, err := strconv.Atoi(port); err != nil {
		fmt.Println("Invalid port")
		os.Exit(1)
	}

	var download_vids_count = -1
	if single_vid {
		download_vids_count = 1
	}

	// check for ffmpeg
	cmd := exec.Command("ffmpeg", "-version")
	err := cmd.Run()
	if err != nil {
		fmt.Println("ffmpeg not found. Please install ffmpeg.")
		os.Exit(1)
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

	cmd = exec.Command("./mediamtx/mediamtx")
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

	
	//ffmpeg -re -stream_loop -1 -i VIRAT_S_000001.mp4 -c:v libx264 -x264opts bframes=0 -g 50 -keyint_min 50 -c:a aac -f rtsp -rtsp_transport tcp rtsp://localhost:8554/mystream
	available_videos, err := ValidVideos()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Println("Hello, world.")
	if len(available_videos) == 0 {
		fmt.Println("No videos available")
		os.Exit(1)
	}

	var stream_lists map[string]string = make(map[string]string)

	if single_vid {
		ffmpeg_stream_loop("Video/"+available_videos[0], "mystream")
		stream_lists["mystream"] = available_videos[0]
	} else {
		for _, video := range available_videos {
			stream_name := strings.ToLower(strings.Split(video, ".")[0])
			ffmpeg_stream_loop("Video/"+video, stream_name)
			stream_lists[stream_name] = video
		}
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// json
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stream_lists)
	})

	go func() {
	http.ListenAndServe(":"+port, nil)
	}()
	// get port
	fmt.Println("Server started on port " + port)
	fmt.Println("http://localhost:" + port)
	fmt.Println("Press Ctrl+C to exit")
	// Wait for Ctrl+C
	<-make(chan struct{})

	// Exit the program
	cmd.Process.Kill()
	os.Exit(0)
}

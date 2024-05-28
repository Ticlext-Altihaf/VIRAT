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
	var err = DownloadVideos()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
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

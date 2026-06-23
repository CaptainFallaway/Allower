package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

func getEnvPort() (string, error) {
	port, found := os.LookupEnv("PORT")
	if !found {
		return "", fmt.Errorf("PORT environment variable not set")
	}
	return port, nil
}

func main() {
	port, err := getEnvPort()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	cmd := exec.Command("iperf3", "-s", "-p", port)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, os.Interrupt)

	go func() {
		<-c
		cmd.Process.Kill()
	}()

	if err = cmd.Run(); err != nil {
		fmt.Printf("Error starting iperf3 server: %v\n", err)
		os.Exit(1)
	}
}

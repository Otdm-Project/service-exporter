package main

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	aliveMetric = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "systemd_socket_alive",
		Help: "Indicates if 'alive' message was received on the systemd socket.",
	})
	pidFile = "/var/run/service-exporter.pid"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: service-exporter {up|down}")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "up":
		startService()
	case "down":
		stopService()
	default:
		fmt.Println("Unknown command:", os.Args[1])
		fmt.Println("Usage: service-exporter {up|down}")
		os.Exit(1)
	}
}

func startService() {
	err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating PID file: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(pidFile)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		fmt.Println("Received shutdown signal")
		os.Remove(pidFile)
		os.Exit(0)
	}()

	listener, err := net.Listen("unix", "/run/systemd/service-exporter.sock")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listening on socket: %v\n", err)
		os.Exit(1)
	}
	defer listener.Close()

	http.Handle("/metrics", promhttp.Handler())
	go func() {
		http.ListenAndServe(":9200", nil)
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error accepting connection: %v\n", err)
			continue
		}

		go handleConnection(conn)
	}
}

func stopService() {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading PID file: %v\n", err)
		os.Exit(1)
	}

	pid, err := strconv.Atoi(string(data))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error converting PID to integer: %v\n", err)
		os.Exit(1)
	}

	err = syscall.Kill(pid, syscall.SIGTERM)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error sending SIGTERM to process: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Service stopped")
}

func handleConnection(conn net.Conn) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	var mu sync.Mutex
	receivedAlive := false

	go func() {
		for range ticker.C {
			mu.Lock()
			if !receivedAlive {
				aliveMetric.Set(0)
			}
			receivedAlive = false
			mu.Unlock()
		}
	}()

	for scanner.Scan() {
		message := scanner.Text()
		mu.Lock()
		if message == "alive" {
			aliveMetric.Set(1)
			receivedAlive = true
		} else {
			aliveMetric.Set(0)
		}
		mu.Unlock()
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading from socket: %v\n", err)
	}
}

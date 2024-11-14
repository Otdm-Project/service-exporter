package main

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	aliveMetric = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "service_alive",
		Help: "Indicates if 'alive' message was received on the systemd socket.",
	})
)

func handleConnection(conn net.Conn) {
	defer conn.Close()
	currentTime := time.Now().Unix()
	scanner := bufio.NewScanner(conn)

	http.Handle("/metrics", promhttp.Handler())
	go func() {
		http.ListenAndServe(":9200", nil)
	}()

	go func() {
		for range time.Tick(10 * time.Second) {
			if time.Now().Unix()-currentTime > 15 {
				aliveMetric.Set(0)
			} else {
				aliveMetric.Set(1)
			}
		}
	}()

	for scanner.Scan() {
		currentTime = time.Now().Unix()
	}

	if err := scanner.Err(); err != nil {
		fmt.Println("Error reading from connection:", err)
	}
}

func main() {
	listener, err := net.Listen("tcp", "localhost:2000")
	if err != nil {
		fmt.Println("Error starting TCP server:", err)
		return
	}
	defer listener.Close()

	fmt.Println("TCP server listening on port 2000")

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error accepting connection:", err)
			continue
		}

		go handleConnection(conn)
	}
}

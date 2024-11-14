package main

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"sync"
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
	lastMessageTime int64
	mu              sync.Mutex
)

func handleConnection(conn net.Conn) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)

	for scanner.Scan() {
		// メッセージ受信時にlastMessageTimeを更新
		mu.Lock()
		lastMessageTime = time.Now().Unix()
		mu.Unlock()
	}

	// 接続エラーが発生した場合のエラーログ出力
	if err := scanner.Err(); err != nil {
		fmt.Println("Error reading from connection:", err)
	}
}

func main() {
	// /metricsエンドポイントを1回だけ登録
	http.Handle("/metrics", promhttp.Handler())

	// HTTPサーバーをバックグラウンドで起動
	go func() {
		if err := http.ListenAndServe(":9200", nil); err != nil {
			fmt.Println("Error starting HTTP server:", err)
		}
	}()

	// メトリクスを定期的に更新するゴルーチンを開始
	go func() {
		for range time.Tick(10 * time.Second) {
			mu.Lock()
			if time.Now().Unix()-lastMessageTime > 15 {
				aliveMetric.Set(0)
				fmt.Println("Metric set to 0 (no message in last 15 seconds)")
			} else {
				aliveMetric.Set(1)
				fmt.Println("Metric set to 1 (message received recently)")
			}
			mu.Unlock()
		}
	}()

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

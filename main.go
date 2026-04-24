package main

import (
	"flag"
	"fmt"
	"net/http"
	"screen/configuration"
	"screen/logging"
	"screen/metrics"
	"screen/proxy"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var log = logging.NewLogger("main")

func main() {
	flag.Parse()
	config := configuration.GetConfig()

	if config.MetricsPort > 0 {
		go startMetricsServer(config.MetricsPort)
	}

	server, err := proxy.NewServer(config)
	if err != nil {
		log.Fatalf("server: %v", err)
	}

	addr := fmt.Sprintf(":%d", config.Port)
	log.Infof("proxy listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, server))
}

func startMetricsServer(port int) {
	addr := fmt.Sprintf(":%d", port)
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(metrics.Registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	}))
	log.Infof("metrics listening on %s/metrics", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Errorf("metrics server stopped: %v", err)
	}
}

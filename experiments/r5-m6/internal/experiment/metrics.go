package experiment

import (
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"
)

type Metrics struct {
	mutex  sync.Mutex
	values map[string]int64
}

func (metrics *Metrics) Add(name string, delta int64) {
	metrics.mutex.Lock()
	defer metrics.mutex.Unlock()
	if metrics.values == nil {
		metrics.values = make(map[string]int64)
	}
	metrics.values[name] += delta
}
func MetricsServer(address string, identities []Identity, metrics *Metrics) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(writer http.ResponseWriter, request *http.Request) {
		identity, valid := Authenticate(identities, request.Header.Get("X-Service-ID"), request.Header.Get("Authorization"))
		if request.Method != http.MethodGet || !valid || !hasScope(identity, "metrics") {
			http.Error(writer, "forbidden", http.StatusForbidden)
			return
		}
		writer.Header().Set("Content-Type", "text/plain; version=0.0.4")
		metrics.mutex.Lock()
		defer metrics.mutex.Unlock()
		names := make([]string, 0, len(metrics.values))
		for name := range metrics.values {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Fprintf(writer, "r5_%s_total %d\n", name, metrics.values[name])
		}
	})
	return &http.Server{Addr: address, Handler: mux, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 15 * time.Second, MaxHeaderBytes: 8192}
}

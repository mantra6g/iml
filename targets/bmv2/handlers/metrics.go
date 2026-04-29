package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	v1 "github.com/p4lang/p4runtime/go/p4/v1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MetricsHandler reads counter data from the switch and serves it in Prometheus text format.
func (d *Driver) MetricsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	counterNames := d.buildCounterNameMap()

	stream, err := d.Client.Read(ctx, &v1.ReadRequest{
		Entities: []*v1.Entity{
			{Entity: &v1.Entity_CounterEntry{CounterEntry: &v1.CounterEntry{}}},
		},
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var entries []*v1.CounterEntry
	for {
		response, err := stream.Recv()
		if err != nil {
			break
		}
		for _, entity := range response.GetEntities() {
			if ce := entity.GetCounterEntry(); ce != nil {
				entries = append(entries, ce)
			}
		}
	}

	reg := prometheus.NewRegistry()
	labelNames := []string{"counter_id", "counter_name", "index"}
	packetGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "p4_counter_packets_total",
		Help: "Packet count per P4 counter entry",
	}, labelNames)
	byteGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "p4_counter_bytes_total",
		Help: "Byte count per P4 counter entry",
	}, labelNames)
	reg.MustRegister(packetGauge, byteGauge)

	for _, e := range entries {
		if e.Data == nil || (e.Data.PacketCount == 0 && e.Data.ByteCount == 0) {
			continue
		}
		idx := int64(0)
		if e.Index != nil {
			idx = e.Index.Index
		}
		lv := prometheus.Labels{
			"counter_id":   fmt.Sprintf("%d", e.CounterId),
			"counter_name": counterNames[e.CounterId],
			"index":        fmt.Sprintf("%d", idx),
		}
		packetGauge.With(lv).Set(float64(e.Data.PacketCount))
		byteGauge.With(lv).Set(float64(e.Data.ByteCount))
	}

	promhttp.HandlerFor(reg, promhttp.HandlerOpts{}).ServeHTTP(w, r)
}

func (d *Driver) buildCounterNameMap() map[uint32]string {
	names := make(map[uint32]string)
	if d.CurrentProgram == nil || d.CurrentProgram.P4Info == nil {
		return names
	}
	for _, c := range d.CurrentProgram.P4Info.Counters {
		if c.Preamble != nil {
			names[c.Preamble.Id] = c.Preamble.Name
		}
	}
	for _, c := range d.CurrentProgram.P4Info.DirectCounters {
		if c.Preamble != nil {
			names[c.Preamble.Id] = c.Preamble.Name
		}
	}
	return names
}

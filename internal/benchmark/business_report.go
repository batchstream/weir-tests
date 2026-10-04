package benchmark

// BusinessComparison describes measured single-request throughput and latency
// at the same client concurrency, even when database full capacity is unknown.
type BusinessComparison struct {
	Concurrency   int     `json:"concurrency"`
	DirectOps     float64 `json:"direct_successful_operations_per_second"`
	WeirOps       float64 `json:"weir_successful_operations_per_second"`
	Ratio         float64 `json:"observed_weir_direct_throughput_ratio"`
	DeltaPercent  float64 `json:"observed_weir_throughput_delta_percent"`
	DirectLatency Latency `json:"direct_business_request_latency"`
	WeirLatency   Latency `json:"weir_business_request_latency"`
	DirectDBFull  bool    `json:"direct_all_rounds_database_cpu_saturated"`
	WeirDBFull    bool    `json:"weir_all_rounds_database_cpu_saturated"`
	Scope         string  `json:"scope"`
}

type BusinessPeaks struct {
	Direct       *SaturationPoint `json:"direct_observed_peak"`
	Weir         *SaturationPoint `json:"weir_observed_peak"`
	Ratio        float64          `json:"observed_weir_direct_peak_throughput_ratio"`
	DeltaPercent float64          `json:"observed_weir_peak_throughput_delta_percent"`
	Scope        string           `json:"scope"`
}

func (r *SaturationReport) aggregateBusiness() {
	r.BusinessComparisons = nil
	if r.Parameters.ClientProcesses == 0 || r.Incomplete != "" {
		return
	}
	for _, workers := range r.Parameters.Concurrency {
		comparison := BusinessComparison{Concurrency: workers, Scope: "observed steady single-request business throughput and pooled per-request latency on the recorded hardware; does not imply maximum database capacity"}
		var directHistogram, weirHistogram histogram
		verified := true
		rounds := 0
		for _, pair := range r.Pairs {
			if pair.Concurrency != workers {
				continue
			}
			rounds++
			verified = verified && pair.Direct.complete() && pair.Weir.complete() && pair.Direct.Verified && pair.Weir.Verified
			for _, client := range pair.Direct.ClientProcesses {
				value := histogramFromClient(client)
				directHistogram.merge(&value)
			}
			for _, client := range pair.Weir.ClientProcesses {
				value := histogramFromClient(client)
				weirHistogram.merge(&value)
			}
		}
		if !verified || rounds != r.Parameters.Rounds {
			continue
		}
		for _, point := range r.Points {
			if point.Concurrency != workers {
				continue
			}
			if point.Path == "direct" {
				comparison.DirectOps = point.OperationsPerSec
				comparison.DirectDBFull = point.AllRoundsCPUSaturated
			} else {
				comparison.WeirOps = point.OperationsPerSec
				comparison.WeirDBFull = point.AllRoundsCPUSaturated
			}
		}
		if comparison.DirectOps <= 0 {
			continue
		}
		comparison.Ratio = comparison.WeirOps / comparison.DirectOps
		comparison.DeltaPercent = 100 * (comparison.Ratio - 1)
		comparison.DirectLatency = directHistogram.summary()
		comparison.WeirLatency = weirHistogram.summary()
		r.BusinessComparisons = append(r.BusinessComparisons, comparison)
	}
	peaks := &BusinessPeaks{Scope: "best verified business throughput within this measured concurrency ladder on the recorded hardware; does not imply maximum database capacity"}
	for index := range r.Points {
		point := &r.Points[index]
		if !point.AllRoundsVerified {
			continue
		}
		if point.Path == "direct" && (peaks.Direct == nil || point.OperationsPerSec > peaks.Direct.OperationsPerSec) {
			peaks.Direct = point
		}
		if point.Path == "weir" && (peaks.Weir == nil || point.OperationsPerSec > peaks.Weir.OperationsPerSec) {
			peaks.Weir = point
		}
	}
	if peaks.Direct != nil && peaks.Weir != nil && peaks.Direct.OperationsPerSec > 0 {
		peaks.Ratio = peaks.Weir.OperationsPerSec / peaks.Direct.OperationsPerSec
		peaks.DeltaPercent = 100 * (peaks.Ratio - 1)
		r.BusinessPeaks = peaks
	}
}

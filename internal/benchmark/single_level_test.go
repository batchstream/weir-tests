package benchmark

import (
	"testing"
	"time"
)

func TestSingleCalibrationLevelCannotInferAThroughputPlateau(t *testing.T) {
	parameters := SaturationParameters{Concurrency: []int{512}, BatchSizes: []int{1}, Rounds: 1}
	base := Result{Planned: 1000, Attempted: 1000, Succeeded: 1000, ElapsedNS: int64(time.Second), Verified: true}
	resources := Resources{CPUSaturated: true, MeanCPUPercent: 100}
	stage := SaturationResult{Result: base, Resources: resources}
	pair := SaturationPair{Round: 1, Concurrency: 512, BatchSize: 1, Direct: stage, Weir: stage}
	report := &SaturationReport{Parameters: parameters, Pairs: []SaturationPair{pair}}
	report.aggregate()
	if len(report.Points) != 2 || len(report.Comparisons) != 1 || report.Comparisons[0].Ratio != nil {
		t.Fatal("one calibrated load level produced a maximum-capacity claim", report)
	}
	for _, point := range report.Points {
		if point.Plateau || point.Saturated {
			t.Fatal("database CPU alone established a missing next-level plateau", point)
		}
	}
}

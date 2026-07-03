package statsview

import (
	"testing"

	"v.wingsnet.org/internal/storage/dbmodel"
)

func TestAggregateTrafficSeries(t *testing.T) {
	samples := []dbmodel.TrafficSample{
		{NodeID: "n1", TsUnix: 0, RxBytes: 0, TxBytes: 0},
		{NodeID: "n1", TsUnix: 60, RxBytes: 100, TxBytes: 10},
		{NodeID: "n1", TsUnix: 120, RxBytes: 300, TxBytes: 40},
		{NodeID: "n2", TsUnix: 0, RxBytes: 1000, TxBytes: 0},
		{NodeID: "n2", TsUnix: 60, RxBytes: 1050, TxBytes: 5},
	}
	series, rx24, tx24 := aggregateTrafficSeries(samples, 60)
	if rx24 != 350 || tx24 != 45 {
		t.Fatalf("totals wrong: rx=%d tx=%d (want 350/45)", rx24, tx24)
	}
	if len(series) != 2 {
		t.Fatalf("want 2 buckets, got %d: %+v", len(series), series)
	}
	if series[0].Ts != 60 || series[0].RxBytes != 150 || series[0].TxBytes != 15 {
		t.Fatalf("bucket 60 wrong: %+v", series[0])
	}
	if series[1].Ts != 120 || series[1].RxBytes != 200 || series[1].TxBytes != 30 {
		t.Fatalf("bucket 120 wrong: %+v", series[1])
	}
}

func TestAggregateTrafficSeriesCounterReset(t *testing.T) {
	samples := []dbmodel.TrafficSample{
		{NodeID: "n1", TsUnix: 0, RxBytes: 500},
		{NodeID: "n1", TsUnix: 60, RxBytes: 50},
		{NodeID: "n1", TsUnix: 120, RxBytes: 90},
	}
	_, rx24, _ := aggregateTrafficSeries(samples, 60)
	if rx24 != 40 {
		t.Fatalf("counter reset should yield only the 40-byte post-reset delta, got %d", rx24)
	}
}

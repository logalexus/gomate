package integration

import (
	"context"
	"os"
	"testing"

	"gomate/internal/storage"
)

func TestRemoveHistoricalTheftContribution(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	_, err := s.Pool.Exec(ctx, `INSERT INTO gomate_streams(flow_key,service,protocol,start_ms,end_ms,ttl,client,server,pattern_ids)
 SELECT 'migration-'||i,80,'TCP',i,i,0,'client','server',ARRAY[7] FROM generate_series(1,4) i;
 INSERT INTO gomate_anomalies(stream_id,service,score,kind,summary,reasons,dismissed,updated_ms) VALUES
 (1,80,40,'theft','pure','[{"detector":"theft","bits":40}]',false,1),
 (2,80,52,'theft','mixed','[{"detector":"theft","bits":40},{"detector":"chars","bits":12}]',true,2),
 (3,80,46,'theft','threshold','[{"detector":"theft","bits":40},{"detector":"enum","bits":6}]',false,3),
 (4,80,20,'','ordinary','[{"detector":"length","bits":20}]',false,4);
 DELETE FROM gomate_schema_versions WHERE version=10;`)
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		reopened, err := storage.Open(ctx, os.Getenv("TEST_DATABASE_URL"))
		if err != nil {
			t.Fatal(err)
		}
		reopened.Close()
		var count int
		var score float64
		var kind, reasons string
		var dismissed bool
		if err = s.Pool.QueryRow(ctx, "SELECT count(*) FROM gomate_anomalies").Scan(&count); err != nil || count != 3 {
			t.Fatal("pure flag alert remains", count, err)
		}
		if err = s.Pool.QueryRow(ctx, "SELECT score,kind,reasons::text,dismissed FROM gomate_anomalies WHERE stream_id=2").Scan(&score, &kind, &reasons, &dismissed); err != nil || score != 12 || kind != "" || !dismissed || reasons != `[{"bits": 12, "detector": "chars"}]` {
			t.Fatal("mixed alert corrupted", score, kind, reasons, dismissed, err)
		}
		for id, want := range map[int]float64{3: 6, 4: 20} {
			if err = s.Pool.QueryRow(ctx, "SELECT score FROM gomate_anomalies WHERE stream_id=$1", id).Scan(&score); err != nil || score != want {
				t.Fatal("threshold or unrelated score changed", id, score, err)
			}
		}
		if err = s.Pool.QueryRow(ctx, "SELECT count(*) FROM gomate_streams WHERE pattern_ids=ARRAY[7]").Scan(&count); err != nil || count != 4 {
			t.Fatal("session matches lost", count, err)
		}
	}
}

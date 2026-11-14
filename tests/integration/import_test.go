package integration

import (
	"context"
	"gomate/internal/model"
	"net/url"
	"os"
	"testing"
)

func TestLegacyImportResumesWithoutDuplicates(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	_, e := s.Pool.Exec(ctx, `
 DROP SCHEMA IF EXISTS legacy_fixture CASCADE; CREATE SCHEMA legacy_fixture;
 CREATE TABLE legacy_fixture.service(port int,name text,http bool,urldecode_http_requests bool,merge_adjacent_packets bool,parse_web_sockets bool,decrypt_tls bool);
 INSERT INTO legacy_fixture.service VALUES(80,'web',true,true,true,false,false);
 CREATE TABLE legacy_fixture.pattern(id int,enabled bool,deleted bool,name text,value text,color text,search_type int,direction_type int,action_type int,service_id int);
 INSERT INTO legacy_fixture.pattern VALUES(7,true,false,'flag','FLAG','#aabbcc',1,2,0,80);
 CREATE TABLE legacy_fixture.stream(id bigint,service_id int,protocol int,start_timestamp bigint,end_timestamp bigint,favorite bool,ttl int,user_agent_hash text,size_bytes int,packets_count int);
 INSERT INTO legacy_fixture.stream VALUES(21,80,0,1,2,true,64,'abc',4,1);
 CREATE TABLE legacy_fixture.stream_found_patterns(stream_id bigint,pattern_id int);INSERT INTO legacy_fixture.stream_found_patterns VALUES(21,7);
 CREATE TABLE legacy_fixture.packet(id bigint,stream_id bigint,timestamp bigint,incoming bool,content bytea,http_processed bool,web_socket_parsed bool,tls_decrypted bool,has_http_body bool);
 INSERT INTO legacy_fixture.packet VALUES(42,21,1,false,'FLAG'::bytea,true,false,false,true);
 CREATE TABLE legacy_fixture.found_pattern(packet_id bigint,pattern_id int,start_position int,end_position int);INSERT INTO legacy_fixture.found_pattern VALUES(42,7,0,3);
 `)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Pool.Exec(ctx, "DROP SCHEMA legacy_fixture CASCADE") })
	u, _ := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	q := u.Query()
	q.Set("search_path", "legacy_fixture")
	u.RawQuery = q.Encode()
	if e = s.ImportLegacy(ctx, u.String(), "fixture", false); e != nil {
		t.Fatal(e)
	}
	if e = s.ImportLegacy(ctx, u.String(), "fixture", true); e != nil {
		t.Fatal(e)
	}
	if e = s.ImportLegacy(ctx, u.String(), "fixture", true); e != nil {
		t.Fatal(e)
	}
	streams, e := s.Streams(ctx, 80, model.Page{PageSize: 10})
	if e != nil || len(streams) != 1 || streams[0].ID != 21 || !streams[0].Favorite || streams[0].Patterns[0] != 7 {
		t.Fatal(streams, e)
	}
	packets, e := s.Packets(ctx, 21, model.Page{PageSize: 10})
	if e != nil || len(packets) != 1 || packets[0].ID != 42 || packets[0].Kind != "legacy" || len(packets[0].Matches) != 1 {
		t.Fatal(packets, e)
	}
	p := model.Pattern{Name: "next"}
	if e = s.PutPattern(ctx, &p); e != nil || p.ID <= 7 {
		t.Fatal(p, e)
	}
	var sourceCount int
	if e = s.Pool.QueryRow(ctx, "SELECT count(*) FROM legacy_fixture.packet").Scan(&sourceCount); e != nil || sourceCount != 1 {
		t.Fatal(sourceCount, e)
	}
}

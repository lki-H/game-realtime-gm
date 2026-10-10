package experiment

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	reportingv1 "game-realtime-gm/experiments/r5-m6/gen/reporting/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type testReader struct {
	delay       time.Duration
	unavailable bool
}

func (reader testReader) Leaderboard(ctx context.Context, limit uint32) (*reportingv1.GetLeaderboardResponse, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(reader.delay):
	}
	if reader.unavailable {
		return nil, errors.New("downstream failed")
	}
	return &reportingv1.GetLeaderboardResponse{Entries: []*reportingv1.LeaderboardEntry{{PlayerId: 1, Score: 100, Rank: 1}}}, nil
}
func (reader testReader) Result(ctx context.Context, id string) (*reportingv1.GetRunResultResponse, error) {
	return &reportingv1.GetRunResultResponse{RunId: id, Participants: []*reportingv1.ParticipantResult{{PlayerId: 1}}}, nil
}
func testIdentities() []Identity {
	return []Identity{{Name: "test", TokenHash: Hash([]byte(strings.Repeat("a", 32))), Scopes: []string{"leaderboard", "result", "metrics"}, Players: []int64{1, 2}}, {Name: "limited", TokenHash: Hash([]byte(strings.Repeat("b", 32))), Scopes: []string{"metrics"}}}
}
func rpcClient(t *testing.T, reader Reader) reportingv1.ReportingClient {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := RPCServer(Config{Identities: testIdentities()}, reader, &Metrics{})
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	connection, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { connection.Close() })
	return reportingv1.NewReportingClient(connection)
}
func authenticated(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "service-id", "test", "authorization", "Bearer "+strings.Repeat("a", 32))
}
func TestRPCIdentityDeadlinesAndLimits(t *testing.T) {
	client := rpcClient(t, testReader{})
	for _, scenario := range []struct {
		name  string
		ctx   func(context.Context) context.Context
		limit uint32
		code  codes.Code
	}{
		{"missing_identity", func(ctx context.Context) context.Context { return ctx }, 10, codes.Unauthenticated},
		{"wrong_token", func(ctx context.Context) context.Context {
			return metadata.AppendToOutgoingContext(ctx, "service-id", "test", "authorization", "Bearer "+strings.Repeat("z", 32))
		}, 10, codes.Unauthenticated},
		{"missing_scope", func(ctx context.Context) context.Context {
			return metadata.AppendToOutgoingContext(ctx, "service-id", "limited", "authorization", "Bearer "+strings.Repeat("b", 32))
		}, 10, codes.PermissionDenied},
		{"zero_limit", authenticated, 0, codes.InvalidArgument},
		{"large_limit", authenticated, 101, codes.InvalidArgument},
		{"valid", authenticated, 10, codes.OK},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, err := client.GetLeaderboard(scenario.ctx(ctx), &reportingv1.GetLeaderboardRequest{Limit: scenario.limit})
			if status.Code(err) != scenario.code {
				t.Fatalf("got %v expected %v", status.Code(err), scenario.code)
			}
		})
	}
	if _, err := client.GetLeaderboard(authenticated(context.Background()), &reportingv1.GetLeaderboardRequest{Limit: 10}); status.Code(err) != codes.InvalidArgument {
		t.Fatal("missing deadline accepted")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := client.GetLeaderboard(authenticated(ctx), &reportingv1.GetLeaderboardRequest{Limit: 10}); status.Code(err) != codes.InvalidArgument {
		t.Fatal("long deadline accepted")
	}
	ctx, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	for _, player := range []int64{2, 3} {
		_, err := client.GetRunResult(authenticated(ctx), &reportingv1.GetRunResultRequest{RunId: "run_test", PlayerId: player})
		if status.Code(err) != codes.PermissionDenied {
			t.Fatal("run/player authorization bypass")
		}
	}
	_, err := client.GetRunResult(authenticated(ctx), &reportingv1.GetRunResultRequest{RunId: strings.Repeat("x", 40000), PlayerId: 1})
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatal("oversized request accepted")
	}
}
func TestRPCDownstreamDeadlineAndCancellation(t *testing.T) {
	client := rpcClient(t, testReader{delay: 100 * time.Millisecond})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := client.GetLeaderboard(authenticated(ctx), &reportingv1.GetLeaderboardRequest{Limit: 1}); status.Code(err) != codes.DeadlineExceeded {
		t.Fatal("deadline not observed")
	}
	ctx, cancel2 := context.WithCancel(authenticated(context.Background()))
	cancel2()
	if _, err := client.GetLeaderboard(ctx, &reportingv1.GetLeaderboardRequest{Limit: 1}); status.Code(err) != codes.Canceled {
		t.Fatal("cancel not observed")
	}
	client = rpcClient(t, testReader{unavailable: true})
	ctx, cancel3 := context.WithTimeout(context.Background(), time.Second)
	defer cancel3()
	if _, err := client.GetLeaderboard(authenticated(ctx), &reportingv1.GetLeaderboardRequest{Limit: 1}); status.Code(err) != codes.Unavailable {
		t.Fatal("downstream error leaked or ignored")
	}
}
func TestMetricsAuthorization(t *testing.T) {
	metrics := &Metrics{}
	metrics.Add("applied", 2)
	server := httptest.NewServer(MetricsServer("127.0.0.1:0", testIdentities(), metrics).Handler)
	defer server.Close()
	response, err := http.Get(server.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("metrics exposed without identity")
	}
	request, _ := http.NewRequest("GET", server.URL+"/metrics", nil)
	request.Header.Set("X-Service-ID", "test")
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 32))
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("metrics identity rejected")
	}
}

package experiment

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"log/slog"
	"strings"
	"time"

	reportingv1 "game-realtime-gm/experiments/r5-m6/gen/reporting/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type identityKey struct{}

func Authenticate(identities []Identity, name, authorization string) (Identity, bool) {
	if !strings.HasPrefix(authorization, "Bearer ") || len(authorization) < 39 || len(authorization) > 256 {
		return Identity{}, false
	}
	digest := sha256.Sum256([]byte(strings.TrimPrefix(authorization, "Bearer ")))
	for _, identity := range identities {
		if identity.Name != name {
			continue
		}
		expected, err := hex.DecodeString(identity.TokenHash)
		if err == nil && subtle.ConstantTimeCompare(digest[:], expected) == 1 {
			return identity, true
		}
	}
	return Identity{}, false
}
func hasScope(identity Identity, scope string) bool {
	for _, allowed := range identity.Scopes {
		if allowed == scope {
			return true
		}
	}
	return false
}
func RPCServer(cfg Config, reader Reader, metrics *Metrics) *grpc.Server {
	capacity := make(chan struct{}, 16)
	interceptor := func(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (response any, err error) {
		started := time.Now()
		defer func() {
			if recover() != nil {
				err = status.Error(codes.Internal, "request failed")
			}
			metrics.Add("rpc_requests", 1)
			if err != nil {
				metrics.Add("rpc_errors", 1)
			}
			slog.Info("r5 rpc", "method", info.FullMethod, "code", status.Code(err).String(), "duration_ms", time.Since(started).Milliseconds())
		}()
		values, _ := metadata.FromIncomingContext(ctx)
		if len(values.Get("service-id")) != 1 || len(values.Get("authorization")) != 1 {
			return nil, status.Error(codes.Unauthenticated, "service identity required")
		}
		identity, valid := Authenticate(cfg.Identities, values.Get("service-id")[0], values.Get("authorization")[0])
		if !valid {
			return nil, status.Error(codes.Unauthenticated, "service identity rejected")
		}
		scope := "result"
		if info.FullMethod == reportingv1.Reporting_GetLeaderboard_FullMethodName {
			scope = "leaderboard"
		}
		if !hasScope(identity, scope) {
			return nil, status.Error(codes.PermissionDenied, "query scope required")
		}
		deadline, present := ctx.Deadline()
		if !present {
			return nil, status.Error(codes.InvalidArgument, "explicit deadline required")
		}
		if time.Until(deadline) > 5*time.Second {
			return nil, status.Error(codes.InvalidArgument, "deadline exceeds five seconds")
		}
		select {
		case capacity <- struct{}{}:
			defer func() { <-capacity }()
		default:
			return nil, status.Error(codes.ResourceExhausted, "query capacity reached")
		}
		return handler(context.WithValue(ctx, identityKey{}, identity), request)
	}
	server := grpc.NewServer(grpc.UnaryInterceptor(interceptor), grpc.MaxRecvMsgSize(32768), grpc.MaxSendMsgSize(1<<20), grpc.MaxConcurrentStreams(16))
	reportingv1.RegisterReportingServer(server, &reportingService{Reader: reader})
	return server
}

type reportingService struct {
	reportingv1.UnimplementedReportingServer
	Reader Reader
}

func (service *reportingService) GetLeaderboard(ctx context.Context, request *reportingv1.GetLeaderboardRequest) (*reportingv1.GetLeaderboardResponse, error) {
	if request.Limit < 1 || request.Limit > 100 {
		return nil, status.Error(codes.InvalidArgument, "limit must be between one and one hundred")
	}
	result, err := service.Reader.Leaderboard(ctx, request.Limit)
	return result, queryError(err)
}
func (service *reportingService) GetRunResult(ctx context.Context, request *reportingv1.GetRunResultRequest) (*reportingv1.GetRunResultResponse, error) {
	if !identifier.MatchString(request.RunId) || request.PlayerId <= 0 {
		return nil, status.Error(codes.InvalidArgument, "valid run and player required")
	}
	identity, _ := ctx.Value(identityKey{}).(Identity)
	allowed := false
	for _, player := range identity.Players {
		allowed = allowed || player == request.PlayerId
	}
	if !allowed {
		return nil, status.Error(codes.PermissionDenied, "player scope required")
	}
	result, err := service.Reader.Result(ctx, request.RunId)
	if err != nil {
		return nil, queryError(err)
	}
	for _, participant := range result.Participants {
		if participant.PlayerId == request.PlayerId {
			return result, nil
		}
	}
	return nil, status.Error(codes.PermissionDenied, "run participation required")
}
func queryError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return status.Error(codes.NotFound, "settled run not found")
	}
	if errors.Is(err, context.Canceled) {
		return status.Error(codes.Canceled, "query canceled")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return status.Error(codes.DeadlineExceeded, "query deadline exceeded")
	}
	return status.Error(codes.Unavailable, "report source unavailable")
}

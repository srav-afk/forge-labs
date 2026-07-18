package observability

import (
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/stats"
)

func GinMiddleware(serviceName string) gin.HandlerFunc {
	return otelgin.Middleware(serviceName)
}

func GRPCServerStatsHandler() stats.Handler {
	return otelgrpc.NewServerHandler()
}

func GRPCClientStatsHandler() stats.Handler {
	return otelgrpc.NewClientHandler()
}

func GRPCServerOption() grpc.ServerOption {
	return grpc.StatsHandler(GRPCServerStatsHandler())
}

func GRPCClientDialOption() grpc.DialOption {
	return grpc.WithStatsHandler(GRPCClientStatsHandler())
}

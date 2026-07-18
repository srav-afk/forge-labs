package workergrpc

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	workerv1 "github.com/srav-afk/forge-labs/gen/worker/v1"
	"github.com/srav-afk/forge-labs/worker/adapters"
)

// compile-time interface check
var _ adapters.RuntimeAdapter = (*fakeAdapter)(nil)

type fakeAdapter struct {
	chunks []adapters.TokenChunk
	err    error
}

func (f *fakeAdapter) Generate(ctx context.Context, req adapters.GenerateRequest, sink func(adapters.TokenChunk) error) error {
	for _, c := range f.chunks {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := sink(c); err != nil {
			return err
		}
	}
	return f.err
}

func (f *fakeAdapter) Capabilities(context.Context) (adapters.Capabilities, error) {
	return adapters.Capabilities{}, nil
}

func (f *fakeAdapter) Ready(context.Context) bool { return true }

func startTestServer(t *testing.T, a adapters.RuntimeAdapter) workerv1.WorkerServiceClient {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	s := grpc.NewServer()
	workerv1.RegisterWorkerServiceServer(s, NewServer(a, "5m"))
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return workerv1.NewWorkerServiceClient(conn)
}

func TestGenerateMapsChunks(t *testing.T) {
	client := startTestServer(t, &fakeAdapter{chunks: []adapters.TokenChunk{
		{Text: "The", Done: false},
		{Text: " sky", Done: false},
		{Text: "", Done: true, FinishReason: "stop", PromptTokens: 11, EvalTokens: 42, TotalDurNs: 4900000000},
	}})

	stream, err := client.Generate(context.Background(), &workerv1.GenerateRequest{
		Model:  &workerv1.ModelRef{BaseModel: "llama3.2"},
		Prompt: "Why is the sky blue?",
	})
	if err != nil {
		t.Fatal(err)
	}

	var got []*workerv1.TokenChunk
	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, msg)
	}
	if len(got) != 3 {
		t.Fatalf("len=%d", len(got))
	}
	if got[2].GetDone() != true || got[2].GetUsage().GetPromptTokens() != 11 {
		t.Fatalf("final=%+v", got[2])
	}
}

func TestGenerateModelNotFound(t *testing.T) {
	client := startTestServer(t, &fakeAdapter{err: adapters.ModelNotFound("missing")})
	stream, err := client.Generate(context.Background(), &workerv1.GenerateRequest{
		Model:  &workerv1.ModelRef{BaseModel: "missing"},
		Prompt: "hi",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = stream.Recv()
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.NotFound {
		t.Fatalf("err=%v", err)
	}
}

func TestGenerateRequiresModel(t *testing.T) {
	client := startTestServer(t, &fakeAdapter{})
	stream, err := client.Generate(context.Background(), &workerv1.GenerateRequest{
		Prompt: "hi",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = stream.Recv()
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.InvalidArgument {
		t.Fatalf("err=%v", err)
	}
}

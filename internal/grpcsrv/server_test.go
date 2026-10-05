package grpcsrv

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"notes/api/gen"
	"notes/internal/notes"
)

func TestGRPCFlow(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(notes.NewStore())
	go srv.Serve(ln)
	defer srv.Stop()

	cc, err := grpc.NewClient(ln.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer cc.Close()
	client := gen.NewNotesServiceClient(cc)
	ctx := context.Background()

	n, err := client.CreateNote(ctx, &gen.CreateNoteRequest{Title: "g", Content: "rpc"})
	if err != nil || n.Etag == "" {
		t.Fatalf("create failed: %v %v", n, err)
	}
	if _, err := client.UpdateNote(ctx, &gen.UpdateNoteRequest{Id: n.Id, Title: "g2", Content: "x", IfMatch: `"stale"`}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition, got %v", err)
	}
	upd, err := client.UpdateNote(ctx, &gen.UpdateNoteRequest{Id: n.Id, Title: "g2", Content: "x", IfMatch: n.Etag})
	if err != nil || upd.Version != 2 {
		t.Fatalf("update failed: %v %v", upd, err)
	}
	list, err := client.ListNotes(ctx, &emptypb.Empty{})
	if err != nil || len(list.Notes) != 1 {
		t.Fatalf("list failed: %v %v", list, err)
	}
	if _, err := client.DeleteNote(ctx, &gen.DeleteNoteRequest{Id: n.Id, IfMatch: upd.Etag}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetNote(ctx, &gen.GetNoteRequest{Id: n.Id}); status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}
}

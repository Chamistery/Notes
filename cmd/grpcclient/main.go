package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	"notes/api/gen"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:9090", "gRPC server address")
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		usage()
	}
	cc, err := grpc.NewClient(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fail(err)
	}
	defer cc.Close()
	client := gen.NewNotesServiceClient(cc)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var msg proto.Message
	switch args[0] {
	case "create":
		need(args, 3)
		msg, err = client.CreateNote(ctx, &gen.CreateNoteRequest{Title: args[1], Content: args[2]})
	case "get":
		need(args, 2)
		msg, err = client.GetNote(ctx, &gen.GetNoteRequest{Id: args[1]})
	case "list":
		msg, err = client.ListNotes(ctx, &emptypb.Empty{})
	case "update":
		need(args, 5)
		msg, err = client.UpdateNote(ctx, &gen.UpdateNoteRequest{Id: args[1], IfMatch: args[2], Title: args[3], Content: args[4]})
	case "delete":
		need(args, 3)
		msg, err = client.DeleteNote(ctx, &gen.DeleteNoteRequest{Id: args[1], IfMatch: args[2]})
	default:
		usage()
	}
	if err != nil {
		fail(err)
	}
	out, _ := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(msg)
	fmt.Println(string(out))
}

func need(args []string, n int) {
	if len(args) < n {
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  grpcclient [-addr host:port] create <title> <content>
  grpcclient [-addr host:port] get <id>
  grpcclient [-addr host:port] list
  grpcclient [-addr host:port] update <id> <if_match|*> <title> <content>
  grpcclient [-addr host:port] delete <id> <if_match|*>`)
	os.Exit(2)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

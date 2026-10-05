package grpcsrv

import (
	"context"
	"errors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"notes/api/gen"
	"notes/internal/notes"
)

type Service struct {
	gen.UnimplementedNotesServiceServer
	store *notes.Store
}

func NewServer(store *notes.Store) *grpc.Server {
	srv := grpc.NewServer()
	gen.RegisterNotesServiceServer(srv, &Service{store: store})
	reflection.Register(srv)
	return srv
}

func toProto(n notes.Note) *gen.Note {
	return &gen.Note{
		Id:        n.ID,
		Title:     n.Title,
		Content:   n.Content,
		CreatedAt: timestamppb.New(n.CreatedAt),
		UpdatedAt: timestamppb.New(n.UpdatedAt),
		Version:   n.Version,
		Etag:      n.ETag(),
	}
}

func toStatus(err error) error {
	switch {
	case errors.Is(err, notes.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, notes.ErrPreconditionFailed):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, notes.ErrInvalid):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

func (s *Service) CreateNote(_ context.Context, req *gen.CreateNoteRequest) (*gen.Note, error) {
	n, err := s.store.Create(req.GetTitle(), req.GetContent())
	if err != nil {
		return nil, toStatus(err)
	}
	return toProto(n), nil
}

func (s *Service) GetNote(_ context.Context, req *gen.GetNoteRequest) (*gen.Note, error) {
	n, err := s.store.Get(req.GetId())
	if err != nil {
		return nil, toStatus(err)
	}
	return toProto(n), nil
}

func (s *Service) ListNotes(context.Context, *emptypb.Empty) (*gen.ListNotesResponse, error) {
	all := s.store.List()
	out := &gen.ListNotesResponse{Notes: make([]*gen.Note, 0, len(all))}
	for _, n := range all {
		out.Notes = append(out.Notes, toProto(n))
	}
	return out, nil
}

func (s *Service) UpdateNote(_ context.Context, req *gen.UpdateNoteRequest) (*gen.Note, error) {
	n, err := s.store.Update(req.GetId(), req.GetIfMatch(), req.GetTitle(), req.GetContent())
	if err != nil {
		return nil, toStatus(err)
	}
	return toProto(n), nil
}

func (s *Service) DeleteNote(_ context.Context, req *gen.DeleteNoteRequest) (*emptypb.Empty, error) {
	if err := s.store.Delete(req.GetId(), req.GetIfMatch()); err != nil {
		return nil, toStatus(err)
	}
	return &emptypb.Empty{}, nil
}

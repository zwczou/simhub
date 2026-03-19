package errfmt

import (
	"testing"

	"github.com/iot/simhub/apis/basepb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestErrorf(t *testing.T) {
	err := Errorf(codes.NotFound, basepb.Code_CODE_NOT_FOUND, "test message")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("expected gRPC status error")
	}

	if st.Code() != codes.NotFound {
		t.Errorf("expected code %v, got %v", codes.NotFound, st.Code())
	}

	pbErr := Parse(err)
	if pbErr == nil {
		t.Fatal("expected pb.Error details, got nil")
	}

	if pbErr.Code != uint32(basepb.Code_CODE_NOT_FOUND) {
		t.Errorf("expected pb code %v, got %v", basepb.Code_CODE_NOT_FOUND, pbErr.Code)
	}
}

func TestShortcuts(t *testing.T) {
	tests := []struct {
		name     string
		fn       func(basepb.Code, ...any) error
		expected codes.Code
	}{
		{"Internal", Internal, codes.Internal},
		{"NotFound", NotFound, codes.NotFound},
		{"Unauthorized", Unauthorized, codes.Unauthenticated},
		{"Forbidden", Forbidden, codes.PermissionDenied},
		{"BadRequest", BadRequest, codes.InvalidArgument},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn(basepb.Code_CODE_INTERNAL_SERVER, "msg")
			st, _ := status.FromError(err)
			if st.Code() != tt.expected {
				t.Errorf("%s: expected code %v, got %v", tt.name, tt.expected, st.Code())
			}
		})
	}
}

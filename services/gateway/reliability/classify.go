package reliability

import (
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func Retryable(err error) bool {
	if err == nil {
		return false
	}
	st, ok := status.FromError(err)
	if !ok {
		return true
	}
	switch st.Code() {
	case codes.Unavailable, codes.ResourceExhausted, codes.Internal, codes.Unknown:
		return true
	case codes.Canceled, codes.DeadlineExceeded, codes.InvalidArgument, codes.NotFound, codes.PermissionDenied:
		return false
	default:
		return false
	}
}

func IsExcluded(err error) bool {
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	return st.Code() == codes.Canceled || st.Code() == codes.DeadlineExceeded
}

func IsBreakerOpen(err error) bool {
	return errors.Is(err, ErrOpen)
}
